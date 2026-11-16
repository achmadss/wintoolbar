package win

import (
	"syscall"
	"unsafe"
)

// Window styles and messages used by wintoolbar.
const (
	WS_POPUP        = 0x80000000
	WS_CHILD        = 0x40000000
	WS_VISIBLE      = 0x10000000
	WS_CLIPSIBLINGS = 0x04000000

	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_NOACTIVATE  = 0x08000000

	GWL_STYLE   = -16
	GWL_EXSTYLE = -20

	WM_CREATE        = 0x0001
	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_SETTINGCHANGE = 0x001A
	WM_DISPLAYCHANGE = 0x007E
	WM_NCHITTEST     = 0x0084
	WM_COMMAND       = 0x0111
	WM_TIMER         = 0x0113
	WM_MOUSEACTIVATE = 0x0021
	WM_DPICHANGED    = 0x02E0
	WM_LBUTTONUP     = 0x0202
	WM_RBUTTONUP     = 0x0205
	WM_CONTEXTMENU   = 0x007B
	WM_APP           = 0x8000
	WM_NULL          = 0x0000

	HTTRANSPARENT     = ^uintptr(0) // -1
	MA_NOACTIVATE     = 3
	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4

	SWP_NOSIZE         = 0x0001
	SWP_NOMOVE         = 0x0002
	SWP_NOZORDER       = 0x0004
	SWP_NOACTIVATE     = 0x0010
	SWP_SHOWWINDOW     = 0x0040
	SWP_HIDEWINDOW     = 0x0080
	SWP_ASYNCWINDOWPOS = 0x4000

	ULW_ALPHA = 0x00000002

	MF_STRING    = 0x0000
	MF_CHECKED   = 0x0008
	MF_SEPARATOR = 0x0800
	MF_GRAYED    = 0x0001

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100
	TPM_NONOTIFY    = 0x0080
	TPM_BOTTOMALIGN = 0x0020
)

// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the pseudo handle -4.
const DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = ^uintptr(3)

type Point struct{ X, Y int32 }

type Rect struct{ Left, Top, Right, Bottom int32 }

func (r Rect) W() int32 { return r.Right - r.Left }
func (r Rect) H() int32 { return r.Bottom - r.Top }

type Size struct{ CX, CY int32 }

type Msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
	private uint32
}

type WndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type BlendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

var (
	procRegisterClassEx        = user32.proc("RegisterClassExW")
	procCreateWindowEx         = user32.proc("CreateWindowExW")
	procDefWindowProc          = user32.proc("DefWindowProcW")
	procDestroyWindow          = user32.proc("DestroyWindow")
	procGetMessage             = user32.proc("GetMessageW")
	procTranslateMessage       = user32.proc("TranslateMessage")
	procDispatchMessage        = user32.proc("DispatchMessageW")
	procPostQuitMessage        = user32.proc("PostQuitMessage")
	procPostMessage            = user32.proc("PostMessageW")
	procSetTimer               = user32.proc("SetTimer")
	procKillTimer              = user32.proc("KillTimer")
	procFindWindow             = user32.proc("FindWindowW")
	procFindWindowEx           = user32.proc("FindWindowExW")
	procGetWindowRect          = user32.proc("GetWindowRect")
	procSetParent              = user32.proc("SetParent")
	procGetParent              = user32.proc("GetParent")
	procSetWindowPos           = user32.proc("SetWindowPos")
	procShowWindow             = user32.proc("ShowWindow")
	procIsWindow               = user32.proc("IsWindow")
	procUpdateLayeredWindow    = user32.proc("UpdateLayeredWindow")
	procGetDpiForWindow        = user32.proc("GetDpiForWindow")
	procSetProcessDpiAwareness = user32.proc("SetProcessDpiAwarenessContext")
	procRegisterWindowMessage  = user32.proc("RegisterWindowMessageW")
	procCreatePopupMenu        = user32.proc("CreatePopupMenu")
	procAppendMenu             = user32.proc("AppendMenuW")
	procTrackPopupMenu         = user32.proc("TrackPopupMenu")
	procDestroyMenu            = user32.proc("DestroyMenu")
	procSetForegroundWindow    = user32.proc("SetForegroundWindow")
	procGetCursorPos           = user32.proc("GetCursorPos")
	procGetDC                  = user32.proc("GetDC")
	procReleaseDC              = user32.proc("ReleaseDC")
	procCreateIconIndirect     = user32.proc("CreateIconIndirect")
	procDestroyIcon            = user32.proc("DestroyIcon")
	procSetWindowLongPtr       = user32.proc("SetWindowLongPtrW")
	procGetWindowLongPtr       = user32.proc("GetWindowLongPtrW")
	procMapWindowPoints        = user32.proc("MapWindowPoints")
	procIsWindowVisible        = user32.proc("IsWindowVisible")
)

