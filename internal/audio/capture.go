// Package audio captures system output through WASAPI loopback and keeps the
// latest mono samples in a ring buffer.
//
// Capture runs on its own locked OS thread (COM MTA), woken by the WASAPI
// event. Recovery follows FluentFlyout's Visualizer.cs: the capture restarts
// when the default render device changes, when the device is invalidated,
// and on request (session unlock, resume from sleep).
package audio

import (
	"errors"
	"fmt"
	"log"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"wintoolbar/internal/win"
)

// RingSize is the number of mono samples kept, one FFT window.
const RingSize = 4096

var (
	clsidMMDeviceEnumerator = win.NewGUID(0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E})
	iidIMMDeviceEnumerator  = win.NewGUID(0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6})
	iidIAudioClient         = win.NewGUID(0x1CB9AD4C, 0xDBFA, 0x4C32, [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2})
	iidIAudioCaptureClient  = win.NewGUID(0xC8ADBD64, 0xE71E, 0x48A0, [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17})
	subtypeIEEEFloat        = win.NewGUID(0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71})
)

// vtable indexes.
const (
	enumGetDefaultAudioEndpoint = 4
	deviceActivate              = 3
	deviceGetID                 = 5
	clientInitialize            = 3
	clientGetMixFormat          = 8
	clientStart                 = 10
	clientStop                  = 11
	clientSetEventHandle        = 13
	clientGetService            = 14
	captureGetBuffer            = 3
	captureReleaseBuffer        = 4
	captureGetNextPacketSize    = 5
)

const (
	streamFlagsLoopback      = 0x00020000
	streamFlagsEventCallback = 0x00040000
	bufferFlagsSilent        = 0x2
	waveFormatIEEEFloat      = 3
	formatTagExtensible      = 0xFFFE

	hrDeviceInvalidated = 0x88890004
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// WAVEFORMATEXTENSIBLE is packed: SubFormat sits at byte 24, which a Go
// struct embedding the 18-byte WAVEFORMATEX would misplace (Go pads to 20).
const subFormatOffset = 24

var errRestart = errors.New("restart")

// Capture is a restartable loopback capture.
type Capture struct {
	mu    sync.Mutex
	ring  [RingSize]float32
	write uint64 // total samples written
	rate  atomic.Uint32

	// OnData is called from the capture thread when samples arrive after
	// the reader marked itself idle with SetIdle(true).
	OnData func()
	idle   atomic.Bool

	running atomic.Bool
	stop    chan struct{}
	restart chan struct{}
	done    chan struct{}
}

// SampleRate of the current device, or 0.
func (c *Capture) SampleRate() int { return int(c.rate.Load()) }

// SetIdle tells the capture thread to call OnData on the next packet.
func (c *Capture) SetIdle(v bool) { c.idle.Store(v) }

// Latest copies the newest RingSize samples, oldest first, into dst and
// returns the total sample counter (unchanged counter = no new audio).
func (c *Capture) Latest(dst *[RingSize]float32) uint64 {
	c.mu.Lock()
	w := c.write
	start := int(w % RingSize)
	n := copy(dst[:], c.ring[start:])
	copy(dst[n:], c.ring[:start])
	c.mu.Unlock()
	return w
}

// Start begins capturing. It is a no-op if already running.
func (c *Capture) Start() {
	if c.running.Swap(true) {
		return
	}
	c.stop = make(chan struct{})
	c.restart = make(chan struct{}, 1)
	c.done = make(chan struct{})
	go c.loop(c.stop, c.restart, c.done)
}

// Stop ends capturing and waits for the capture thread.
func (c *Capture) Stop() {
	if !c.running.Swap(false) {
		return
	}
	close(c.stop)
	<-c.done
	c.mu.Lock()
	clear(c.ring[:])
	c.mu.Unlock()
}

// Restart re-opens the device, for example after a session unlock.
func (c *Capture) Restart() {
	if !c.running.Load() {
		return
	}
	select {
	case c.restart <- struct{}{}:
	default:
	}
}

func (c *Capture) loop(stop, restart <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	win.CoInitializeEx(win.COINIT_MULTITHREADED)
	defer win.CoUninitialize()

	backoff := 500 * time.Millisecond
	for {
		err := c.session(stop, restart)
		if err == nil {
			return // stopped
		}
		if !errors.Is(err, errRestart) {
			log.Printf("audio: %v; retrying in %v", err, backoff)
		} else {
			backoff = 500 * time.Millisecond
		}
		c.rate.Store(0)
		select {
		case <-stop:
			return
		case <-time.After(backoff):
		}
		if !errors.Is(err, errRestart) {
			backoff = min(backoff*2, 5*time.Second)
		}
	}
}

func defaultDeviceID(enum uintptr) (string, uintptr, error) {
	var dev uintptr
	if hr := win.VCall(enum, enumGetDefaultAudioEndpoint, 0 /*eRender*/, 0 /*eConsole*/, uintptr(unsafe.Pointer(&dev)), 0, 0, 0); hr.Failed() {
		return "", 0, hr
	}
	var id *uint16
	if hr := win.VCall(dev, deviceGetID, uintptr(unsafe.Pointer(&id)), 0, 0, 0, 0, 0); hr.Failed() {
		win.Release(dev)
		return "", 0, hr
	}
	s := win.UTF16PtrToString(id)
	win.CoTaskMemFree(uintptr(unsafe.Pointer(id)))
	return s, dev, nil
}

// session runs one capture session. It returns nil when stopped, errRestart
// when the device should be reopened, or another error on failure.
func (c *Capture) session(stop, restart <-chan struct{}) error {
	enum, err := win.CoCreateInstance(&clsidMMDeviceEnumerator, &iidIMMDeviceEnumerator)
	if err != nil {
		return err
	}
	defer win.Release(enum)

	devID, dev, err := defaultDeviceID(enum)
	if err != nil {
		return err
	}
	defer win.Release(dev)

	var client uintptr
	if hr := win.VCall(dev, deviceActivate, uintptr(unsafe.Pointer(&iidIAudioClient)), win.CLSCTX_ALL, 0, uintptr(unsafe.Pointer(&client)), 0, 0); hr.Failed() {
		return hr
	}
	defer win.Release(client)

	var pwfx *waveFormatEx
	if hr := win.VCall(client, clientGetMixFormat, uintptr(unsafe.Pointer(&pwfx)), 0, 0, 0, 0, 0); hr.Failed() {
		return hr
	}
	defer win.CoTaskMemFree(uintptr(unsafe.Pointer(pwfx)))

	channels := int(pwfx.Channels)
	bits := int(pwfx.BitsPerSample)
	isFloat := pwfx.FormatTag == waveFormatIEEEFloat ||
		(pwfx.FormatTag == formatTagExtensible && pwfx.CbSize >= 22 &&
			*(*win.GUID)(unsafe.Add(unsafe.Pointer(pwfx), subFormatOffset)) == subtypeIEEEFloat)
	if channels == 0 || !(isFloat && bits == 32) && !(!isFloat && bits == 16) {
		return fmt.Errorf("unsupported mix format tag=%#x bits=%d ch=%d", pwfx.FormatTag, bits, channels)
	}

	const bufferDuration = 2_000_000 // 200 ms in 100 ns units
	if hr := win.VCall(client, clientInitialize, 0 /*shared*/, streamFlagsLoopback|streamFlagsEventCallback,
		bufferDuration, 0, uintptr(unsafe.Pointer(pwfx)), 0); hr.Failed() {
		return hr
	}
	event := win.CreateEvent()
	defer win.CloseHandle(event)
	if hr := win.VCall(client, clientSetEventHandle, event, 0, 0, 0, 0, 0); hr.Failed() {
		return hr
	}
	var capture uintptr
	if hr := win.VCall(client, clientGetService, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capture)), 0, 0, 0, 0); hr.Failed() {
		return hr
	}
	defer win.Release(capture)
	if hr := win.VCall(client, clientStart, 0, 0, 0, 0, 0, 0); hr.Failed() {
		return hr
	}
	defer win.VCall(client, clientStop, 0, 0, 0, 0, 0, 0)

	c.rate.Store(pwfx.SamplesPerSec)
	log.Printf("audio: capturing %d Hz, %d ch, %d-bit float=%v", pwfx.SamplesPerSec, channels, bits, isFloat)

	lastCheck := time.Now()
	for {
		select {
		case <-stop:
			return nil
		case <-restart:
			return errRestart
		default:
		}
		win.WaitForSingleObject(event, 200)

		if err := c.drain(capture, channels, isFloat); err != nil {
			return err
		}

		// The default device can change without invalidating this one.
		if time.Since(lastCheck) >= 2*time.Second {
			lastCheck = time.Now()
			id, d, err := defaultDeviceID(enum)
			if err == nil {
				win.Release(d)
			}
			if err != nil || id != devID {
				return errRestart
			}
		}
	}
}

