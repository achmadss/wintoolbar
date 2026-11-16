package media

import (
	"bytes"
	"image"
	_ "image/jpeg" // SMTC thumbnails are JPEG or PNG
	_ "image/png"
	"math"
)

// ArtSize is the edge of the square, center-cropped art kept in memory.
// The largest cover drawn is 36 * 0.9 logical px at 250 % = 81 px.
const ArtSize = 96

// Art is a decoded, center-cropped square thumbnail.
type Art struct {
	Pix  []uint32 // ArtSize*ArtSize, 0xFFRRGGBB (opaque)
	Peak uint32   // dominant color 0xRRGGBB before theme adjustment
	Has  bool     // Peak is valid (the image has a vivid color)
}

// decodeArt decodes image bytes into a cropped ArtSize square.
func decodeArt(data []byte) (*Art, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	if side <= 0 {
		return nil, image.ErrFormat
	}
	// UniformToFill: crop the center square, then box-sample down to ArtSize.
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	a := &Art{Pix: make([]uint32, ArtSize*ArtSize)}
	for ty := range ArtSize {
		sy0 := y0 + ty*side/ArtSize
		sy1 := max(y0+(ty+1)*side/ArtSize, sy0+1)
		for tx := range ArtSize {
			sx0 := x0 + tx*side/ArtSize
			sx1 := max(x0+(tx+1)*side/ArtSize, sx0+1)
			var r, g, bl, n uint32
			// Sample at most a 4x4 grid per target pixel.
			stepY := max((sy1-sy0)/4, 1)
			stepX := max((sx1-sx0)/4, 1)
			for sy := sy0; sy < sy1; sy += stepY {
				for sx := sx0; sx < sx1; sx += stepX {
					cr, cg, cb, _ := img.At(sx, sy).RGBA()
					r, g, bl, n = r+cr>>8, g+cg>>8, bl+cb>>8, n+1
				}
			}
			a.Pix[ty*ArtSize+tx] = 0xFF000000 | (r/n)<<16 | (g/n)<<8 | bl/n
		}
	}
	a.Peak, a.Has = dominant(a.Pix)
	return a, nil
}

// dominant ports FluentFlyout BitmapHelper.GetDominantColors(1): a 16-level
// RGB histogram over vivid, mid-lightness pixels, weighted by chroma
// squared. Every 4th pixel is sampled instead of a random 10 %, so the
// result is stable.
func dominant(pix []uint32) (uint32, bool) {
	const quantBits = 4
	const bins = 1 << quantBits
	var hist [bins * bins * bins]int
	for i := 0; i < len(pix); i += 4 {
		p := pix[i]
		r8, g8, b8 := p>>16&0xFF, p>>8&0xFF, p&0xFF
		r, g, b := float32(r8)/255, float32(g8)/255, float32(b8)/255
		mx, mn := max(r, g, b), min(r, g, b)
		chroma := mx - mn
		light := (mx + mn) / 2
		if chroma < 0.15 || light < 0.15 || light > 0.85 {
			continue
		}
		hist[(r8>>4)*bins*bins+(g8>>4)*bins+b8>>4] += int(chroma * chroma * 100)
	}
	peak := 0
	for i := range hist {
		if hist[i] > hist[peak] {
			peak = i
		}
	}
	if hist[peak] == 0 {
		return 0, false
	}
	c := func(i int) uint32 { return uint32(i<<(8-quantBits) + 1<<(8-quantBits-1)) }
	return c(peak/(bins*bins))<<16 | c(peak/bins%bins)<<8 | c(peak%bins), true
}

// ThemeColor adjusts a dominant color like FluentFlyout: on a dark taskbar,
// lift luminance to at least 0.75 (linear) and desaturate 35 %; on a light
// taskbar only desaturate.
func ThemeColor(rgb uint32, dark bool) uint32 {
	lin := func(v uint32) float64 { return math.Pow(float64(v)/255, 2.2) }
	gam := func(v float64) uint32 {
		return uint32(min(max(math.Pow(max(v, 0), 1/2.2)*255, 0), 255))
	}
	r, g, b := lin(rgb>>16&0xFF), lin(rgb>>8&0xFF), lin(rgb&0xFF)
	if dark {
		l := 0.2126*r + 0.7152*g + 0.0722*b
		s := max(l, 0.75) / max(l, 0.0001)
		r, g, b = r*s, g*s, b*s
	}
	l := 0.2126*r + 0.7152*g + 0.0722*b
	r += (l - r) * 0.35
	g += (l - g) * 0.35
	b += (l - b) * 0.35
	return gam(r)<<16 | gam(g)<<8 | gam(b)
}