func RegisterClassEx(wc *WndClassEx) uint16 {
	wc.Size = uint32(unsafe.Sizeof(*wc))
	return uint16(procRegisterClassEx.Call0(uintptr(unsafe.Pointer(wc))))
}

func CreateWindowEx(exStyle uint32, class, title *uint16, style uint32, x, y, w, h int32, parent, menu, inst uintptr) (uintptr, error) {
	r, _, e := procCreateWindowEx.Call(uintptr(exStyle), uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
		uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, menu, inst, 0)
	if r == 0 {
		return 0, e
	}
	return r, nil
}

func DefWindowProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	return procDefWindowProc.Call0(h, uintptr(msg), wp, lp)
}

func DestroyWindow(h uintptr)                         { procDestroyWindow.Call(h) }
func PostQuitMessage(code int32)                      { procPostQuitMessage.Call(uintptr(code)) }
func PostMessage(h uintptr, m uint32, wp, lp uintptr) { procPostMessage.Call(h, uintptr(m), wp, lp) }
func TranslateMessage(m *Msg)                         { procTranslateMessage.Call(uintptr(unsafe.Pointer(m))) }
func DispatchMessage(m *Msg)                          { procDispatchMessage.Call(uintptr(unsafe.Pointer(m))) }

// GetMessage returns false on WM_QUIT or error.
func GetMessage(m *Msg) bool {
	r := procGetMessage.Call0(uintptr(unsafe.Pointer(m)), 0, 0, 0)
	return int32(r) > 0
}

func SetTimer(h, id uintptr, ms uint32) { procSetTimer.Call(h, id, uintptr(ms), 0) }
func KillTimer(h, id uintptr)           { procKillTimer.Call(h, id) }

func FindWindow(class string) uintptr {
	return procFindWindow.Call0(uintptr(unsafe.Pointer(UTF16Ptr(class))), 0)
}

func FindWindowEx(parent, after uintptr, class string) uintptr {
	return procFindWindowEx.Call0(parent, after, uintptr(unsafe.Pointer(UTF16Ptr(class))), 0)
}

func GetWindowRect(h uintptr) (Rect, bool) {
	var r Rect
	ok := procGetWindowRect.Call0(h, uintptr(unsafe.Pointer(&r))) != 0
	return r, ok
}

func SetParent(child, parent uintptr) uintptr { return procSetParent.Call0(child, parent) }
func GetParent(h uintptr) uintptr             { return procGetParent.Call0(h) }

func SetWindowPos(h, after uintptr, x, y, w, ht int32, flags uint32) bool {
	return procSetWindowPos.Call0(h, after, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), uintptr(flags)) != 0
}

func ShowWindow(h uintptr, cmd int32) { procShowWindow.Call(h, uintptr(cmd)) }
func IsWindow(h uintptr) bool         { return h != 0 && procIsWindow.Call0(h) != 0 }
func IsWindowVisible(h uintptr) bool  { return procIsWindowVisible.Call0(h) != 0 }

