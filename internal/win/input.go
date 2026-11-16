package win

import "unsafe"

const (
	WM_MOUSEMOVE   = 0x0200
	WM_LBUTTONDOWN = 0x0201
	WM_MOUSELEAVE  = 0x02A3
	TME_LEAVE      = 0x00000002
	HTCLIENT       = 1
)

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Hwnd      uintptr
	HoverTime uint32
}

var (
	procTrackMouseEvent             = user32.proc("TrackMouseEvent")
	procUpdateLayeredWindowIndirect = user32.proc("UpdateLayeredWindowIndirect")
)

// TrackMouseLeave asks for one WM_MOUSELEAVE when the cursor leaves h.
func TrackMouseLeave(h uintptr) {
	t := trackMouseEvent{Flags: TME_LEAVE, Hwnd: h}
	t.Size = uint32(unsafe.Sizeof(t))
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
}

type updateLayeredWindowInfo struct {
	Size   uint32
	HdcDst uintptr
	PtDst  *Point
	PSize  *Size
	HdcSrc uintptr
	PtSrc  *Point
	CrKey  uint32
	Blend  *BlendFunction
	Flags  uint32
	Dirty  *Rect
}

// UpdateLayeredWindowDirty is UpdateLayeredWindow limited to the dirty rect.
// size must equal the window size.
func UpdateLayeredWindowDirty(h uintptr, size *Size, src uintptr, dirty *Rect) error {
	var zero Point
	blend := BlendFunction{SourceConstantAlpha: 255, AlphaFormat: 1}
	info := updateLayeredWindowInfo{PSize: size, HdcSrc: src, PtSrc: &zero, Blend: &blend, Flags: ULW_ALPHA, Dirty: dirty}
	info.Size = uint32(unsafe.Sizeof(info))
	r, _, e := procUpdateLayeredWindowIndirect.Call(h, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return e
	}
	return nil
}

var (
	shcore                                 = systemDLL("shcore.dll")
	procCreateStreamOverRandomAccessStream = shcore.proc("CreateStreamOverRandomAccessStream")
	iidIStream                             = NewGUID(0x0000000C, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46})
)

// ReadRandomAccessStream reads a WinRT IRandomAccessStream (any interface
// pointer of the object) to the end, up to limit bytes.
func ReadRandomAccessStream(stream unsafe.Pointer, limit int) ([]byte, error) {
	var istream uintptr
	hr := HRESULT(procCreateStreamOverRandomAccessStream.Call0(uintptr(stream), uintptr(unsafe.Pointer(&iidIStream)), uintptr(unsafe.Pointer(&istream))))
	if hr.Failed() {
		return nil, hr
	}
	defer Release(istream)
	out := make([]byte, 0, 64<<10)
	chunk := make([]byte, 64<<10)
	for len(out) < limit {
		var n uint32
		hr := VCall(istream, 3 /*ISequentialStream::Read*/, uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)), uintptr(unsafe.Pointer(&n)), 0, 0, 0)
		if hr.Failed() {
			return nil, hr
		}
		out = append(out, chunk[:n]...)
		if n == 0 || hr == 1 /*S_FALSE: end*/ {
			break
		}
	}
	return out, nil
}
