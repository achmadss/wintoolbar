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

func TestNoAllocs(t *testing.T) {
	sp := New(10)
	in := sine(440, 48000, 0.3)
	pix := make([]uint32, 80*40)
	n := testing.AllocsPerRun(20, func() {
		sp.Update(in, 48000, 2, 3)
		Draw(pix, 80, 0, 0, 80, 40, sp.Bars, Style{Color: 0x60CDFF, Spacing: 2, Radius: 2})
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