// UpdateLayeredWindow pushes a premultiplied 32-bit bitmap selected into src.
// pos is in parent client coordinates for a child window.
func UpdateLayeredWindow(h uintptr, pos *Point, size *Size, src uintptr) error {
	var zero Point
	blend := BlendFunction{SourceConstantAlpha: 255, AlphaFormat: 1} // AC_SRC_ALPHA
	r, _, e := procUpdateLayeredWindow.Call(h, 0, uintptr(unsafe.Pointer(pos)), uintptr(unsafe.Pointer(size)),
		src, uintptr(unsafe.Pointer(&zero)), 0, uintptr(unsafe.Pointer(&blend)), ULW_ALPHA)
	if r == 0 {
		return e
	}
	return nil
}

func GetDpiForWindow(h uintptr) uint32 {
	if !procGetDpiForWindow.Find() {
		return 96
	}
	d := uint32(procGetDpiForWindow.Call0(h))
	if d == 0 {
		return 96
	}
	return d
}

func SetPerMonitorDpiAware() bool {
	if !procSetProcessDpiAwareness.Find() {
		return false
	}
	return procSetProcessDpiAwareness.Call0(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2) != 0
}

func RegisterWindowMessage(name string) uint32 {
	return uint32(procRegisterWindowMessage.Call0(uintptr(unsafe.Pointer(UTF16Ptr(name)))))
}

func CreatePopupMenu() uintptr { return procCreatePopupMenu.Call0() }
func DestroyMenu(m uintptr)    { procDestroyMenu.Call(m) }

func AppendMenu(m uintptr, flags uint32, id uintptr, text string) {
	var p uintptr
	if text != "" {
		p = uintptr(unsafe.Pointer(UTF16Ptr(text)))
	}
	procAppendMenu.Call(m, uintptr(flags), id, p)
}

func TrackPopupMenu(m uintptr, flags uint32, x, y int32, h uintptr) uintptr {
	return procTrackPopupMenu.Call0(m, uintptr(flags), uintptr(x), uintptr(y), 0, h, 0)
}

func SetForegroundWindow(h uintptr) { procSetForegroundWindow.Call(h) }

func GetCursorPos() Point {
	var p Point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func GetDC(h uintptr) uintptr { return procGetDC.Call0(h) }
func ReleaseDC(h, dc uintptr) { procReleaseDC.Call(h, dc) }
func DestroyIcon(i uintptr)   { procDestroyIcon.Call(i) }

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	Mask     uintptr
	Color    uintptr
}

// CreateIcon builds an icon from a 32-bit color bitmap and a mask bitmap.
func CreateIcon(color, mask uintptr) uintptr {
	ii := iconInfo{FIcon: 1, Mask: mask, Color: color}
	return procCreateIconIndirect.Call0(uintptr(unsafe.Pointer(&ii)))
}

func GetWindowLongPtr(h uintptr, idx int32) uintptr {
	return procGetWindowLongPtr.Call0(h, uintptr(idx))
}

func SetWindowLongPtr(h uintptr, idx int32, v uintptr) {
	procSetWindowLongPtr.Call(h, uintptr(idx), v)
}

// ScreenToClientRect maps a screen rect into the client coordinates of h.
func ScreenToClientRect(h uintptr, r Rect) Rect {
	procMapWindowPoints.Call(0, h, uintptr(unsafe.Pointer(&r)), 2)
	return r
}

// NewCallback wraps a Go window procedure.
func NewCallback(fn func(h uintptr, msg uint32, wp, lp uintptr) uintptr) uintptr {
	return syscall.NewCallback(fn)
}

var procGetSystemMetrics = user32.proc("GetSystemMetricsForDpi")

const SM_CXSMICON = 49

// GetSystemMetricsForDpi returns a metric scaled for dpi.
func GetSystemMetricsForDpi(idx int32, dpi uint32) int32 {
	return int32(procGetSystemMetrics.Call0(uintptr(idx), uintptr(dpi)))
}

var procLoadCursor = user32.proc("LoadCursorW")

// LoadArrowCursor returns the shared IDC_ARROW cursor.
func LoadArrowCursor() uintptr { return procLoadCursor.Call0(0, 32512) }
