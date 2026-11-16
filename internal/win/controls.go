package win

import (
	"syscall"
	"unsafe"
)

// Standard control styles and messages used by the settings window.
const (
	WS_OVERLAPPED  = 0x00000000
	WS_CAPTION     = 0x00C00000
	WS_SYSMENU     = 0x00080000
	WS_MINIMIZEBOX = 0x00020000
	WS_TABSTOP     = 0x00010000
	WS_GROUP       = 0x00020000
	WS_VSCROLL     = 0x00200000
	WS_BORDER      = 0x00800000
	WS_DISABLED    = 0x08000000

	WS_EX_CLIENTEDGE    = 0x00000200
	WS_EX_CONTROLPARENT = 0x00010000

	BS_PUSHBUTTON   = 0x0
	BS_AUTOCHECKBOX = 0x3
	BS_GROUPBOX     = 0x7

	SS_LEFT = 0x0

	ES_AUTOHSCROLL = 0x80
	ES_NUMBER      = 0x2000

	CBS_DROPDOWNLIST     = 0x3
	LBS_NOTIFY           = 0x1
	LBS_NOINTEGRALHEIGHT = 0x100

	TBS_HORZ      = 0x0
	TBS_AUTOTICKS = 0x1

	UDS_SETBUDDYINT = 0x2
	UDS_ALIGNRIGHT  = 0x4
	UDS_AUTOBUDDY   = 0x10
	UDS_ARROWKEYS   = 0x20
	UDS_NOTHOUSANDS = 0x80

	WM_SETFONT        = 0x0030
	WM_NOTIFY         = 0x004E
	WM_HSCROLL        = 0x0114
	WM_CTLCOLORSTATIC = 0x0138
	WM_CTLCOLORBTN    = 0x0135
	WM_GETMINMAXINFO  = 0x0024
	WM_KEYDOWN        = 0x0100

	BM_GETCHECK = 0x00F0
	BM_SETCHECK = 0x00F1
	BN_CLICKED  = 0

	EN_CHANGE = 0x0300

	CB_ADDSTRING    = 0x0143
	CB_GETCURSEL    = 0x0147
	CB_SETCURSEL    = 0x014E
	CB_RESETCONTENT = 0x014B
	CBN_SELCHANGE   = 1

	LB_ADDSTRING    = 0x0180
	LB_RESETCONTENT = 0x0184
	LB_SETCURSEL    = 0x0186
	LB_GETCURSEL    = 0x0188
	LBN_SELCHANGE   = 1

	TBM_GETPOS      = 0x0400
	TBM_SETPOS      = 0x0405
	TBM_SETRANGE    = 0x0406
	TBM_SETPAGESIZE = 0x0415

	UDM_SETRANGE32 = 0x046F
	UDM_SETPOS32   = 0x0471

	TCM_INSERTITEMW = 0x133E
	TCM_GETCURSEL   = 0x130B
	TCM_ADJUSTRECT  = 0x1328
	TCN_SELCHANGE   = ^uint32(550) // -551

	COLOR_WINDOW = 5

	ICC_BAR_CLASSES      = 0x4
	ICC_TAB_CLASSES      = 0x8
	ICC_UPDOWN_CLASS     = 0x10
	ICC_STANDARD_CLASSES = 0x4000

	SW_SHOW       = 5
	SW_SHOWNORMAL = 1
)

var (
	comctl32                     = systemDLL("comctl32.dll")
	procInitCommonControlsEx     = comctl32.proc("InitCommonControlsEx")
	procSendMessage              = user32.proc("SendMessageW")
	procSetWindowText            = user32.proc("SetWindowTextW")
	procGetWindowText            = user32.proc("GetWindowTextW")
	procGetWindowTextLength      = user32.proc("GetWindowTextLengthW")
	procEnableWindow             = user32.proc("EnableWindow")
	procIsDialogMessage          = user32.proc("IsDialogMessageW")
	procGetSysColorBrush         = user32.proc("GetSysColorBrush")
	procGetSysColor              = user32.proc("GetSysColor")
	procAdjustWindowRectExForDpi = user32.proc("AdjustWindowRectExForDpi")
	procMonitorFromPoint         = user32.proc("MonitorFromPoint")
	procGetMonitorInfo           = user32.proc("GetMonitorInfoW")
	procGetClientRect            = user32.proc("GetClientRect")
	procSetFocus                 = user32.proc("SetFocus")
	procBringWindowToTop         = user32.proc("BringWindowToTop")
	procSetBkColor               = gdi32.proc("SetBkColor")
	procGetDpiForMonitor         = shcore.proc("GetDpiForMonitor")
)

