package viz

// Style controls how bars are drawn.
type Style struct {
	Color    uint32    // 0xRRGGBB
	Centered bool      // bars grow from the middle instead of the bottom
	Baseline int32     // minimum bar height in pixels (0 = none)
	Spacing  int32     // gap between bars in pixels
	Radius   float32   // cap radius clamp; 0 rounds the tip to half the bar width
	Peaks    []float32 // peak-hold markers, same 0..1 scale as the bars (optional)
}

// peakMarkH is the peak-hold marker thickness in pixels.
const peakMarkH = 2

// Draw rasterizes bars into pix (premultiplied BGRA words, row stride
// stride) inside the rect x, y, w, h. Ported from Visualizer.cs DrawBars and
// RasterizeBar: a half-circle cap the width of the bar, a vertical gradient
// from Color at the base to a lighter warm shade at the tip, and a 2 px
// peak-hold marker (Style.Peaks) that trails above each bar. Coverage is
// sampled at pixel centers, as in upstream FluentFlyout d39b56e, so left and
// right edges match. Bars that sit on the bottom keep a flat bottom. It does
// not allocate.
func Draw(pix []uint32, stride int32, x, y, w, h int32, bars []float32, st Style) {
	n := int32(len(bars))
	if n == 0 || w <= 0 || h <= 0 {
		return
	}
	totalSpacing := (n - 1) * st.Spacing
	barW := (w - totalSpacing - 1) / n
	if barW <= 0 {
		return
	}
	used := barW*n + totalSpacing
	offX := x + (w-used)/2
	centerY := y + h/2

	// Gradient endpoints. The tip lifts toward white with a warm bias (red most),
	// so the bars gain depth without leaving the hue. Both ends stay saturated:
	// the dark base keeps a light taskbar readable, the bright tip a dark one.
	br, bg, bb := st.Color>>16&0xFF, st.Color>>8&0xFF, st.Color&0xFF
	tr := br + (255-br)*30/100
	tg := bg + (255-bg)*26/100
	tb := bb + (255-bb)*20/100
	// Markers float above the tip, so lift them once more to stand apart.
	mr := tr + (255-tr)*45/100
	mg := tg + (255-tg)*45/100
	mb := tb + (255-tb)*45/100
	mc := 0xFF000000 | mr<<16 | mg<<8 | mb

	const aa = 1.25
	invAA := float32(1 / aa)

	for i := range n {
		bx := offX + i*(barW+st.Spacing)
		bh := min(max(int32(min(max(bars[i], 0), 1)*float32(h)), st.Baseline), h)
		if bh <= 0 {
			continue
		}
		var top, bottom int32
		if st.Centered {
			top, bottom = centerY-bh/2, centerY+bh/2
		} else {
			top, bottom = y+h-bh, y+h
		}
		// A cap wider than the bar or taller than the bar would clip, so shrink it.
		rad := float32(barW) * 0.5
		if st.Radius > 0 {
			rad = min(rad, st.Radius)
		}
		rad = min(rad, float32(bh)*0.5)
		if rad < 0.5 {
			rad = 0 // too small to see: draw the bar square
		}
		rsq := rad * rad
		il, ir := float32(bx)+rad, float32(bx+barW)-rad
		it := float32(top) + rad
		ib := float32(bottom) // flat bottom: no vertical inset, square corners
		if st.Centered {
			ib = float32(bottom) - rad
		}
		invH := float32(1) / max(float32(bottom-top-1), 1)

		for py := max(top, y); py < min(bottom, y+h); py++ {
			row := pix[py*stride:]
			t := float32(bottom-1-py) * invH // 0 at the base row, 1 at the tip
			gr := br + uint32(float32(tr-br)*t)
			gg := bg + uint32(float32(tg-bg)*t)
			gb := bb + uint32(float32(tb-bb)*t)
			fy := float32(py) + 0.5 // sample at pixel centers (upstream d39b56e: symmetric AA)
			if fy >= it && fy <= ib {
				// Body row: the cap curve cannot reach it, so every pixel is solid.
				c := 0xFF000000 | gr<<16 | gg<<8 | gb
				for px := bx; px < bx+barW; px++ {
					row[px] = c
				}
				continue
			}
			for px := bx; px < bx+barW; px++ {
				fx := float32(px) + 0.5
				cx := min(max(fx, il), ir)
				cy := min(max(fy, it), ib)
				dx, dy := fx-cx, fy-cy
				sdf := (dx*dx + dy*dy - rsq) / (2 * rad)
				alpha := min(0.5-sdf*invAA, 1)
				if alpha <= 0 {
					continue
				}
				a := uint32(alpha * 255)
				row[px] = a<<24 | (gr*a/255)<<16 | (gg*a/255)<<8 | gb*a/255
			}
		}
	}

	// Markers get their own pass: a silent bar with a hidden baseline is not
	// drawn, yet its marker still has to be seen falling away. Hold does the
	// motion, so this only has to place the dash.
	for i := range min(int32(len(st.Peaks)), n) {
		ph := int32(min(max(st.Peaks[i], 0), 1) * float32(h))
		if ph <= 1 {
			continue
		}
		bh := min(max(int32(min(max(bars[i], 0), 1)*float32(h)), st.Baseline), h)
		if ph <= bh+1 {
			continue // level with the tip: a dash there just notches the cap
		}
		bx := offX + i*(barW+st.Spacing)
		lo := y + h - ph // bottom-aligned: the dash sits at the peak height
		if st.Centered {
			lo = centerY - ph/2
		}
		for py := max(lo, y); py < min(lo+peakMarkH, y+h); py++ {
			row := pix[py*stride:]
			for px := bx; px < bx+barW; px++ {
				row[px] = mc
			}
		}
		if st.Centered {
			// Mirror to the lower half so the pair straddles the middle.
			for py := max(centerY+ph/2-peakMarkH, y); py < min(centerY+ph/2, y+h); py++ {
				row := pix[py*stride:]
				for px := bx; px < bx+barW; px++ {
					row[px] = mc
				}
			}
		}
	}
}
