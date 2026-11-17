package win

import (
	"cmp"
	"slices"
	"syscall"
	"unsafe"
)

// Taskbar is one taskbar window and the display it lives on.
type Taskbar struct {
	Hwnd    uintptr
	Device  string // monitor device name, such as `\\.\DISPLAY7`
	Primary bool
	Monitor Rect // full monitor rect
}

const secondaryTrayClass = "Shell_SecondaryTrayWnd"

// PrimaryTaskbar returns Shell_TrayWnd, 0 when Explorer is not running.
func PrimaryTaskbar() uintptr {
	return procFindWindow.Call0(uintptr(unsafe.Pointer(trayClass)), 0)
}

// Taskbars lists the primary taskbar first, then the secondary ones by monitor
// position. It allocates, so it belongs to apply and settings, not to frames.
func Taskbars() []Taskbar {
	out := make([]Taskbar, 0, 3)
	if h := PrimaryTaskbar(); h != 0 {
		out = append(out, taskbarInfo(h, true))
	}
	for h := FindWindowEx(0, 0, secondaryTrayClass); h != 0; h = FindWindowEx(0, h, secondaryTrayClass) {
		out = append(out, taskbarInfo(h, false))
	}
	slices.SortFunc(out, func(a, b Taskbar) int {
		if a.Primary != b.Primary {
			if a.Primary {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(a.Monitor.Left, b.Monitor.Left); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Monitor.Top, b.Monitor.Top); c != 0 {
			return c
		}
		return cmp.Compare(a.Hwnd, b.Hwnd)
	})
	return out
}

// taskbarInfo reads the monitor of one taskbar window.
func taskbarInfo(h uintptr, primary bool) Taskbar {
	t := Taskbar{Hwnd: h, Primary: primary}
	m := procMonitorFromWindow.Call0(h, MONITOR_DEFAULTTONEAREST)
	if m == 0 {
		return t
	}
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if procGetMonitorInfo.Call0(m, uintptr(unsafe.Pointer(&mi))) == 0 {
		return t
	}
	t.Monitor = mi.Monitor
	// UTF16ToString stops at the first NUL, so the unused tail is ignored.
	t.Device = syscall.UTF16ToString(mi.Device[:])
	return t
}