// InitCommonControls registers the trackbar, tab and up-down classes.
func InitCommonControls() {
	icc := struct{ Size, ICC uint32 }{8, ICC_BAR_CLASSES | ICC_TAB_CLASSES | ICC_UPDOWN_CLASS | ICC_STANDARD_CLASSES}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
}

func SendMessage(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	return procSendMessage.Call0(h, uintptr(msg), wp, lp)
}

// SendMessageStr sends a message whose lParam is a UTF-16 string.
func SendMessageStr(h uintptr, msg uint32, wp uintptr, s string) uintptr {
	return procSendMessage.Call0(h, uintptr(msg), wp, uintptr(unsafe.Pointer(UTF16Ptr(s))))
}

func SetWindowText(h uintptr, s string) {
	procSetWindowText.Call(h, uintptr(unsafe.Pointer(UTF16Ptr(s))))
}

func GetWindowText(h uintptr) string {
	n := int(procGetWindowTextLength.Call0(h))
	buf := make([]uint16, n+1)
	procGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func EnableWindow(h uintptr, on bool) {
	var v uintptr
	if on {
		v = 1
	}
	procEnableWindow.Call(h, v)
}

func IsDialogMessage(h uintptr, m *Msg) bool {
	return procIsDialogMessage.Call0(h, uintptr(unsafe.Pointer(m))) != 0
}

func GetSysColorBrush(i int) uintptr  { return procGetSysColorBrush.Call0(uintptr(i)) }
func GetSysColor(i int) uint32        { return uint32(procGetSysColor.Call0(uintptr(i))) }
func SetBkColor(dc uintptr, c uint32) { procSetBkColor.Call(dc, uintptr(c)) }
func SetFocus(h uintptr)              { procSetFocus.Call(h) }
func BringWindowToTop(h uintptr)      { procBringWindowToTop.Call(h) }

func GetClientRect(h uintptr) Rect {
	var r Rect
	procGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return r
}

// AdjustWindowRectForDpi grows a client rect to the window rect.
func AdjustWindowRectForDpi(r *Rect, style, exStyle, dpi uint32) {
	procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(r)), uintptr(style), 0, uintptr(exStyle), uintptr(dpi))
}

type monitorInfo struct {
	Size    uint32
	Monitor Rect
	Work    Rect
	Flags   uint32
}

// MonitorAt returns the work area and DPI of the monitor containing pt.
func MonitorAt(pt Point) (work Rect, dpi uint32) {
	m := procMonitorFromPoint.Call0(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 2 /*MONITOR_DEFAULTTONEAREST*/)
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	procGetMonitorInfo.Call(m, uintptr(unsafe.Pointer(&mi)))
	var dx, dy uint32
	if procGetDpiForMonitor.Call0(m, 0 /*MDT_EFFECTIVE_DPI*/, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))) != 0 || dx == 0 {
		dx = 96
	}
	return mi.Work, dx
}

// TabInsert adds a tab with text at index i.
func TabInsert(tab uintptr, i int, text string) {
	item := struct {
		Mask      uint32
		State     uint32
		StateMask uint32
		Text      *uint16
		TextMax   int32
		Image     int32
		Param     uintptr
	}{Mask: 1 /*TCIF_TEXT*/, Text: UTF16Ptr(text)}
	SendMessage(tab, TCM_INSERTITEMW, uintptr(i), uintptr(unsafe.Pointer(&item)))
}

// TabDisplayRect returns the page area of a tab control in its parent's coordinates.
func TabDisplayRect(tab uintptr, r Rect) Rect {
	SendMessage(tab, TCM_ADJUSTRECT, 0, uintptr(unsafe.Pointer(&r)))
	return r
}

// NotifyCode reads NMHDR.code from a WM_NOTIFY lParam.
func NotifyCode(lp uintptr) (from uintptr, code uint32) {
	// lp points to an NMHDR owned by the sender, not the Go heap.
	h := *(**struct {
		From uintptr
		ID   uintptr
		Code uint32
	})(unsafe.Pointer(&lp))
	return h.From, h.Code
}
