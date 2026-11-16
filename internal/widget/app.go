// Package widget owns the windows: a hidden top-level host window (tray icon,
// timers, shell broadcasts) and up to three layered child windows inside
// Shell_TrayWnd, one per layout zone (left, center, right).
//
// Everything except audio capture runs on one locked OS thread; the window
// procedures and timers never run concurrently.
package widget

import (
	"log"

	"wintoolbar/internal/config"
	"wintoolbar/internal/win"
)

const (
	timerSecond = 1 // network poll and taskbar layout check
	timerRender = 2 // 33 ms visualizer frame, only while bars are visible
	timerTrim   = 3 // one-shot working-set trim after startup or settings

	msgTray         = win.WM_APP + 1
	msgAudio        = win.WM_APP + 2 // audio arrived while the render tick was stopped
	msgMedia        = win.WM_APP + 3 // media session state changed
	msgOpenSettings = win.WM_APP + 4

	hostClass   = "wintoolbar.host"
	widgetClass = "wintoolbar.widget"

	// sectionGap is the space between sections, FluentFlyout ItemSpacing.
	sectionGap = 8
)

// section is one widget part (visualizer, network, later media).
type section interface {
	enabled(a *App) bool
	// width in physical pixels at the current DPI.
	width(a *App) int32
	// place assigns the zone and the rect inside the zone window.
	place(z *zone, r win.Rect)
	// draw renders into the zone surface (no present).
	draw(a *App)
}

// zone is one click-through layered child window and its sections.
type zone struct {
	id       int
	hwnd     uintptr
	bounds   win.Rect // taskbar client coordinates
	surf     surface
	sections []section
}

// App is the single widget instance.
type App struct {
	inst uintptr
	host uintptr

	cfg config.Config

	taskbarCreated uint32
	tray           tray

	taskbar uintptr
	dpi     uint32
	light   bool // taskbar uses the light theme

	zones [config.ZoneCount]zone

	secondTimer bool

	vis   vizSection
	media mediaSection
	net   netSection

	settings settingsWin
}

var app *App

// Run creates the windows and runs the message loop until Exit.
func Run(cfg config.Config) error {
	app = &App{cfg: cfg, inst: win.ModuleHandle()}
	a := app
	for i := range a.zones {
		a.zones[i].id = i
	}
	a.net.init()
	a.vis.init(a)
	a.taskbarCreated = win.RegisterWindowMessage("TaskbarCreated")

	hostProc := win.NewCallback(a.hostProc)
	widgetProc := win.NewCallback(a.widgetProc)
	buttonsProc := win.NewCallback(a.buttonsProc)
	win.RegisterClassEx(&win.WndClassEx{WndProc: hostProc, Instance: a.inst, ClassName: win.UTF16Ptr(hostClass)})
	win.RegisterClassEx(&win.WndClassEx{WndProc: widgetProc, Instance: a.inst, ClassName: win.UTF16Ptr(widgetClass)})
	win.RegisterClassEx(&win.WndClassEx{WndProc: buttonsProc, Instance: a.inst, ClassName: win.UTF16Ptr(buttonsClass),
		Cursor: win.LoadArrowCursor()})
	settingsProc := win.NewCallback(a.settingsProc)

	h, err := win.CreateWindowEx(win.WS_EX_TOOLWINDOW, win.UTF16Ptr(hostClass), win.UTF16Ptr("wintoolbar"),
		win.WS_POPUP, 0, 0, 0, 0, 0, 0, a.inst)
	if err != nil {
		return err
	}
	a.host = h
	win.RegisterSessionNotification(h)
	a.tray.add(h, msgTray)
	a.scheduleTrim()
	if win.RegisterClassEx(&win.WndClassEx{WndProc: settingsProc, Instance: a.inst, ClassName: win.UTF16Ptr(settingsClass),
		Cursor: win.LoadArrowCursor(), Background: win.COLOR_WINDOW + 1, Icon: a.tray.icon, IconSm: a.tray.icon}) == 0 {
		log.Print("register settings class failed")
	}
	a.readTheme()
	a.apply()

	var m win.Msg
	for win.GetMessage(&m) {
		// Tab, arrow keys and Esc inside the settings window.
		if a.settings.hwnd != 0 && win.IsDialogMessage(a.settings.hwnd, &m) {
			continue
		}
		win.TranslateMessage(&m)
		win.DispatchMessage(&m)
	}
	a.settings.close()
	a.stopAll()
	a.tray.remove()
	return nil
}

