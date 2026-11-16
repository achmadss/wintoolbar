package widget

import (
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"

	"wintoolbar/internal/config"
	"wintoolbar/internal/netmon"
	"wintoolbar/internal/win"
)

// The settings window uses native common controls (v6, from the manifest).
// It exists only while open: closing destroys every control, font and
// handler map. Every change applies to the live widget and saves the config.

const (
	settingsClass = "wintoolbar.settings"
	idCancel      = 2 // IDCANCEL, sent by IsDialogMessage on Esc

	// Client size and spacing in logical px.
	sw       = 460
	sh       = 430
	sMargin  = 12
	sRow     = 30
	sLabelW  = 110
	sTrackW  = 220
	sButtonH = 28
)

var pageNames = [...]string{"Layout", "Visualizer", "Media", "Network"}

var sectionLabels = map[string]string{
	config.SectionVisualizer: "Audio visualizer",
	config.SectionMedia:      "Media player",
	config.SectionNetwork:    "Network monitor",
}

type settingsWin struct {
	hwnd uintptr
	font uintptr
	tab  uintptr
	dpi  uint32
	page int

	pages    [len(pageNames)][]uintptr
	handlers map[uintptr]func(code uint32)
	loading  bool // true while filling controls: handlers ignore events

	// Layout page.
	lists    [config.ZoneCount]uintptr
	offsets  [config.ZoneCount]uintptr
	selected string // section ID selected in a zone list

	// Controls whose state depends on others.
	vizDeps   []uintptr
	mediaDeps []uintptr
	netDeps   []uintptr
	autoHide  uintptr
	stopBox   uintptr
	colorEdit uintptr
	autostart uintptr
	valLabels map[uintptr]uintptr // trackbar -> value label
}

func (s *settingsWin) sc(v int32) int32 { return (v*int32(s.dpi) + 48) / 96 }

// openSettings shows the window, or brings the open one to the front.
func (a *App) openSettings() {
	s := &a.settings
	if win.IsWindow(s.hwnd) {
		win.ShowWindow(s.hwnd, win.SW_SHOWNORMAL)
		win.SetForegroundWindow(s.hwnd)
		return
	}
	win.InitCommonControls()
	work, dpi := win.MonitorAt(win.GetCursorPos())
	s.dpi = dpi
	*s = settingsWin{dpi: dpi, handlers: map[uintptr]func(uint32){}, valLabels: map[uintptr]uintptr{}}

	const style = win.WS_OVERLAPPED | win.WS_CAPTION | win.WS_SYSMENU | win.WS_MINIMIZEBOX
	r := win.Rect{Right: s.sc(sw), Bottom: s.sc(sh)}
	win.AdjustWindowRectForDpi(&r, style, win.WS_EX_CONTROLPARENT, dpi)
	x := work.Left + (work.W()-r.W())/2
	y := work.Top + (work.H()-r.H())/2
	h, err := win.CreateWindowEx(win.WS_EX_CONTROLPARENT, win.UTF16Ptr(settingsClass), win.UTF16Ptr("wintoolbar settings"),
		style, x, y, r.W(), r.H(), 0, 0, a.inst)
	if err != nil {
		log.Printf("create settings window: %v", err)
		return
	}
	s.hwnd = h
	s.font = win.CreateFont(-s.sc(12), win.FW_NORMAL, "Segoe UI")
	s.build(a)
	s.load(a)
	s.showPage(0)
	// SetWindowPos, not ShowWindow: the first ShowWindow of a process may
	// take the launcher's show state (e.g. hidden).
	win.SetWindowPos(h, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_SHOWWINDOW)
	win.SetForegroundWindow(h)
}

func (s *settingsWin) close() {
	if win.IsWindow(s.hwnd) {
		win.DestroyWindow(s.hwnd)
	}
}

