package widget

import (
	"strconv"
	"strings"

	"wintoolbar/internal/audio"
	"wintoolbar/internal/viz"
	"wintoolbar/internal/win"
)

// Visualizer box, from FluentFlyout TaskbarVisualizerControl: an 84x40
// control whose 4 px side margins leave a 76x32 bar area, centered.
// Bars are 2 px apart with a 2 px corner radius at 10 bars.
const (
	vizBoxW    = 84
	vizBarsW   = 76
	vizBarsH   = 32
	vizSpacing = 2
	vizFrameMs = 33
)

type vizSection struct {
	capture audio.Capture
	spec    *viz.Spectrum
	buf     [audio.RingSize]float32
	lastN   uint64

	running bool
	ticking bool
	visible bool // bars drawn last frame

	zone   *zone
	rect   win.Rect
	color  uint32 // bar color in use
	accent uint32 // Windows accent (tertiary), the fallback
}

func (v *vizSection) init(a *App) {
	v.spec = viz.New(a.cfg.Visualizer.Bars)
	v.capture.OnData = func() { win.PostMessage(a.host, msgAudio, 0, 0) }
}

func (v *vizSection) enabled(a *App) bool       { return a.cfg.Visualizer.Enabled }
func (v *vizSection) width(a *App) int32        { return a.scale(vizBoxW) }
func (v *vizSection) place(z *zone, r win.Rect) { v.zone, v.rect = z, r }

func (v *vizSection) start() {
	v.spec.SetBars(app.cfg.Visualizer.Bars)
	if v.running {
		return
	}
	v.running = true
	v.capture.SetIdle(true) // the first packet wakes the render tick
	v.capture.Start()
}

func (v *vizSection) stop() {
	if !v.running {
		return
	}
	v.running = false
	v.capture.Stop()
	v.setTicking(false)
	clear(v.spec.Bars)
	v.visible = false
}

func (v *vizSection) setTicking(on bool) {
	if on == v.ticking {
		return
	}
	if on {
		win.SetTimer(app.host, timerRender, vizFrameMs)
	} else {
		win.KillTimer(app.host, timerRender)
	}
	v.ticking = on
}

// wake starts the render tick after audio arrived.
func (v *vizSection) wake(a *App) {
	if v.running {
		v.setTicking(true)
	}
}

// tick is one 33 ms frame. It stops itself once the bars have decayed and
// no new audio arrives; the capture thread wakes it again.
func (v *vizSection) tick(a *App) {
	if !v.running {
		v.setTicking(false)
		return
	}
	n := v.capture.Latest(&v.buf)
	fresh := n != v.lastN
	var visible bool
	if fresh && v.capture.SampleRate() > 0 {
		v.lastN = n
		c := &a.cfg.Visualizer
		visible = v.spec.Update(&v.buf, v.capture.SampleRate(), c.Sensitivity, c.Peak)
	} else {
		visible = v.spec.Decay()
	}
	if visible || v.visible {
		v.redraw(a)
	}
	v.visible = visible
	if !visible && !fresh {
		v.setTicking(false)
		v.capture.SetIdle(true)
	}
}

func (v *vizSection) redraw(a *App) {
	if v.zone != nil && a.begin(v.zone) {
		v.draw(a)
		a.present(v.zone)
	}
}

// draw renders the bars into the zone surface (no present).
func (v *vizSection) draw(a *App) {
	r := v.rect
	if v.zone == nil || r.W() <= 0 {
		return
	}
	surf := &v.zone.surf
	surf.clearRect(r)
	c := &a.cfg.Visualizer
	bw, bh := a.scale(vizBarsW), a.scale(vizBarsH)
	x := r.Left + (r.W()-bw)/2
	y := r.Top + (r.H()-bh)/2
	var baseline int32
	if c.Baseline && !(c.BaselineAutoHide && !v.visible && !anyBar(v.spec.Bars)) {
		baseline = max(1, (4*a.scale(100)+150)/300) // 4 px at 3x supersampling
	}
	radius := float32(a.scale(200)) / 100 / max(1, float32(c.Bars)/10)
	viz.Draw(surf.pix, surf.w, x, y, bw, bh, v.spec.Bars, viz.Style{
		Color: v.color, Centered: c.Centered, Baseline: baseline,
		Spacing: a.scale(vizSpacing), Radius: radius,
	})
}

// readColor picks the bar color: config "#RRGGBB", else the album art color
// (FluentFlyout SavedDominantColors), else the Windows accent palette entry
// WPF UI calls SystemAccentColorTertiary (Light3 on a dark taskbar, Dark3 on
// a light one).
func (v *vizSection) readColor(a *App) {
	v.accent = 0x99EBFF // Windows default blue, Light3
	var pal [32]byte
	if n, ok := win.RegBinary(`Software\Microsoft\Windows\CurrentVersion\Explorer\Accent`, "AccentPalette", pal[:]); ok && n >= 28 {
		i := 0 // Light3
		if a.light {
			i = 6 // Dark3
		}
		v.accent = uint32(pal[i*4])<<16 | uint32(pal[i*4+1])<<8 | uint32(pal[i*4+2])
	}
	v.color = v.accent
	if c := strings.TrimPrefix(a.cfg.Visualizer.Color, "#"); len(c) == 6 {
		if n, err := strconv.ParseUint(c, 16, 32); err == nil {
			v.color = uint32(n)
			return
		}
	}
	if a.cfg.Visualizer.AlbumColor {
		if c, ok := a.media.albumColor(a); ok {
			v.color = c
		}
	}
}

func anyBar(bars []float32) bool {
	for _, b := range bars {
		if b > 0 {
			return true
		}
	}
	return false
}
