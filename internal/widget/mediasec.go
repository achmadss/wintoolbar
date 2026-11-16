package widget

import (
	"log"
	"syscall"

	"wintoolbar/internal/media"
	"wintoolbar/internal/win"
)

// Media layout from FluentFlyout TaskbarWidgetControl.xaml, in logical px
// before its 0.9 ScaleTransform: 4 px left margin, 36x36 cover with 5 px
// corners, 8 px gap, title and artist (artist at 50 % opacity), 8 px gap,
// 32x32 transport buttons. Width is text + 55, capped at 240, and the
// buttons add 102 (CalculateSize in TaskbarWidgetControl.xaml.cs).
const (
	mPad       = 4
	mCover     = 36
	mCoverR    = 5
	mGap       = 8
	mSlack     = 55 - mPad - mCover - mGap // trailing space FluentFlyout leaves after the text
	mMaxText   = 240 - 58
	mBtn       = 32
	mBtnR      = 4 // WPF UI ControlCornerRadius
	mFontPx    = 12
	mIconPx    = 16
	mLineH     = 16
	mGlyphNote = 0xEC4F // Segoe Fluent Icons: music note

	glyphPrev  = 0xF8AC
	glyphPlay  = 0xF5B0
	glyphPause = 0xF8AE
	glyphNext  = 0xF8AD
	glyphStop  = 0xE73B

	buttonsClass = "wintoolbar.buttons"
)

type mediaSection struct {
	w     *media.Watcher
	state media.State

	zone *zone
	rect win.Rect

	font     uintptr // title and artist
	iconFont uintptr // transport and placeholder glyphs
	noteFont uintptr
	title16  []uint16
	artist16 []uint16
	textW    int32

	// Scaled, rounded cover in premultiplied ARGB.
	cover     []uint32
	coverID   uint64
	coverSize int32

	// Transport buttons: their own layered window, the only hit-test target.
	btnHwnd  uintptr
	btnBound win.Rect // taskbar client coordinates
	btnSurf  surface
	glyph    [1]uint16
	hover    int // button index under the cursor, -1 none
	pressed  int
	tracking bool
}

// m scales FluentFlyout logical px, including its 0.9 ScaleTransform.
func (a *App) m(v int32) int32 { return (v*9*int32(a.dpi) + 480) / 960 }

func (s *mediaSection) enabled(a *App) bool { return a.cfg.Media.Enabled }

func (s *mediaSection) place(z *zone, r win.Rect) { s.zone, s.rect = z, r }

func (s *mediaSection) start(a *App) {
	if s.w != nil {
		return
	}
	s.hover, s.pressed = -1, -1
	s.w = media.Start(func() { win.PostMessage(a.host, msgMedia, 0, 0) })
}

func (s *mediaSection) stop() {
	if s.w == nil {
		return
	}
	s.w.Close()
	s.w = nil
	s.state = media.State{}
	s.destroyButtons()
}

var transport = [...]media.Command{media.Previous, media.PlayPause, media.Next, media.Stop}

// buttons lists the visible transport buttons in order (no allocation).
func (s *mediaSection) buttons(a *App) []media.Command {
	switch {
	case !a.cfg.Media.Controls:
		return nil
	case a.cfg.Media.Stop:
		return transport[:4]
	}
	return transport[:3]
}

// measure rebuilds fonts and the text width for the current DPI and text.
func (s *mediaSection) measure(a *App) {
	for _, f := range []*uintptr{&s.font, &s.iconFont, &s.noteFont} {
		if *f != 0 {
			win.DeleteObject(*f)
			*f = 0
		}
	}
	s.font = win.CreateFont(-a.m(mFontPx), win.FW_NORMAL, "Segoe UI Variable Text")
	s.iconFont = win.CreateFont(-a.m(mIconPx), win.FW_NORMAL, "Segoe Fluent Icons")
	s.noteFont = win.CreateFont(-a.m(24), win.FW_NORMAL, "Segoe Fluent Icons")
	s.measureText(a)
}

func (s *mediaSection) measureText(a *App) {
	title, artist := s.state.Title, s.state.Artist
	if title == "" {
		title = "-"
	}
	if artist == "" {
		artist = "-"
	}
	s.title16, _ = syscall.UTF16FromString(title)
	s.artist16, _ = syscall.UTF16FromString(artist)
	s.title16 = s.title16[:len(s.title16)-1]
	s.artist16 = s.artist16[:len(s.artist16)-1]
	dc := win.CreateCompatibleDC(0)
	old := win.SelectObject(dc, s.font)
	w := max(win.TextExtent(dc, s.title16).CX, win.TextExtent(dc, s.artist16).CX)
	win.SelectObject(dc, old)
	win.DeleteDC(dc)
	s.textW = min(w, a.m(mMaxText))
}

