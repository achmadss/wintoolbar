package viz

// Style controls how bars are drawn.
type Style struct {
	Color    uint32 // 0xRRGGBB
	Centered bool   // bars grow from the middle instead of the bottom
	Baseline int32  // minimum bar height in pixels (0 = none)
	Spacing  int32  // gap between bars in pixels
	Radius   float32
}

// Draw rasterizes bars into pix (premultiplied BGRA words, row stride
// stride) inside the rect x, y, w, h. Ported from Visualizer.cs DrawBars and
// RasterizeBar: rounded corners with a 1.25 px antialiased edge, flat bottom
// for bottom-aligned bars. Coverage is sampled at pixel centers, as in
// upstream FluentFlyout d39b56e, so left and right edges match. It does not allocate.
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

	const aa = 1.25
	invAA := float32(1 / aa)
	cr, cg, cb := st.Color>>16&0xFF, st.Color>>8&0xFF, st.Color&0xFF

	for i := range n {
		bx := offX + i*(barW+st.Spacing)
		bh := max(int32(min(max(bars[i], 0), 1)*float32(h)), st.Baseline)
		if bh <= 0 {
			continue
		}
		var top, bottom int32
		if st.Centered {
			top, bottom = centerY-bh/2, centerY+bh/2
		} else {
			top, bottom = y+h-bh, y+h
		}
		radius := min(st.Radius, float32(min(barW, bh))*0.5)
		rsq := radius * radius
		il, ir := float32(bx)+radius, float32(bx+barW)-radius
		it, ib := float32(top)+radius, float32(bottom)-radius

		for py := max(top, y); py < min(bottom, y+h); py++ {
			row := pix[py*stride:]
			fy := float32(py) + 0.5 // sample at pixel centers (upstream d39b56e: symmetric AA)
			for px := bx; px < bx+barW; px++ {
				fx := float32(px) + 0.5
				alpha := float32(1)
				inCore := (fx >= il && fx <= ir) || (fy >= it && fy <= ib) || (!st.Centered && fy >= ib)
				if !inCore && radius > 0 {
					cx := min(max(fx, il), ir)
					cy := min(max(fy, it), ib)
					dx, dy := fx-cx, fy-cy
					sdf := (dx*dx + dy*dy - rsq) / (2 * radius)
					alpha = min(0.5-sdf*invAA, 1)
					if alpha <= 0 {
						continue
					}
				}
				a := uint32(alpha * 255)
				row[px] = a<<24 | (cr*a/255)<<16 | (cg*a/255)<<8 | cb*a/255
			}
		}
	}
}