// ctl creates a child control on page (-1 = always visible).
func (s *settingsWin) ctl(page int, class, text string, style uint32, x, y, w, h int32) uintptr {
	c, err := win.CreateWindowEx(0, win.UTF16Ptr(class), win.UTF16Ptr(text),
		win.WS_CHILD|win.WS_VISIBLE|style, s.sc(x), s.sc(y), s.sc(w), s.sc(h), s.hwnd, 0, 0)
	if err != nil {
		return 0
	}
	win.SendMessage(c, win.WM_SETFONT, s.font, 0)
	if page >= 0 {
		s.pages[page] = append(s.pages[page], c)
	}
	return c
}

func (s *settingsWin) on(c uintptr, fn func(code uint32)) uintptr {
	s.handlers[c] = fn
	return c
}

func (s *settingsWin) label(page int, text string, x, y, w int32) uintptr {
	return s.ctl(page, "STATIC", text, win.SS_LEFT, x, y+4, w, 20)
}

func (s *settingsWin) check(page int, text string, x, y, w int32, fn func(bool)) uintptr {
	c := s.ctl(page, "BUTTON", text, win.BS_AUTOCHECKBOX|win.WS_TABSTOP, x, y, w, 24)
	return s.on(c, func(code uint32) {
		if code == win.BN_CLICKED {
			fn(win.SendMessage(c, win.BM_GETCHECK, 0, 0) == 1)
		}
	})
}

func (s *settingsWin) button(page int, text string, x, y, w int32, fn func()) uintptr {
	c := s.ctl(page, "BUTTON", text, win.BS_PUSHBUTTON|win.WS_TABSTOP, x, y, w, sButtonH)
	return s.on(c, func(code uint32) {
		if code == win.BN_CLICKED {
			fn()
		}
	})
}

// track is a labeled trackbar with a value readout.
func (s *settingsWin) track(page int, text string, y int32, lo, hi int, fn func(int)) uintptr {
	s.label(page, text, sMargin+8, y, sLabelW)
	c := s.ctl(page, "msctls_trackbar32", "", win.TBS_HORZ|win.TBS_AUTOTICKS|win.WS_TABSTOP, sMargin+8+sLabelW, y, sTrackW, 28)
	win.SendMessage(c, win.TBM_SETRANGE, 1, uintptr(lo)|uintptr(hi)<<16)
	win.SendMessage(c, win.TBM_SETPAGESIZE, 0, 1)
	v := s.label(page, "", sMargin+16+sLabelW+sTrackW, y, 40)
	s.valLabels[c] = v
	return s.on(c, func(uint32) {
		pos := int(win.SendMessage(c, win.TBM_GETPOS, 0, 0))
		win.SetWindowText(v, strconv.Itoa(pos))
		fn(pos)
	})
}

func (s *settingsWin) setTrack(c uintptr, v int) {
	win.SendMessage(c, win.TBM_SETPOS, 1, uintptr(v))
	win.SetWindowText(s.valLabels[c], strconv.Itoa(v))
}

func setCheck(c uintptr, on bool) {
	var v uintptr
	if on {
		v = 1
	}
	win.SendMessage(c, win.BM_SETCHECK, v, 0)
}