// width is 0 without an active session, which hides the section.
func (s *mediaSection) width(a *App) int32 {
	if !s.state.Active {
		return 0
	}
	w := a.m(mPad+mCover+mGap+mSlack) + s.textW
	if n := int32(len(s.buttons(a))); n > 0 {
		w += a.m(mGap) + n*a.m(mBtn)
	}
	return w
}

// onState applies a new snapshot from the watcher.
func (s *mediaSection) onState(a *App) {
	if s.w == nil {
		return
	}
	old := s.state
	s.state = s.w.State()
	if old.Title != s.state.Title || old.Artist != s.state.Artist || old.Active != s.state.Active {
		s.measureText(a)
	}
	if old.ArtID != s.state.ArtID || old.Title != s.state.Title {
		a.scheduleTrim() // track change: thumbnail decode and WinRT calls touched extra pages
	}
	a.vis.readColor(a)
	if moved := a.layout(false); s.zone != nil && !moved[s.zone.id] {
		a.renderZone(s.zone)
	}
	s.syncButtons(a)
	s.drawButtons(a, -1)
	if a.vis.zone != nil && !a.vis.ticking {
		a.vis.redraw(a) // new album color while idle
	}
}

// accent returns the album color (theme-adjusted) or the Windows accent.
func (s *mediaSection) albumColor(a *App) (uint32, bool) {
	if s.state.Active && s.state.Art != nil && s.state.Art.Has {
		return media.ThemeColor(s.state.Art.Peak, !a.light), true
	}
	return 0, false
}

func (s *mediaSection) draw(a *App) {
	r := s.rect
	if s.zone == nil || r.W() <= 0 || !s.state.Active {
		return
	}
	surf := &s.zone.surf
	surf.clearRect(r)
	mid := r.Top + r.H()/2

	// Cover, nudged down 1 px like the XAML's -2 bottom margin.
	cs := a.m(mCover)
	cover := win.Rect{Left: r.Left + a.m(mPad), Top: mid - cs/2 + a.m(1)}
	cover.Right, cover.Bottom = cover.Left+cs, cover.Top+cs
	if s.state.Art != nil {
		s.drawCover(a, surf, cover)
	} else {
		color, ok := s.albumColor(a)
		if !ok {
			color = a.vis.accent
		}
		old := win.SelectObject(surf.dc, s.noteFont)
		s.glyph[0] = mGlyphNote
		cr := cover
		win.DrawText(surf.dc, s.glyph[:], &cr, win.DT_SINGLELINE|win.DT_VCENTER|win.DT_NOPREFIX|1 /*DT_CENTER*/)
		win.SelectObject(surf.dc, old)
		surf.colorize(cover, color)
	}

	// Title above artist, both ellipsized to the text column.
	tl := cover.Right + a.m(mGap)
	tr := tl + s.textW
	line := a.m(mLineH)
	old := win.SelectObject(surf.dc, s.font)
	const f = win.DT_SINGLELINE | win.DT_VCENTER | win.DT_NOPREFIX | 0x8000 /*DT_END_ELLIPSIS*/
	tRect := win.Rect{Left: tl, Top: mid - line, Right: tr, Bottom: mid}
	aRect := win.Rect{Left: tl, Top: mid - a.m(1), Right: tr, Bottom: mid - a.m(1) + line}
	win.DrawText(surf.dc, s.title16, &tRect, f)
	win.SelectObject(surf.dc, old)
	surf.colorize(tRect, a.textColor())
	old = win.SelectObject(surf.dc, s.font)
	win.DrawText(surf.dc, s.artist16, &aRect, f)
	win.SelectObject(surf.dc, old)
	surf.colorizeAlpha(aRect, a.textColor(), 128)
}

// drawCover scales the art (bilinear) into a rounded cover, cached per art and size.
func (s *mediaSection) drawCover(a *App, surf *surface, r win.Rect) {
	size := r.W()
	if s.coverID != s.state.ArtID || s.coverSize != size || s.cover == nil {
		s.cover = scaleArt(s.cover, s.state.Art, size, float32(a.m(mCoverR)))
		s.coverID, s.coverSize = s.state.ArtID, size
	}
	for y := range size {
		dy := r.Top + y
		if dy < 0 || dy >= surf.h {
			continue
		}
		copy(surf.pix[dy*surf.w+r.Left:dy*surf.w+r.Left+size], s.cover[y*size:(y+1)*size])
	}
}

