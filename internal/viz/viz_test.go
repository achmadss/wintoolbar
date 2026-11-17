package viz

import (
	"math"
	"testing"
)

func sine(freq float64, rate int, amp float64) *[N]float32 {
	var s [N]float32
	for i := range s {
		s[i] = float32(amp * math.Sin(2*math.Pi*freq*float64(i)/float64(rate)))
	}
	return &s
}

func TestSineLandsInRightBand(t *testing.T) {
	sp := New(10)
	// Band edges: 40 * 200^(i/10). 1 kHz falls in band 6 (~956..1600 Hz).
	sp.Update(sine(1000, 48000, 0.5), 48000, 2, 3)
	best := 0
	for i, v := range sp.Bars {
		if v > sp.Bars[best] {
			best = i
		}
	}
	if best != 6 {
		t.Fatalf("peak bar = %d, want 6; bars %v", best, sp.Bars)
	}
}

func TestSilenceIsZero(t *testing.T) {
	sp := New(10)
	var z [N]float32
	if sp.Update(&z, 48000, 2, 3) {
		t.Fatalf("silence produced bars %v", sp.Bars)
	}
}

func TestPeakJumpsUpThenFalls(t *testing.T) {
	sp := New(2)
	sp.Bars[0] = 0.5
	if !sp.Hold(33, 32) {
		t.Fatal("fresh peak is not visible")
	}
	if got := sp.Peaks[0]; got != 0.5 {
		t.Fatalf("peak = %v, want the bar height 0.5 at once", got)
	}
	sp.Bars[0] = 0 // silent bar: now the marker falls on its own
	sp.Hold(33, 32)
	// One frame of fall rate, not a slow attack.
	want := float32(0.5) - float32(peakFallPxPerSec)*33/(1000*32)
	if got := sp.Peaks[0]; math.Abs(float64(got-want)) > 1e-5 {
		t.Fatalf("peak = %v, want %v", got, want)
	}
}

func TestPeakFallRate(t *testing.T) {
	sp := New(2)
	sp.Bars[0], sp.Peaks[0] = 1, 1
	sp.Bars[1], sp.Peaks[1] = 1, 1
	sp.Bars[0] = 0 // silent bar: the marker falls alone
	sp.Hold(100, 32)
	// 72 px/s over a 32 px bar is 0.225 of the height per 100 ms.
	want := float32(1) - float32(peakFallPxPerSec)*0.1/32
	if got := sp.Peaks[0]; math.Abs(float64(got-want)) > 1e-5 {
		t.Fatalf("silent peak = %v, want %v", got, want)
	}
	if got := sp.Peaks[1]; got != 1 {
		t.Fatalf("peak above a full bar = %v, want 1", got)
	}
}

func TestPeakDecaysToZero(t *testing.T) {
	sp := New(4)
	for i := range sp.Peaks {
		sp.Peaks[i] = 1
	}
	frames := 0
	for sp.Hold(33, 32) {
		frames++
		if frames > 200 {
			t.Fatalf("peaks still %v after %d frames", sp.Peaks, frames)
		}
	}
	if len(sp.Peaks) != 4 || sp.Peaks[3] != 0 {
		t.Fatalf("peaks = %v, want all zero", sp.Peaks)
	}
}

// drawOne renders a single bar into a w x h surface and returns the pixels.
func drawOne(w, h int32, bar, peak float32, st Style) []uint32 {
	pix := make([]uint32, w*h)
	st.Peaks = []float32{peak}
	Draw(pix, w, 0, 0, w, h, []float32{bar}, st)
	return pix
}

func TestCapIsRoundAndBottomIsFlat(t *testing.T) {
	const w, h = 12, 20 // barW = 11, cap radius 5.5
	pix := drawOne(w, h, 1, 0, Style{Color: 0x60CDFF})
	at := func(x, y int32) uint32 { return pix[y*w+x] }
	corner := at(0, 0) >> 24
	mid := at(5, 0) >> 24
	if mid < 0xC0 {
		t.Fatalf("cap center alpha = %#x, want solid", mid)
	}
	if corner != 0 {
		t.Fatalf("cap corner alpha = %#x, want empty", corner)
	}
	// The bar sits on the baseline, so the bottom corners stay square.
	if a := at(0, h-1) >> 24; a != 0xFF {
		t.Fatalf("bottom corner alpha = %#x, want 0xFF", a)
	}
	// The edge inside the cap row is antialiased, not clipped away.
	if a := at(3, 0) >> 24; a == 0 || a == 0xFF {
		t.Fatalf("cap edge alpha = %#x, want partial coverage", a)
	}
}

