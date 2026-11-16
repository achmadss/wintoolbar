package widget

import (
	"math"

	"wintoolbar/internal/win"
)

// surface is a premultiplied 32-bit DIB with a memory DC, reused across frames.
//
// GDI text does not write alpha. Text is drawn white on transparent black and
// colorize turns its grayscale coverage into premultiplied alpha.
type surface struct {
	dc     uintptr
	bmp    uintptr
	oldBmp uintptr
	pix    []uint32
	w, h   int32
}

func (s *surface) ensure(w, h int32) bool {
	if s.bmp != 0 && s.w == w && s.h == h {
		return true
	}
	s.free()
	if w <= 0 || h <= 0 {
		return false
	}
	s.dc = win.CreateCompatibleDC(0)
	s.bmp, s.pix = win.CreateDIB32(w, h)
	if s.bmp == 0 {
		win.DeleteDC(s.dc)
		s.dc = 0
		return false
	}
	s.oldBmp = win.SelectObject(s.dc, s.bmp)
	win.SetBkMode(s.dc, win.TRANSPARENT)
	win.SetTextColor(s.dc, 0xFFFFFF)
	s.w, s.h = w, h
	return true
}

func (s *surface) free() {
	if s.dc != 0 {
		win.SelectObject(s.dc, s.oldBmp)
		win.DeleteDC(s.dc)
	}
	if s.bmp != 0 {
		win.DeleteObject(s.bmp)
	}
	*s = surface{}
}

func (s *surface) clear() {
	clear(s.pix)
}

// colorize converts white GDI text in r into premultiplied rgb (0xRRGGBB).
func (s *surface) colorize(r win.Rect, rgb uint32) { s.colorizeAlpha(r, rgb, 255) }

// colorizeAlpha is colorize with an extra opacity (0..255).
func (s *surface) colorizeAlpha(r win.Rect, rgb uint32, opacity uint32) {
	cr, cg, cb := rgb>>16&0xFF, rgb>>8&0xFF, rgb&0xFF
	win.GdiFlush()
	for y := max(r.Top, 0); y < min(r.Bottom, s.h); y++ {
		row := s.pix[y*s.w : (y+1)*s.w]
		for x := max(r.Left, 0); x < min(r.Right, s.w); x++ {
			p := row[x]
			if p == 0 {
				continue
			}
			// Grayscale antialiasing: channels are equal; take the max to be safe.
			a := max(p>>16&0xFF, p>>8&0xFF, p&0xFF) * opacity / 255
			row[x] = a<<24 | (cr*a/255)<<16 | (cg*a/255)<<8 | cb*a/255
		}
	}
}

// clearRect makes r fully transparent.
func (s *surface) clearRect(r win.Rect) {
	for y := max(r.Top, 0); y < min(r.Bottom, s.h); y++ {
		clear(s.pix[y*s.w+max(r.Left, 0) : y*s.w+min(r.Right, s.w)])
	}
}

// roundCoverage returns the antialiased coverage (0..1) of pixel (x, y) in
// a rounded rect r with corner radius rad.
func roundCoverage(x, y int32, r win.Rect, rad float32) float32 {
	fx, fy := float32(x)+0.5, float32(y)+0.5
	il, ir := float32(r.Left)+rad, float32(r.Right)-rad
	it, ib := float32(r.Top)+rad, float32(r.Bottom)-rad
	cx, cy := min(max(fx, il), ir), min(max(fy, it), ib)
	dx, dy := fx-cx, fy-cy
	if dx == 0 && dy == 0 {
		return 1
	}
	d := float32(math.Sqrt(float64(dx*dx+dy*dy))) - rad
	return min(max(0.5-d, 0), 1)
}

// fillRoundUnder composites a premultiplied ARGB color under the existing
// pixels inside a rounded rect (destination-over).
func (s *surface) fillRoundUnder(r win.Rect, argb uint32, rad float32) {
	ca, cr, cg, cb := argb>>24, argb>>16&0xFF, argb>>8&0xFF, argb&0xFF
	for y := max(r.Top, 0); y < min(r.Bottom, s.h); y++ {
		row := s.pix[y*s.w : (y+1)*s.w]
		for x := max(r.Left, 0); x < min(r.Right, s.w); x++ {
			cov := uint32(roundCoverage(x, y, r, rad) * 255)
			if cov == 0 {
				continue
			}
			p := row[x]
			inv := 255 - p>>24
			k := cov * inv / 255
			row[x] = (p>>24+ca*k/255)<<24 | (p>>16&0xFF+cr*k/255)<<16 | (p>>8&0xFF+cg*k/255)<<8 | (p&0xFF + cb*k/255)
		}
	}
}

// fill sets every pixel in r to a premultiplied ARGB value.
func (s *surface) fill(r win.Rect, argb uint32) {
	for y := max(r.Top, 0); y < min(r.Bottom, s.h); y++ {
		row := s.pix[y*s.w : (y+1)*s.w]
		for x := max(r.Left, 0); x < min(r.Right, s.w); x++ {
			row[x] = argb
		}
	}
}
