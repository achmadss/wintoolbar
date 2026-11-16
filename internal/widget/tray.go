package widget

import (
	"log"

	"wintoolbar/internal/win"
)

const (
	cmdSettings = 1 + iota
	cmdExit
)

type tray struct {
	data win.NotifyIconData
	icon uintptr
}

func (t *tray) add(h uintptr, msg uint32) {
	if t.icon == 0 {
		t.icon = makeTrayIcon(win.GetSystemMetricsForDpi(win.SM_CXSMICON, win.GetDpiForWindow(h)))
	}
	t.data = win.NotifyIconData{Wnd: h, ID: 1, Flags: win.NIF_MESSAGE | win.NIF_ICON | win.NIF_TIP,
		CallbackMessage: msg, Icon: t.icon}
	copy(t.data.Tip[:], utf16z("wintoolbar"))
	win.ShellNotifyIcon(win.NIM_DELETE, &t.data) // after an Explorer restart the old entry may linger
	if !win.ShellNotifyIcon(win.NIM_ADD, &t.data) {
		log.Print("Shell_NotifyIcon add failed")
	}
}

func (t *tray) remove() {
	if t.data.Wnd != 0 {
		win.ShellNotifyIcon(win.NIM_DELETE, &t.data)
	}
	if t.icon != 0 {
		win.DestroyIcon(t.icon)
		t.icon = 0
	}
}

func utf16z(s string) []uint16 {
	out := make([]uint16, 0, len(s)+1)
	for _, r := range s {
		out = append(out, uint16(r))
	}
	return append(out, 0)
}

// makeTrayIcon draws four bars, the app mark, at size px.
func makeTrayIcon(size int32) uintptr {
	if size <= 0 {
		size = 16
	}
	bmp, pix := win.CreateDIB32(size, size)
	if bmp == 0 {
		return 0
	}
	defer win.DeleteObject(bmp)
	mask := win.CreateMonoBitmap(size, size)
	defer win.DeleteObject(mask)

	const color = 0xFF60CDFF // opaque, light blue: readable on light and dark trays
	heights := [4]int32{6, 12, 9, 15}
	bw := size / 6
	gap := (size - 4*bw) / 5
	for i, hh := range heights {
		bh := hh * size / 16
		x0 := gap + int32(i)*(bw+gap)
		for y := size - bh; y < size; y++ {
			for x := x0; x < x0+bw; x++ {
				pix[y*size+x] = color
			}
		}
	}
	return win.CreateIcon(bmp, mask)
}

func (a *App) showMenu() {
	m := win.CreatePopupMenu()
	defer win.DestroyMenu(m)
	win.AppendMenu(m, win.MF_STRING, cmdSettings, "Settings")
	win.AppendMenu(m, win.MF_SEPARATOR, 0, "")
	win.AppendMenu(m, win.MF_STRING, cmdExit, "Quit")

	pt := win.GetCursorPos()
	win.SetForegroundWindow(a.host) // required so the menu closes on outside click
	cmd := win.TrackPopupMenu(m, win.TPM_RIGHTBUTTON|win.TPM_RETURNCMD|win.TPM_NONOTIFY|win.TPM_BOTTOMALIGN, pt.X, pt.Y, a.host)
	win.PostMessage(a.host, win.WM_NULL, 0, 0)

	switch cmd {
	case cmdSettings:
		a.openSettings()
	case cmdExit:
		win.DestroyWindow(a.host)
	}
}