func TestGradientRunsFromBaseToTip(t *testing.T) {
	const w, h = 12, 20
	pix := drawOne(w, h, 1, 0, Style{Color: 0x60CDFF})
	at := func(x, y int32) uint32 { return pix[y*w+x] }
	base, tip := at(5, h-1), at(5, 0)
	// Base row keeps the configured colour exactly.
	if base != 0xFF000000|0x60<<16|0xCD<<8|0xFF {
		t.Fatalf("base row = %#x, want 0xFF60CDFF", base)
	}
	// Tip row is partially covered by the cap curve, so un-premultiply it.
	a := tip >> 24
	if a < 0xC0 {
		t.Fatalf("tip alpha = %#x, want a solid cap", a)
	}
	unp := func(c uint32) uint32 { return c * 255 / a }
	baseR, baseG := base>>16&0xFF, base>>8&0xFF
	tipR, tipG := unp(tip>>16&0xFF), unp(tip>>8&0xFF)
	if tipR <= baseR || tipR-baseR > 0x50 {
		t.Fatalf("tip red = %#x, want a subtle lift above base %#x", tipR, baseR)
	}
	// Warm bias: red lifts further than green.
	if tipR-baseR <= tipG-baseG {
		t.Fatalf("red lift %#x does not exceed green lift %#x", tipR-baseR, tipG-baseG)
	}
}

func TestPeakMarkerAboveBar(t *testing.T) {
	const w, h = 12, 20
	// Bar at 20% = 4 px tall, peak at full height.
	pix := drawOne(w, h, 0.2, 1, Style{Color: 0x60CDFF})
	at := func(x, y int32) uint32 { return pix[y*w+x] }
	for y := int32(0); y < peakMarkH; y++ {
		if a := at(5, y) >> 24; a != 0xFF {
			t.Fatalf("marker row %d alpha = %#x, want 0xFF", y, a)
		}
	}
	if at(5, peakMarkH) != 0 {
		t.Fatalf("pixel under marker = %#x, want empty", at(5, peakMarkH))
	}
	if at(5, h-1)>>24 != 0xFF {
		t.Fatal("bar body missing under the marker")
	}
	// A marker level with the bar would notch its cap, so it is not drawn.
	pix = drawOne(w, h, 1, 1, Style{Color: 0x60CDFF})
	if pix[(h-1)*w+5] != 0xFF000000|0x60<<16|0xCD<<8|0xFF {
		t.Fatalf("marker overwrote the bar base: %#x", pix[(h-1)*w+5])
	}
}

func TestPeakMarkerSurvivesSilentBar(t *testing.T) {
	const w, h = 12, 20
	// No bar at all (hidden baseline): the marker still has to be visible.
	pix := drawOne(w, h, 0, 0.5, Style{Color: 0x60CDFF})
	at := func(x, y int32) uint32 { return pix[y*w+x] }
	for y := int32(10); y < 10+peakMarkH; y++ {
		if a := at(5, y) >> 24; a != 0xFF {
			t.Fatalf("marker row %d alpha = %#x, want 0xFF", y, a)
		}
	}
	if at(5, 9) != 0 || at(5, 12) != 0 {
		t.Fatal("marker bled outside its two rows")
	}
}

func TestPeakMarkerMirrorsWhenCentered(t *testing.T) {
	const w, h = 12, 20
	pix := drawOne(w, h, 0.5, 1, Style{Color: 0x60CDFF, Centered: true})
	at := func(x, y int32) uint32 { return pix[y*w+x] }
	for _, y := range []int32{0, 1, h - 1, h - 2} {
		if a := at(5, y) >> 24; a != 0xFF {
			t.Fatalf("marker row %d alpha = %#x, want 0xFF", y, a)
		}
	}
	if at(5, peakMarkH) != 0 {
		t.Fatalf("row %d = %#x, want empty between marker and bar", peakMarkH, at(5, peakMarkH))
	}
}

func TestNoAllocs(t *testing.T) {
	sp := New(10)
	in := sine(440, 48000, 0.3)
	pix := make([]uint32, 80*40)
	n := testing.AllocsPerRun(20, func() {
		sp.Update(in, 48000, 2, 3)
		sp.Hold(33, 32)
		Draw(pix, 80, 0, 0, 80, 40, sp.Bars, Style{Color: 0x60CDFF, Spacing: 2, Peaks: sp.Peaks})
	})
	if n != 0 {
		t.Fatalf("%v allocs per frame", n)
	}
}

func BenchmarkUpdate(b *testing.B) {
	sp := New(10)
	in := sine(440, 48000, 0.3)
	for b.Loop() {
		sp.Update(in, 48000, 2, 3)
	}
}

func BenchmarkDraw(b *testing.B) {
	sp := New(10)
	in := sine(440, 48000, 0.3)
	pix := make([]uint32, 76*32)
	st := Style{Color: 0x60CDFF, Spacing: 2, Peaks: sp.Peaks}
	for b.Loop() {
		sp.Update(in, 48000, 2, 3)
		sp.Hold(33, 32)
		Draw(pix, 76, 0, 0, 76, 32, sp.Bars, st)
	}
}