// sectionByID maps config IDs to sections.
func (a *App) sectionByID(id string) section {
	switch id {
	case config.SectionVisualizer:
		return &a.vis
	case config.SectionMedia:
		return &a.media
	case config.SectionNetwork:
		return &a.net
	}
	return nil
}

func (a *App) stopAll() {
	a.vis.stop()
	a.media.stop()
	a.net.stop()
	a.setSecondTimer(false)
	for i := range a.zones {
		a.destroyZone(&a.zones[i])
	}
	a.taskbar = 0
}

// apply brings windows, capture and timers in line with the config.
func (a *App) apply() {
	// Assign enabled sections to zones in config order.
	a.vis.zone, a.media.zone, a.net.zone = nil, nil, nil
	any := false
	for i := range a.zones {
		z := &a.zones[i]
		z.sections = z.sections[:0]
		for _, id := range *a.cfg.Layout.Zone(i) {
			if s := a.sectionByID(id); s != nil && s.enabled(a) {
				z.sections = append(z.sections, s)
				any = true
			}
		}
	}
	tb := win.FindWindow("Shell_TrayWnd")
	if !a.cfg.Visible || !any || tb == 0 {
		// Hidden, or the taskbar is not there yet (TaskbarCreated retries).
		a.stopAll()
		return
	}
	a.taskbar = tb
	a.dpi = win.GetDpiForWindow(tb)
	a.net.measure(a)
	a.media.measure(a)
	for i := range a.zones {
		a.ensureZone(&a.zones[i])
	}
	a.setSecondTimer(true)
	if a.cfg.Network.Enabled {
		a.net.start(a)
	} else {
		a.net.stop()
	}
	if a.cfg.Visualizer.Enabled {
		a.vis.start()
	} else {
		a.vis.stop()
	}
	if a.cfg.Media.Enabled {
		a.media.start(a)
	} else {
		a.media.stop()
	}
	a.vis.readColor(a)
	a.layout(true)
}

// ensureZone creates or destroys the zone window to match its sections.
func (a *App) ensureZone(z *zone) {
	if len(z.sections) == 0 {
		a.destroyZone(z)
		return
	}
	if win.IsWindow(z.hwnd) && win.GetParent(z.hwnd) == a.taskbar {
		return
	}
	a.destroyZone(z)
	h, err := win.CreateWindowEx(
		win.WS_EX_LAYERED|win.WS_EX_TRANSPARENT|win.WS_EX_NOACTIVATE|win.WS_EX_TOOLWINDOW,
		win.UTF16Ptr(widgetClass), win.UTF16Ptr("wintoolbar"),
		win.WS_CHILD|win.WS_CLIPSIBLINGS, 0, 0, 1, 1, a.taskbar, 0, a.inst)
	if err != nil {
		log.Printf("create %s zone window: %v", config.ZoneNames[z.id], err)
		return
	}
	z.hwnd = h
	z.bounds = win.Rect{}
	log.Printf("%s zone window %#x in taskbar %#x", config.ZoneNames[z.id], h, a.taskbar)
}

func (a *App) destroyZone(z *zone) {
	if win.IsWindow(z.hwnd) {
		win.DestroyWindow(z.hwnd)
	}
	z.hwnd = 0
	z.bounds = win.Rect{}
	z.surf.free()
}

// taskbarAlive reports whether the zone windows still sit in the live taskbar.
func (a *App) taskbarAlive() bool {
	tb := win.FindWindow("Shell_TrayWnd")
	if tb == 0 || tb != a.taskbar {
		return false
	}
	for i := range a.zones {
		z := &a.zones[i]
		if len(z.sections) > 0 && !win.IsWindow(z.hwnd) {
			return false
		}
	}
	return true
}