// drain reads every pending packet into the ring buffer.
func (c *Capture) drain(capture uintptr, channels int, isFloat bool) error {
	got := false
	for {
		var packet uint32
		hr := win.VCall(capture, captureGetNextPacketSize, uintptr(unsafe.Pointer(&packet)), 0, 0, 0, 0, 0)
		if uint32(hr) == hrDeviceInvalidated {
			return errRestart
		}
		if hr.Failed() {
			return hr
		}
		if packet == 0 {
			break
		}
		var data unsafe.Pointer
		var frames, flags uint32
		hr = win.VCall(capture, captureGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)), 0, 0, 0)
		if uint32(hr) == hrDeviceInvalidated {
			return errRestart
		}
		if hr.Failed() {
			return hr
		}
		c.push(data, int(frames), channels, isFloat, flags&bufferFlagsSilent != 0)
		win.VCall(capture, captureReleaseBuffer, uintptr(frames), 0, 0, 0, 0, 0)
		got = true
	}
	if got && c.OnData != nil && c.idle.CompareAndSwap(true, false) {
		c.OnData()
	}
	return nil
}

// push downmixes frames to mono and appends them to the ring.
func (c *Capture) push(data unsafe.Pointer, frames, channels int, isFloat, silent bool) {
	inv := 1 / float32(channels)
	c.mu.Lock()
	w := c.write
	if silent || data == nil {
		for range frames {
			c.ring[w%RingSize] = 0
			w++
		}
	} else if isFloat {
		s := unsafe.Slice((*float32)(data), frames*channels)
		for f := range frames {
			var sum float32
			for _, v := range s[f*channels : (f+1)*channels] {
				sum += v
			}
			c.ring[w%RingSize] = sum * inv
			w++
		}
	} else {
		s := unsafe.Slice((*int16)(data), frames*channels)
		for f := range frames {
			var sum float32
			for _, v := range s[f*channels : (f+1)*channels] {
				sum += float32(v) / 32768
			}
			c.ring[w%RingSize] = sum * inv
			w++
		}
	}
	c.write = w
	c.mu.Unlock()
}

// Peak returns the largest absolute value in s (test helper and diagnostics).
func Peak(s []float32) float32 {
	var p float32
	for _, v := range s {
		p = max(p, float32(math.Abs(float64(v))))
	}
	return p
}
