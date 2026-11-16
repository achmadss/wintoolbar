// Package media follows the current Windows media session (SMTC) and sends
// transport commands to it.
//
// All WinRT work runs on one worker goroutine. WinRT event handlers only set
// a flag and wake the worker; the worker refreshes the state and calls
// Notify. If SMTC is unavailable, State.Active stays false.
package media

import (
	"hash/fnv"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	syswinrt "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/winrt"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/winrt/media/control"

	"wintoolbar/internal/win"
)

// State is an immutable snapshot of the current session.
type State struct {
	Active  bool
	App     string
	Title   string
	Artist  string
	Playing bool

	CanPrev, CanNext, CanPlayPause, CanStop bool

	Art   *Art   // nil when the session has no thumbnail
	ArtID uint64 // changes when Art changes
}

// Command is a transport action.
type Command int

const (
	Previous Command = iota
	PlayPause
	Next
	Stop
)

const (
	wakeSession  = 1 << iota // current session changed: resubscribe
	wakeProps                // title, artist, thumbnail
	wakePlayback             // status and enabled buttons
)

// Watcher runs the worker. Create with Start, end with Close.
type Watcher struct {
	notify func()

	mu    sync.Mutex
	state State

	flags atomic.Uint32
	wake  chan struct{}
	cmds  chan Command
	quit  chan struct{}
	done  chan struct{}

	// Worker-only fields.
	mgr          *control.IGlobalSystemMediaTransportControlsSessionManager
	sess         *control.IGlobalSystemMediaTransportControlsSession
	mgrHandler   *control.TypedEventHandlerOfGlobalSystemMediaTransportControlsSessionManagerAndCurrentSessionChangedEventArgs
	listHandler  *control.TypedEventHandlerOfGlobalSystemMediaTransportControlsSessionManagerAndSessionsChangedEventArgs
	propsHandler *control.TypedEventHandlerOfGlobalSystemMediaTransportControlsSessionAndMediaPropertiesChangedEventArgs
	playHandler  *control.TypedEventHandlerOfGlobalSystemMediaTransportControlsSessionAndPlaybackInfoChangedEventArgs
	mgrToken     syswinrt.EventRegistrationToken
	listToken    syswinrt.EventRegistrationToken
	propsToken   syswinrt.EventRegistrationToken
	playToken    syswinrt.EventRegistrationToken
	artHash      uint64
	artID        uint64
}