// scheduleTrim trims the working set once, 10 s from now: startup code,
// WinRT activation and a track change's thumbnail decode are not needed
// after that. Closing the settings window trims immediately instead.
func (a *App) scheduleTrim() { win.SetTimer(a.host, timerTrim, 10_000) }

func (a *App) setSecondTimer(on bool) {
	if on == a.secondTimer {
		return
	}
	if on {
		win.SetTimer(a.host, timerSecond, 1000)
	} else {
		win.KillTimer(a.host, timerSecond)
	}
	a.secondTimer = on
}

// onSecond polls the network and re-checks the taskbar layout. If Explorer
// restarted without a TaskbarCreated broadcast, the windows are recreated.
func (a *App) onSecond() {
	if !a.taskbarAlive() {
		a.apply()
		return
	}
	changed := a.net.poll()
	moved := a.layout(false)
	if changed && a.net.zone != nil && !moved[a.net.zone.id] && a.begin(a.net.zone) {
		a.net.draw(a)
		a.present(a.net.zone)
	}
}

func (a *App) scale(v int32) int32 { return (v*int32(a.dpi) + 48) / 96 }

// taskbarLeftAligned reads the Windows 11 "Taskbar alignment" setting.
func taskbarLeftAligned() bool {
	v, ok := win.RegDWORD(`Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "TaskbarAl")
	return ok && v == 0
}

// layout positions every zone window. Zones whose size or position changed
// (or all, with force) are fully redrawn; the result marks them.
func (a *App) layout(force bool) (redrawn [config.ZoneCount]bool) {
	tbRect, ok := win.GetWindowRect(a.taskbar)
	if !ok {
		return
	}
	if dpi := win.GetDpiForWindow(a.taskbar); dpi != a.dpi {
		a.dpi = dpi
		a.net.measure(a)
		a.media.measure(a)
		force = true
	}
	h, tbW := tbRect.H(), tbRect.W()

	// Edges: left zone starts after the app icons when they are left-aligned;
	// right zone ends at the notification area.
	leftEdge := int32(0)
	if taskbarLeftAligned() {
		if rb := win.FindWindowEx(a.taskbar, 0, "ReBarWindow32"); rb != 0 {
			if r, ok := win.GetWindowRect(rb); ok {
				leftEdge = win.ScreenToClientRect(a.taskbar, r).Right
			}
		}
	}
	rightEdge := tbW
	if tn := win.FindWindowEx(a.taskbar, 0, "TrayNotifyWnd"); tn != 0 {
		if r, ok := win.GetWindowRect(tn); ok {
			rightEdge = win.ScreenToClientRect(a.taskbar, r).Left
		}
	}
	lay := &a.cfg.Layout

	for i := range a.zones {
		z := &a.zones[i]
		if !win.IsWindow(z.hwnd) {
			continue
		}
		// Section rects inside the zone, left to right.
		var w int32
		type placed struct {
			s section
			r win.Rect
		}
		var rects [len(config.Sections)]placed
		for j, s := range z.sections {
			sw := s.width(a)
			if sw <= 0 {
				rects[j] = placed{s, win.Rect{}} // hidden (e.g. no media session)
				continue
			}
			if w > 0 {
				w += a.scale(sectionGap)
			}
			rects[j] = placed{s, win.Rect{Left: w, Top: 0, Right: w + sw, Bottom: h}}
			w += sw
		}
		var x int32
		switch i {
		case config.ZoneLeft:
			x = leftEdge + a.scale(int32(lay.LeftOffset))
		case config.ZoneCenter:
			x = (tbW-w)/2 + a.scale(int32(lay.CenterOffset))
		default:
			x = rightEdge - w - a.scale(int32(lay.RightOffset))
		}
		nb := win.Rect{Left: x, Top: 0, Right: x + w, Bottom: h}
		if !force && nb == z.bounds {
			continue
		}
		z.bounds = nb
		for j := range z.sections {
			rects[j].s.place(z, rects[j].r)
		}
		win.SetWindowPos(z.hwnd, 0, nb.Left, nb.Top, nb.W(), nb.H(), win.SWP_NOACTIVATE|win.SWP_SHOWWINDOW)
		a.renderZone(z)
		redrawn[i] = true
	}
	a.media.syncButtons(a)
	return
}

func (a *App) readTheme() {
	v, ok := win.RegDWORD(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "SystemUsesLightTheme")
	a.light = ok && v != 0
	a.vis.readColor(a)
}

func (a *App) textColor() uint32 {
	if a.light {
		return 0x1A1A1A
	}
	return 0xFFFFFF
}

// begin readies the zone surface; false means there is nothing to draw into.
func (a *App) begin(z *zone) bool {
	return win.IsWindow(z.hwnd) && z.bounds.W() > 0 && z.surf.ensure(z.bounds.W(), z.bounds.H())
}

// present pushes the zone surface to its layered window.
func (a *App) present(z *zone) {
	size := win.Size{CX: z.surf.w, CY: z.surf.h}
	if err := win.UpdateLayeredWindow(z.hwnd, nil, &size, z.surf.dc); err != nil {
		log.Printf("UpdateLayeredWindow: %v", err)
	}
}

// renderZone redraws every section of one zone.
func (a *App) renderZone(z *zone) {
	if !a.begin(z) {
		return
	}
	z.surf.clear()
	for _, s := range z.sections {
		s.draw(a)
	}
	a.present(z)
}

func (a *App) save() {
	if err := config.Save(a.cfg); err != nil {
		log.Printf("save config: %v", err)
	}
}

func (a *App) hostProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win.WM_TIMER:
		switch wp {
		case timerSecond:
			a.onSecond()
		case timerRender:
			a.vis.tick(a)
		case timerTrim:
			win.KillTimer(a.host, timerTrim)
			win.TrimWorkingSet()
		}
		return 0
	case msgAudio:
		a.vis.wake(a)
		return 0
	case msgMedia:
		a.media.onState(a)
		return 0
	case msgOpenSettings:
		a.openSettings()
		return 0
	case msgTray:
		switch uint32(lp & 0xFFFF) {
		case win.WM_RBUTTONUP, win.WM_LBUTTONUP:
			a.showMenu()
		}
		return 0
	case win.WM_DISPLAYCHANGE, win.WM_DPICHANGED:
		a.apply()
		return 0
	case win.WM_SETTINGCHANGE:
		a.readTheme()
		a.apply()
		a.media.drawButtons(a, -1)
		return 0
	case win.WM_WTSSESSION_CHANGE:
		// Loopback can get stuck after unlock when the device changed while locked.
		if wp == win.WTS_SESSION_UNLOCK || wp == win.WTS_SESSION_LOGON {
			a.vis.capture.Restart()
		}
		return 0
	case win.WM_POWERBROADCAST:
		if wp == win.PBT_APMRESUMEAUTOMATIC {
			a.vis.capture.Restart()
		}
		return 1
	case win.WM_DESTROY:
		win.PostQuitMessage(0)
		return 0
	}
	if msg == a.taskbarCreated && msg != 0 {
		// Explorer restarted: the old child windows died with the old taskbar.
		a.tray.add(h, msgTray)
		a.stopAll()
		a.apply()
		return 0
	}
	return win.DefWindowProc(h, msg, wp, lp)
}

func (a *App) widgetProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win.WM_NCHITTEST:
		// The widget body is never a hit-test target.
		return win.HTTRANSPARENT
	case win.WM_MOUSEACTIVATE:
		return win.MA_NOACTIVATE
	case win.WM_DESTROY:
		for i := range a.zones {
			if a.zones[i].hwnd == h {
				a.zones[i].hwnd = 0
			}
		}
		return 0
	}
	return win.DefWindowProc(h, msg, wp, lp)
}
