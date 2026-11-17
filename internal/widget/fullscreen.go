package widget

import "wintoolbar/internal/win"

// fullscreenSlack is the pixel error allowed when comparing a window frame with
// a monitor rect; DPI rounding and DWM borders are off by a pixel or two.
const fullscreenSlack = 2

// shellClasses are the shell's own windows: the desktop, the wallpaper host and
// the taskbars. They cover a monitor but are not fullscreen content.
var shellClasses = [...]string{"Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd"}

// fullscreenOnTop reports whether the foreground window fills the monitor that
// holds the taskbar. The widget lives on that taskbar, so fullscreen content on
// another monitor must not hide it.
func fullscreenOnTop(tb uintptr) bool {
	h := win.Foreground()
	if h == 0 || h == win.ShellWindow() || !win.IsWindowVisible(h) || win.IsMinimized(h) {
		return false
	}
	if win.OwnerPID(h) == win.PID() {
		return false
	}
	for i := range shellClasses {
		if win.HasClass(h, shellClasses[i]) {
			return false
		}
	}
	r, ok := win.WindowFrame(h)
	if !ok {
		return false
	}
	m, ok := taskbarMonitor(tb)
	if !ok {
		c := win.Point{X: (r.Left + r.Right) / 2, Y: (r.Top + r.Bottom) / 2}
		if m, ok = win.MonitorRectAt(c); !ok {
			return false
		}
	}
	return r.Left <= m.Left+fullscreenSlack && r.Top <= m.Top+fullscreenSlack &&
		r.Right >= m.Right-fullscreenSlack && r.Bottom >= m.Bottom-fullscreenSlack
}

// taskbarMonitor returns the monitor the given taskbar sits on.
func taskbarMonitor(tb uintptr) (win.Rect, bool) {
	if tb == 0 {
		return win.Rect{}, false
	}
	return win.MonitorRect(tb)
}
