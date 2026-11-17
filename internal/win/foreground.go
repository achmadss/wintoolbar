package win

import "unsafe"

// Foreground window and monitor queries. Everything here is meant for a poll
// path: no string building, no allocation.
const (
	DWMWA_EXTENDED_FRAME_BOUNDS = 9
	MONITOR_DEFAULTTONEAREST    = 2

	maxClassName = 64
)

var (
	dwmapi = systemDLL("dwmapi.dll")

	procGetForegroundWindow    = user32.proc("GetForegroundWindow")
	procGetWindowThreadProcess = user32.proc("GetWindowThreadProcessId")
	procGetShellWindow         = user32.proc("GetShellWindow")
	procGetClassName           = user32.proc("GetClassNameW")
	procIsIconic               = user32.proc("IsIconic")
	procMonitorFromWindow      = user32.proc("MonitorFromWindow")
	procDwmGetWindowAttribute  = dwmapi.proc("DwmGetWindowAttribute")
	procGetCurrentProcessID    = kern32.proc("GetCurrentProcessId")

	// Built once: FindWindow is polled, UTF16Ptr would allocate per call.
	trayClass = UTF16Ptr("Shell_TrayWnd")
)

// Foreground returns the window that owns the user's attention, 0 if none.
func Foreground() uintptr { return procGetForegroundWindow.Call0() }

// ShellWindow returns the shell's desktop window, 0 if unavailable.
func ShellWindow() uintptr { return procGetShellWindow.Call0() }

// IsMinimized reports whether h is minimized.
func IsMinimized(h uintptr) bool { return procIsIconic.Call0(h) != 0 }

// OwnerPID returns the process that created h.
func OwnerPID(h uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcess.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// PID returns the id of this process.
func PID() uint32 { return uint32(procGetCurrentProcessID.Call0()) }

// HasClass reports whether h belongs to the window class name. The name must
// match the whole class, so "Shell_TrayWnd" does not accept "Shell_TrayWndX".
func HasClass(h uintptr, name string) bool {
	var buf [maxClassName]uint16
	n := int(procGetClassName.Call0(h, uintptr(unsafe.Pointer(&buf[0])), maxClassName))
	if n != len(name) {
		return false
	}
	for i := 0; i < n; i++ {
		if buf[i] != uint16(name[i]) {
			return false
		}
	}
	return true
}

// MonitorRect returns the full rect of the monitor that contains h.
func MonitorRect(h uintptr) (Rect, bool) {
	return monitorRect(procMonitorFromWindow.Call0(h, MONITOR_DEFAULTTONEAREST))
}

// MonitorRectAt returns the full rect of the monitor nearest pt.
func MonitorRectAt(pt Point) (Rect, bool) {
	return monitorRect(procMonitorFromPoint.Call0(uintptr(*(*uint64)(unsafe.Pointer(&pt))), MONITOR_DEFAULTTONEAREST))
}

func monitorRect(m uintptr) (Rect, bool) {
	if m == 0 {
		return Rect{}, false
	}
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if procGetMonitorInfo.Call0(m, uintptr(unsafe.Pointer(&mi))) == 0 {
		return Rect{}, false
	}
	return mi.Monitor, true
}

// WindowFrame returns the visible frame of h: the DWM extended bounds when DWM
// knows them, else the window rect. GetWindowRect of a maximized window sticks
// out past its monitor, so the DWM value is the one to compare with a monitor.
func WindowFrame(h uintptr) (Rect, bool) {
	var r Rect
	if procDwmGetWindowAttribute.Find() {
		if procDwmGetWindowAttribute.Call0(h, DWMWA_EXTENDED_FRAME_BOUNDS,
			uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r)) == 0 {
			return r, true
		}
	}
	return GetWindowRect(h)
}