// build creates every control. Positions are logical px.
func (s *settingsWin) build(a *App) {
	cfg := &a.cfg
	// Only the tab strip is used; pages sit on the plain window background
	// so labels and checkboxes match it.
	s.tab = s.ctl(-1, "SysTabControl32", "", win.WS_CLIPSIBLINGS|win.WS_TABSTOP, 8, 8, sw-16, 26)
	for i, n := range pageNames {
		win.TabInsert(s.tab, i, n)
	}
	top := int32(8 + 26 + 14)
	x := int32(sMargin + 8)
	inner := int32(sw - 2*(sMargin+8))

	// --- Layout page ---
	y := top
	s.check(0, "Show widget in the taskbar", x, y, inner/2, func(on bool) { cfg.Visible = on; s.changed(a) })
	s.autostart = s.check(0, "Start with Windows", x+inner/2, y, inner/2, func(on bool) {
		if err := setAutostart(on); err != nil {
			log.Printf("autostart: %v", err)
		}
		setCheck(s.autostart, autostartEnabled())
	})
	y += sRow + 4
	s.label(0, "Select a widget, then move it to another zone or change its order.", x, y, inner)
	y += sRow - 4
	colW := (inner - 2*8) / 3
	for z := range config.ZoneCount {
		cx := x + int32(z)*(colW+8)
		s.label(0, strings.ToUpper(config.ZoneNames[z][:1])+config.ZoneNames[z][1:], cx, y, colW)
		lb := s.ctl(0, "LISTBOX", "", win.LBS_NOTIFY|win.LBS_NOINTEGRALHEIGHT|win.WS_VSCROLL|win.WS_TABSTOP|win.WS_BORDER, cx, y+24, colW, 84)
		s.lists[z] = lb
		zone := z
		s.on(lb, func(code uint32) {
			if code != win.LBN_SELCHANGE {
				return
			}
			i := int(win.SendMessage(lb, win.LB_GETCURSEL, 0, 0))
			list := *cfg.Layout.Zone(zone)
			if i >= 0 && i < len(list) {
				s.selected = list[i]
			}
			s.syncSelection(a)
		})
	}
	y += 24 + 84 + 10
	bw := (inner - 3*8) / 4
	s.button(0, "◀ Zone", x, y, bw, func() { s.move(a, -1, 0) })
	s.button(0, "▲ Earlier", x+bw+8, y, bw, func() { s.move(a, 0, -1) })
	s.button(0, "▼ Later", x+2*(bw+8), y, bw, func() { s.move(a, 0, 1) })
	s.button(0, "Zone ▶", x+3*(bw+8), y, bw, func() { s.move(a, 1, 0) })
	y += sRow + 14
	s.label(0, "Offset from the zone anchor, in pixels:", x, y, inner)
	y += sRow - 4
	for z := range config.ZoneCount {
		cx := x + int32(z)*(colW+8)
		s.label(0, strings.ToUpper(config.ZoneNames[z][:1])+config.ZoneNames[z][1:], cx, y, 50)
		style := uint32(win.ES_AUTOHSCROLL | win.WS_TABSTOP | win.WS_BORDER)
		lo, hi := 0, 2000
		if z == config.ZoneCenter {
			lo, hi = -2000, 2000
		} else {
			style |= win.ES_NUMBER
		}
		ed := s.ctl(0, "EDIT", "", style, cx+52, y+2, colW-52, 24)
		ud := s.ctl(0, "msctls_updown32", "", win.UDS_AUTOBUDDY|win.UDS_SETBUDDYINT|win.UDS_ALIGNRIGHT|win.UDS_ARROWKEYS|win.UDS_NOTHOUSANDS, 0, 0, 0, 0)
		win.SendMessage(ud, win.UDM_SETRANGE32, uintptr(lo), uintptr(hi))
		s.offsets[z] = ed
		zone := z
		s.on(ed, func(code uint32) {
			if code != win.EN_CHANGE {
				return
			}
			v, err := strconv.Atoi(strings.TrimSpace(win.GetWindowText(ed)))
			if err != nil {
				return
			}
			v = min(max(v, lo), hi)
			switch zone {
			case config.ZoneLeft:
				cfg.Layout.LeftOffset = v
			case config.ZoneCenter:
				cfg.Layout.CenterOffset = v
			default:
				cfg.Layout.RightOffset = v
			}
			s.changed(a)
		})
	}

	// --- Visualizer page ---
	vc := &cfg.Visualizer
	y = top
	s.check(1, "Show the audio visualizer", x, y, inner, func(on bool) { vc.Enabled = on; s.changed(a) })
	y += sRow + 6
	s.vizDeps = append(s.vizDeps,
		s.track(1, "Bars", y, 1, 20, func(v int) { vc.Bars = v; s.changed(a) }))
	y += sRow + 4
	s.vizDeps = append(s.vizDeps,
		s.track(1, "Sensitivity", y, 1, 3, func(v int) { vc.Sensitivity = v; s.changed(a) }))
	y += sRow + 4
	s.vizDeps = append(s.vizDeps,
		s.track(1, "Peak level", y, 1, 3, func(v int) { vc.Peak = v; s.changed(a) }))
	y += sRow + 8
	s.vizDeps = append(s.vizDeps,
		s.check(1, "Centered bars (grow from the middle)", x, y, inner, func(on bool) { vc.Centered = on; s.changed(a) }))
	y += sRow
	s.vizDeps = append(s.vizDeps,
		s.check(1, "Show a baseline during silence", x, y, inner, func(on bool) { vc.Baseline = on; s.changed(a) }))
	y += sRow
	s.autoHide = s.check(1, "Hide the baseline when no audio plays", x+20, y, inner-20, func(on bool) { vc.BaselineAutoHide = on; s.changed(a) })
	s.vizDeps = append(s.vizDeps, s.autoHide)
	y += sRow
	s.vizDeps = append(s.vizDeps,
		s.check(1, "Use the album art color", x, y, inner, func(on bool) { vc.AlbumColor = on; s.changed(a) }))
	y += sRow + 4
	s.vizDeps = append(s.vizDeps, s.label(1, "Fixed color", x, y, sLabelW))
	color := s.ctl(1, "EDIT", "", win.ES_AUTOHSCROLL|win.WS_TABSTOP|win.WS_BORDER, x+sLabelW, y+2, 90, 24)
	s.colorEdit = color
	s.vizDeps = append(s.vizDeps, color, s.label(1, "#RRGGBB, empty = automatic", x+sLabelW+98, y, 200))
	s.on(color, func(code uint32) {
		if code != win.EN_CHANGE {
			return
		}
		t := strings.TrimSpace(win.GetWindowText(color))
		h := strings.TrimPrefix(t, "#")
		if t != "" {
			if _, err := strconv.ParseUint(h, 16, 32); err != nil || len(h) != 6 {
				return // wait for a complete color
			}
			t = "#" + strings.ToUpper(h)
		}
		vc.Color = t
		s.changed(a)
	})
	s.vizDeps = append(s.vizDeps, color)

	// --- Media page ---
	mc := &cfg.Media
	y = top
	s.check(2, "Show the media player", x, y, inner, func(on bool) { mc.Enabled = on; s.changed(a) })
	y += sRow + 6
	s.mediaDeps = append(s.mediaDeps,
		s.check(2, "Show playback controls", x, y, inner, func(on bool) { mc.Controls = on; s.changed(a) }))
	y += sRow
	s.stopBox = s.check(2, "Show a stop button", x+20, y, inner-20, func(on bool) { mc.Stop = on; s.changed(a) })
	s.mediaDeps = append(s.mediaDeps, s.stopBox)
	y += sRow + 8
	s.label(2, "The player follows the session Windows shows in its media controls.", x, y, inner)
	y += sRow - 8
	s.label(2, "It takes no space while nothing is playing.", x, y, inner)

	// --- Network page ---
	nc := &cfg.Network
	y = top
	s.check(3, "Show the network monitor", x, y, inner, func(on bool) { nc.Enabled = on; s.changed(a) })
	y += sRow + 6
	s.netDeps = append(s.netDeps,
		s.check(3, "Show bits per second (Mb/s) instead of bytes (MB/s)", x, y, inner, func(on bool) { nc.Bits = on; s.changed(a) }))
	y += sRow + 6
	s.netDeps = append(s.netDeps, s.label(3, "Adapter", x, y, sLabelW))
	combo := s.ctl(3, "COMBOBOX", "", win.CBS_DROPDOWNLIST|win.WS_VSCROLL|win.WS_TABSTOP, x+sLabelW, y, inner-sLabelW, 200)
	s.netDeps = append(s.netDeps, combo)
	adapters := []string{""}
	win.SendMessageStr(combo, win.CB_ADDSTRING, 0, "Automatic")
	for _, c := range netmon.Enumerate(nil) {
		if !c.Hardware || c.Filter {
			continue
		}
		state := "down"
		if c.Up {
			state = "up"
		}
		adapters = append(adapters, c.Alias)
		win.SendMessageStr(combo, win.CB_ADDSTRING, 0, fmt.Sprintf("%s (%s)", c.Alias, state))
	}
	sel := 0
	for i, n := range adapters {
		if i > 0 && strings.EqualFold(n, nc.Adapter) {
			sel = i
		}
	}
	win.SendMessage(combo, win.CB_SETCURSEL, uintptr(sel), 0)
	s.on(combo, func(code uint32) {
		if code != win.CBN_SELCHANGE {
			return
		}
		i := int(win.SendMessage(combo, win.CB_GETCURSEL, 0, 0))
		if i >= 0 && i < len(adapters) {
			nc.Adapter = adapters[i]
			s.changed(a)
		}
	})

	// --- Bottom row, always visible ---
	by := int32(sh - 8 - sButtonH)
	s.button(-1, "Open config file", 8, by, 130, func() {
		if p, err := config.Path(); err == nil {
			win.ShellOpen(p)
		}
	})
	s.button(-1, "Close", sw-8-90, by, 90, s.close)
}