func scaleArt(dst []uint32, art *media.Art, size int32, rad float32) []uint32 {
	n := int(size * size)
	if cap(dst) < n {
		dst = make([]uint32, n)
	}
	dst = dst[:n]
	box := win.Rect{Right: size, Bottom: size}
	k := float32(media.ArtSize) / float32(size)
	for y := range size {
		sy := max((float32(y)+0.5)*k-0.5, 0)
		y0 := min(int(sy), media.ArtSize-1)
		y1 := min(y0+1, media.ArtSize-1)
		fy := sy - float32(y0)
		for x := range size {
			sx := max((float32(x)+0.5)*k-0.5, 0)
			x0 := min(int(sx), media.ArtSize-1)
			x1 := min(x0+1, media.ArtSize-1)
			fx := sx - float32(x0)
			p00, p01 := art.Pix[y0*media.ArtSize+x0], art.Pix[y0*media.ArtSize+x1]
			p10, p11 := art.Pix[y1*media.ArtSize+x0], art.Pix[y1*media.ArtSize+x1]
			ch := func(sh uint) float32 {
				c00, c01 := float32(p00>>sh&0xFF), float32(p01>>sh&0xFF)
				c10, c11 := float32(p10>>sh&0xFF), float32(p11>>sh&0xFF)
				return (c00*(1-fx)+c01*fx)*(1-fy) + (c10*(1-fx)+c11*fx)*fy
			}
			cov := roundCoverage(x, y, box, rad)
			a := uint32(cov * 255)
			dst[y*size+x] = a<<24 | uint32(ch(16)*cov)<<16 | uint32(ch(8)*cov)<<8 | uint32(ch(0)*cov)
		}
	}
	return dst
}

// --- transport buttons window ---

// syncButtons creates, moves or destroys the buttons window.
func (s *mediaSection) syncButtons(a *App) {
	btns := s.buttons(a)
	if s.zone == nil || !win.IsWindow(s.zone.hwnd) || !s.state.Active || len(btns) == 0 || s.rect.W() <= 0 {
		s.destroyButtons()
		return
	}
	if !win.IsWindow(s.btnHwnd) || win.GetParent(s.btnHwnd) != a.taskbar {
		s.destroyButtons()
		h, err := win.CreateWindowEx(win.WS_EX_LAYERED|win.WS_EX_NOACTIVATE|win.WS_EX_TOOLWINDOW,
			win.UTF16Ptr(buttonsClass), win.UTF16Ptr("wintoolbar"),
			win.WS_CHILD|win.WS_CLIPSIBLINGS, 0, 0, 1, 1, a.taskbar, 0, a.inst)
		if err != nil {
			log.Printf("create buttons window: %v", err)
			return
		}
		s.btnHwnd = h
		s.btnBound = win.Rect{}
	}
	bs := a.m(mBtn)
	n := int32(len(btns))
	right := s.zone.bounds.Left + s.rect.Right
	mid := s.zone.bounds.Top + s.rect.Top + s.rect.H()/2
	nb := win.Rect{Left: right - n*bs, Top: mid - bs/2, Right: right, Bottom: mid - bs/2 + bs}
	if nb != s.btnBound {
		s.btnBound = nb
		// HWND_TOP keeps the buttons above their zone window.
		win.SetWindowPos(s.btnHwnd, 0, nb.Left, nb.Top, nb.W(), nb.H(), win.SWP_NOACTIVATE|win.SWP_SHOWWINDOW)
		s.drawButtons(a, -1)
	}
}

func (s *mediaSection) destroyButtons() {
	if win.IsWindow(s.btnHwnd) {
		win.DestroyWindow(s.btnHwnd)
	}
	s.btnHwnd = 0
	s.btnBound = win.Rect{}
	s.btnSurf.free()
	s.hover, s.pressed, s.tracking = -1, -1, false
}