// Start launches the worker. notify is called from the worker goroutine
// after every state change.
func Start(notify func()) *Watcher {
	w := &Watcher{
		notify: notify,
		wake:   make(chan struct{}, 1),
		cmds:   make(chan Command, 4),
		quit:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go w.run()
	return w
}

// State returns the latest snapshot.
func (w *Watcher) State() State {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// Send queues a transport command; it never blocks.
func (w *Watcher) Send(c Command) {
	select {
	case w.cmds <- c:
	default:
	}
}

// Close stops the worker and releases every WinRT object.
func (w *Watcher) Close() {
	close(w.quit)
	<-w.done
}

func (w *Watcher) signal(flags uint32) {
	w.flags.Or(flags)
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Watcher) publish(s State) {
	w.mu.Lock()
	w.state = s
	w.mu.Unlock()
	if w.notify != nil {
		w.notify()
	}
}

func (w *Watcher) run() {
	defer close(w.done)
	defer w.teardown()

	backoff := 5 * time.Second
	for w.mgr == nil {
		if err := w.connect(); err != nil {
			log.Printf("media: SMTC unavailable: %v (retry in %v)", err, backoff)
			select {
			case <-w.quit:
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
		}
	}
	w.signal(wakeSession)

	var cur State
	for {
		select {
		case <-w.quit:
			return
		case c := <-w.cmds:
			w.command(c)
		case <-w.wake:
			// Coalesce event bursts (a track change raises several events).
			time.Sleep(50 * time.Millisecond)
			f := w.flags.Swap(0)
			if f&wakeSession != 0 {
				w.subscribe()
				f |= wakeProps | wakePlayback
			}
			next := cur
			if w.sess == nil {
				next = State{}
			} else {
				if f&wakeProps != 0 {
					w.readProps(&next)
				}
				if f&wakePlayback != 0 {
					w.readPlayback(&next)
				}
			}
			cur = next
			w.publish(cur)
		}
	}
}

func (w *Watcher) connect() error {
	statics, err := control.GlobalSystemMediaTransportControlsSessionManagerStatics()
	if err != nil {
		return err
	}
	defer statics.Release()
	op, err := statics.RequestAsync()
	if err != nil {
		return err
	}
	defer op.Release() // Await does not release the operation
	mgr, err := op.Await()
	if err != nil {
		return err
	}
	w.mgr = mgr
	w.mgrHandler, err = control.NewTypedEventHandlerOfGlobalSystemMediaTransportControlsSessionManagerAndCurrentSessionChangedEventArgs(
		func(*control.IGlobalSystemMediaTransportControlsSessionManager, *control.ICurrentSessionChangedEventArgs) {
			w.signal(wakeSession)
		})
	if err == nil {
		w.mgrToken, _ = mgr.AddCurrentSessionChanged(w.mgrHandler)
	}
	w.listHandler, err = control.NewTypedEventHandlerOfGlobalSystemMediaTransportControlsSessionManagerAndSessionsChangedEventArgs(
		func(*control.IGlobalSystemMediaTransportControlsSessionManager, *control.ISessionsChangedEventArgs) {
			w.signal(wakeSession)
		})
	if err == nil {
		w.listToken, _ = mgr.AddSessionsChanged(w.listHandler)
	}
	return nil
}

// subscribe drops the old session and follows the current one.
func (w *Watcher) subscribe() {
	w.unsubscribe()
	s, err := w.mgr.GetCurrentSession()
	if err != nil || s == nil {
		return
	}
	w.sess = s
	w.propsHandler, err = control.NewTypedEventHandlerOfGlobalSystemMediaTransportControlsSessionAndMediaPropertiesChangedEventArgs(
		func(*control.IGlobalSystemMediaTransportControlsSession, *control.IMediaPropertiesChangedEventArgs) {
			w.signal(wakeProps)
		})
	if err == nil {
		w.propsToken, _ = s.AddMediaPropertiesChanged(w.propsHandler)
	}
	w.playHandler, err = control.NewTypedEventHandlerOfGlobalSystemMediaTransportControlsSessionAndPlaybackInfoChangedEventArgs(
		func(*control.IGlobalSystemMediaTransportControlsSession, *control.IPlaybackInfoChangedEventArgs) {
			w.signal(wakePlayback)
		})
	if err == nil {
		w.playToken, _ = s.AddPlaybackInfoChanged(w.playHandler)
	}
}

func (w *Watcher) unsubscribe() {
	if w.sess == nil {
		return
	}
	if w.propsHandler != nil {
		_ = w.sess.RemoveMediaPropertiesChanged(w.propsToken)
		w.propsHandler.Close()
		w.propsHandler = nil
	}
	if w.playHandler != nil {
		_ = w.sess.RemovePlaybackInfoChanged(w.playToken)
		w.playHandler.Close()
		w.playHandler = nil
	}
	w.sess.Release()
	w.sess = nil
}

func (w *Watcher) teardown() {
	w.unsubscribe()
	if w.mgr == nil {
		return
	}
	if w.mgrHandler != nil {
		_ = w.mgr.RemoveCurrentSessionChanged(w.mgrToken)
		w.mgrHandler.Close()
	}
	if w.listHandler != nil {
		_ = w.mgr.RemoveSessionsChanged(w.listToken)
		w.listHandler.Close()
	}
	w.mgr.Release()
	w.mgr = nil
}

func (w *Watcher) readProps(s *State) {
	s.Active = true
	s.App, _ = w.sess.SourceAppUserModelId()
	op, err := w.sess.TryGetMediaPropertiesAsync()
	if err != nil {
		return
	}
	defer op.Release()
	props, err := op.Await()
	if err != nil || props == nil {
		return
	}
	defer props.Release()
	s.Title, _ = props.Title()
	s.Artist, _ = props.Artist()

	art, hash := w.readArt(props)
	if hash != w.artHash {
		w.artHash = hash
		w.artID++
		s.Art = art
	}
	s.ArtID = w.artID
}

// readArt loads and decodes the thumbnail; unchanged bytes keep the old art.
func (w *Watcher) readArt(props *control.IGlobalSystemMediaTransportControlsSessionMediaProperties) (*Art, uint64) {
	ref, err := props.Thumbnail()
	if err != nil || ref == nil {
		return nil, 0
	}
	defer ref.Release()
	op, err := ref.OpenReadAsync()
	if err != nil {
		return nil, 0
	}
	defer op.Release()
	stream, err := op.Await()
	if err != nil || stream == nil {
		return nil, 0
	}
	defer stream.Release()
	data, err := win.ReadRandomAccessStream(unsafe.Pointer(stream), 8<<20)
	if err != nil || len(data) == 0 {
		return nil, 0
	}
	h := fnv.New64a()
	h.Write(data)
	sum := h.Sum64()
	if sum == w.artHash {
		w.mu.Lock()
		old := w.state.Art
		w.mu.Unlock()
		return old, sum
	}
	art, err := decodeArt(data)
	// The full-size decoded image (often 1280x720) is garbage now; hand the
	// pages back to Windows instead of keeping them in the Go heap.
	data = nil
	debug.FreeOSMemory()
	if err != nil {
		log.Printf("media: thumbnail decode: %v", err)
		return nil, sum
	}
	return art, sum
}

func (w *Watcher) readPlayback(s *State) {
	s.Active = true
	info, err := w.sess.GetPlaybackInfo()
	if err != nil || info == nil {
		return
	}
	defer info.Release()
	st, _ := info.PlaybackStatus()
	s.Playing = st == control.GlobalSystemMediaTransportControlsSessionPlaybackStatusPlaying
	c, err := info.Controls()
	if err != nil || c == nil {
		return
	}
	defer c.Release()
	s.CanPrev, _ = c.IsPreviousEnabled()
	s.CanNext, _ = c.IsNextEnabled()
	s.CanStop, _ = c.IsStopEnabled()
	toggle, _ := c.IsPlayPauseToggleEnabled()
	play, _ := c.IsPlayEnabled()
	pause, _ := c.IsPauseEnabled()
	s.CanPlayPause = toggle || play || pause
}

func (w *Watcher) command(c Command) {
	if w.sess == nil {
		return
	}
	var op *control.IAsyncOperationOfBool
	var err error
	switch c {
	case Previous:
		op, err = w.sess.TrySkipPreviousAsync()
	case Next:
		op, err = w.sess.TrySkipNextAsync()
	case PlayPause:
		op, err = w.sess.TryTogglePlayPauseAsync()
	case Stop:
		op, err = w.sess.TryStopAsync()
	}
	if err != nil || op == nil {
		log.Printf("media: command %d: %v", c, err)
		return
	}
	defer op.Release()
	if _, err := op.Await(); err != nil {
		log.Printf("media: command %d: %v", c, err)
	}
}