// load copies the config into the controls.
func (s *settingsWin) load(a *App) {
	s.loading = true
	defer func() { s.loading = false }()
	cfg := &a.cfg
	byText := func(page int, text string) uintptr {
		for _, c := range s.pages[page] {
			if win.GetWindowText(c) == text {
				return c
			}
		}
		return 0
	}
	setCheck(byText(0, "Show widget in the taskbar"), cfg.Visible)
	setCheck(s.autostart, autostartEnabled())
	for z := range config.ZoneCount {
		v := []int{cfg.Layout.LeftOffset, cfg.Layout.CenterOffset, cfg.Layout.RightOffset}[z]
		win.SetWindowText(s.offsets[z], strconv.Itoa(v))
	}
	s.fillLists(a)

	vc := &cfg.Visualizer
	setCheck(byText(1, "Show the audio visualizer"), vc.Enabled)
	s.setTrack(s.vizDeps[0], vc.Bars)
	s.setTrack(s.vizDeps[1], vc.Sensitivity)
	s.setTrack(s.vizDeps[2], vc.Peak)
	setCheck(byText(1, "Centered bars (grow from the middle)"), vc.Centered)
	setCheck(byText(1, "Show a baseline during silence"), vc.Baseline)
	setCheck(s.autoHide, vc.BaselineAutoHide)
	setCheck(byText(1, "Use the album art color"), vc.AlbumColor)
	win.SetWindowText(s.colorEdit, vc.Color)

	mc := &cfg.Media
	setCheck(byText(2, "Show the media player"), mc.Enabled)
	setCheck(byText(2, "Show playback controls"), mc.Controls)
	setCheck(s.stopBox, mc.Stop)

	nc := &cfg.Network
	setCheck(byText(3, "Show the network monitor"), nc.Enabled)
	setCheck(byText(3, "Show bits per second (Mb/s) instead of bytes (MB/s)"), nc.Bits)
	s.refreshEnabled(a)
}