// drawButtons repaints one button (or all with only = -1) and pushes only
// that rect to the window.
func (s *mediaSection) drawButtons(a *App, only int) {
	if !win.IsWindow(s.btnHwnd) {
		return
	}
	btns := s.buttons(a)
	w, h := s.btnBound.W(), s.btnBound.H()
	if !s.btnSurf.ensure(w, h) {
		return
	}
	bs := a.m(mBtn)
	dirty := win.Rect{Right: w, Bottom: h}
	if only >= 0 {
		dirty = win.Rect{Left: int32(only) * bs, Right: int32(only+1) * bs, Bottom: h}
	}
	surf := &s.btnSurf
	surf.clearRect(dirty)
	old := win.SelectObject(surf.dc, s.iconFont)
	for i, c := range btns {
		if only >= 0 && i != only {
			continue
		}
		r := win.Rect{Left: int32(i) * bs, Right: int32(i+1) * bs, Bottom: h}
		var enabled bool
		switch c {
		case media.Previous:
			s.glyph[0], enabled = glyphPrev, s.state.CanPrev
		case media.PlayPause:
			s.glyph[0], enabled = glyphPlay, s.state.CanPlayPause
			if s.state.Playing {
				s.glyph[0] = glyphPause
			}
		case media.Next:
			s.glyph[0], enabled = glyphNext, s.state.CanNext
		case media.Stop:
			s.glyph[0], enabled = glyphStop, s.state.CanStop
		}
		gr := r
		win.DrawText(surf.dc, s.glyph[:], &gr, win.DT_SINGLELINE|win.DT_VCENTER|win.DT_NOPREFIX|1 /*DT_CENTER*/)
		opacity := uint32(255)
		if !enabled {
			opacity = 100
		}
		win.GdiFlush()
		surf.colorizeAlpha(r, a.textColor(), opacity)

		// Hover/pressed fill under the icon (WPF UI Transparent button:
		// SubtleFillColorSecondary / Tertiary).
		var fill uint32
		switch {
		case enabled && s.pressed == i && s.hover == i:
			fill = 0x0A
			if a.light {
				fill = 0x06
			}
		case enabled && s.hover == i:
			fill = 0x0F
			if a.light {
				fill = 0x09
			}
		}
		if fill != 0 {
			argb := fill << 24
			if !a.light {
				argb |= fill<<16 | fill<<8 | fill // premultiplied white
			}
			surf.fillRoundUnder(r, argb, float32(a.m(mBtnR)))
		}
		// Alpha 1 everywhere else so the whole button stays hit-testable
		// (fully transparent layered pixels pass clicks through).
		surf.fillRoundUnder(r, 0x01000000, 0)
	}
	win.SelectObject(surf.dc, old)
	size := win.Size{CX: w, CY: h}
	if err := win.UpdateLayeredWindowDirty(s.btnHwnd, &size, surf.dc, &dirty); err != nil {
		log.Printf("UpdateLayeredWindowIndirect: %v", err)
	}
}

func (s *mediaSection) buttonAt(a *App, lp uintptr) int {
	x := int32(int16(lp & 0xFFFF))
	i := int(x / max(a.m(mBtn), 1))
	if x < 0 || i >= len(s.buttons(a)) {
		return -1
	}
	return i
}

func (s *mediaSection) setHover(a *App, i int) {
	if i == s.hover {
		return
	}
	prev := s.hover
	s.hover = i
	if prev >= 0 {
		s.drawButtons(a, prev)
	}
	if i >= 0 {
		s.drawButtons(a, i)
	}
}

func (a *App) buttonsProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	s := &a.media
	switch msg {
	case win.WM_NCHITTEST:
		return win.HTCLIENT
	case win.WM_MOUSEACTIVATE:
		return win.MA_NOACTIVATE
	case win.WM_MOUSEMOVE:
		if !s.tracking {
			win.TrackMouseLeave(h)
			s.tracking = true
		}
		s.setHover(a, s.buttonAt(a, lp))
		return 0
	case win.WM_MOUSELEAVE:
		s.tracking = false
		s.pressed = -1
		s.setHover(a, -1)
		return 0
	case win.WM_LBUTTONDOWN:
		s.pressed = s.buttonAt(a, lp)
		if s.pressed >= 0 {
			s.drawButtons(a, s.pressed)
		}
		return 0
	case win.WM_LBUTTONUP:
		i := s.buttonAt(a, lp)
		p := s.pressed
		s.pressed = -1
		if p >= 0 {
			s.drawButtons(a, p)
		}
		if i >= 0 && i == p && s.w != nil {
			s.w.Send(s.buttons(a)[i])
		}
		return 0
	case win.WM_DESTROY:
		if h == s.btnHwnd {
			s.btnHwnd = 0
		}
		return 0
	}
	return win.DefWindowProc(h, msg, wp, lp)
}