// fillLists shows each zone's sections; disabled sections are marked.
func (s *settingsWin) fillLists(a *App) {
	for z := range config.ZoneCount {
		lb := s.lists[z]
		win.SendMessage(lb, win.LB_RESETCONTENT, 0, 0)
		for _, id := range *a.cfg.Layout.Zone(z) {
			text := sectionLabels[id]
			if sec := a.sectionByID(id); sec != nil && !sec.enabled(a) {
				text += " (off)"
			}
			win.SendMessageStr(lb, win.LB_ADDSTRING, 0, text)
		}
	}
	s.syncSelection(a)
}

// syncSelection highlights the selected section in its zone and clears the others.
func (s *settingsWin) syncSelection(a *App) {
	z, i := a.cfg.Layout.Find(s.selected)
	for k := range config.ZoneCount {
		sel := ^uintptr(0) // -1: no selection
		if k == z {
			sel = uintptr(i)
		}
		win.SendMessage(s.lists[k], win.LB_SETCURSEL, sel, 0)
	}
}

// move shifts the selected section by dz zones or di places.
func (s *settingsWin) move(a *App, dz, di int) {
	z, i := a.cfg.Layout.Find(s.selected)
	if z < 0 {
		return
	}
	nz := min(max(z+dz, 0), config.ZoneCount-1)
	ni := i + di
	if dz != 0 {
		if nz == z {
			return
		}
		ni = len(*a.cfg.Layout.Zone(nz)) // append at the end of the new zone
		if dz > 0 {
			ni = 0 // moving right: enter at the start, next to where it was
		}
	}
	if ni < 0 || (dz == 0 && ni >= len(*a.cfg.Layout.Zone(z))) {
		return
	}
	a.cfg.Layout.Move(s.selected, nz, ni)
	s.fillLists(a)
	s.changed(a)
}

func (s *settingsWin) refreshEnabled(a *App) {
	cfg := &a.cfg
	for _, c := range s.vizDeps {
		win.EnableWindow(c, cfg.Visualizer.Enabled)
	}
	win.EnableWindow(s.autoHide, cfg.Visualizer.Enabled && cfg.Visualizer.Baseline)
	for _, c := range s.mediaDeps {
		win.EnableWindow(c, cfg.Media.Enabled)
	}
	win.EnableWindow(s.stopBox, cfg.Media.Enabled && cfg.Media.Controls)
	for _, c := range s.netDeps {
		win.EnableWindow(c, cfg.Network.Enabled)
	}
}

// changed applies and saves the config after a control changed it.
func (s *settingsWin) changed(a *App) {
	if s.loading {
		return
	}
	s.refreshEnabled(a)
	s.fillListsKeepSel(a)
	a.save()
	a.readTheme()
	a.apply()
}

func (s *settingsWin) fillListsKeepSel(a *App) {
	s.loading = true
	s.fillLists(a)
	s.loading = false
}

func (s *settingsWin) showPage(p int) {
	s.page = p
	for i := range s.pages {
		cmd := int32(win.SW_HIDE)
		if i == p {
			cmd = win.SW_SHOW
		}
		for _, c := range s.pages[i] {
			win.ShowWindow(c, cmd)
		}
	}
}

func (a *App) settingsProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	s := &a.settings
	switch msg {
	case win.WM_COMMAND:
		if wp&0xFFFF == idCancel && lp == 0 {
			s.close()
			return 0
		}
		if fn := s.handlers[lp]; fn != nil && !s.loading {
			fn(uint32(wp >> 16))
		}
		return 0
	case win.WM_HSCROLL:
		if fn := s.handlers[lp]; fn != nil && !s.loading {
			fn(0)
		}
		return 0
	case win.WM_NOTIFY:
		if from, code := win.NotifyCode(lp); from == s.tab && code == win.TCN_SELCHANGE {
			s.showPage(int(win.SendMessage(s.tab, win.TCM_GETCURSEL, 0, 0)))
		}
		return 0
	case win.WM_CTLCOLORSTATIC, win.WM_CTLCOLORBTN:
		win.SetBkColor(wp, win.GetSysColor(win.COLOR_WINDOW))
		return win.GetSysColorBrush(win.COLOR_WINDOW)
	case win.WM_DPICHANGED:
		// Rebuild at the new DPI on the monitor the window moved to.
		s.close()
		win.PostMessage(a.host, msgOpenSettings, 0, 0)
		return 0
	case win.WM_CLOSE:
		s.close()
		return 0
	case win.WM_DESTROY:
		if s.font != 0 {
			win.DeleteObject(s.font)
		}
		*s = settingsWin{}
		// The window's controls, fonts and page mappings are dead now, so
		// collect and trim at once instead of waiting for the trim timer.
		debug.FreeOSMemory()
		win.TrimWorkingSet()
		return 0
	}
	return win.DefWindowProc(h, msg, wp, lp)
}
