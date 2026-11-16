// Package viz turns audio samples into bar heights and draws the bars.
//
// The math is ported from FluentFlyout Classes/Visualizer.cs: 4096-point FFT
// with a Hamming window (NAudio scaling, forward transform divided by N),
// log-spaced bands from 40 Hz to 8000 Hz, a linear high-band boost, dB
// scaling between sensitivity and peak settings, and fast-attack/slow-decay
// smoothing.
//
// One difference: FluentFlyout ran the FFT over interleaved stereo samples.
// Here channels are downmixed to mono first, so the band edges are the
// real frequencies. The FFT runs once per 33 ms frame over the newest 4096
// samples (sliding window) instead of once per 4096 interleaved samples.
package viz

import (
	"math"
	"math/bits"
)

// N is the FFT length.
const N = 4096

const (
	minFreq = 40.0
	maxFreq = 8000.0
)

// Spectrum holds FFT scratch space and the smoothed bars. Not safe for concurrent use.
type Spectrum struct {
	re, im  [N]float32
	window  [N]float32
	cos     [N / 2]float32
	sin     [N / 2]float32
	rev     [N]uint16
	current []float32
	Bars    []float32 // smoothed 0..1 values
}

// New builds the window and twiddle tables.
func New(bars int) *Spectrum {
	s := &Spectrum{}
	for i := range N {
		// NAudio FastFourierTransform.HammingWindow.
		s.window[i] = float32(0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(N-1)))
		s.rev[i] = uint16(bits.Reverse16(uint16(i)) >> (16 - 12))
	}
	for i := range N / 2 {
		a := -2 * math.Pi * float64(i) / N
		s.cos[i] = float32(math.Cos(a))
		s.sin[i] = float32(math.Sin(a))
	}
	s.SetBars(bars)
	return s
}

// SetBars resizes the bar arrays (allocates; not for the hot path).
func (s *Spectrum) SetBars(n int) {
	n = max(1, min(n, 64))
	if len(s.Bars) == n {
		return
	}
	s.Bars = make([]float32, n)
	s.current = make([]float32, n)
}

// Decay lowers the bars toward zero without new audio. It returns true while any bar is visible.
func (s *Spectrum) Decay() bool {
	any := false
	for i := range s.Bars {
		s.Bars[i] *= 0.8
		if s.Bars[i] > 0.01 {
			any = true
		} else {
			s.Bars[i] = 0
		}
	}
	return any
}

// Update runs the FFT over samples (oldest first) and updates Bars.
// sensitivity and peak are FluentFlyout's 1..3 settings.
// It returns true while any bar is visible (> 0.01).
func (s *Spectrum) Update(samples *[N]float32, sampleRate int, sensitivity, peak int) bool {
	for i := range N {
		j := s.rev[i]
		s.re[j] = samples[i] * s.window[i]
		s.im[j] = 0
	}
	s.fft()

	binHz := float64(sampleRate) / N
	minDb := float32(sensitivity)*-10 - 30
	maxDb := float32(peak)*10 - 30
	count := len(s.Bars)
	ratio := maxFreq / minFreq

	for i := range count {
		startF := minFreq * math.Pow(ratio, float64(i)/float64(count))
		endF := minFreq * math.Pow(ratio, float64(i+1)/float64(count))
		startBin := int(startF / binHz)
		endBin := int(endF / binHz)
		if endBin <= startBin {
			endBin = startBin + 1
		}
		if endBin >= N/2 {
			endBin = N/2 - 1
		}
		var maxAmp float32
		for j := startBin; j < endBin; j++ {
			// NAudio's forward FFT divides by N.
			re, im := s.re[j]/N, s.im[j]/N
			amp := float32(math.Sqrt(float64(re*re + im*im)))
			maxAmp = max(maxAmp, amp)
		}
		progress := float32(i) / float32(count)
		maxAmp *= 1 + progress*75
		maxAmp = max(maxAmp, 0.001)
		db := 20 * float32(math.Log10(float64(maxAmp)))
		s.current[i] = min(max((db-minDb)/(maxDb-minDb), 0), 1)
	}

	any := false
	for i := range count {
		if s.current[i] > s.Bars[i] {
			s.Bars[i] = s.current[i] // jump up
		} else {
			s.Bars[i] = s.Bars[i]*0.8 + s.current[i]*0.2 // fall slowly
		}
		if s.Bars[i] > 0.01 {
			any = true
		}
	}
	return any
}

// fft is an in-place iterative radix-2 FFT over bit-reversed input.
func (s *Spectrum) fft() {
	for size := 2; size <= N; size <<= 1 {
		half := size >> 1
		step := N / size
		for start := 0; start < N; start += size {
			for k := range half {
				wr, wi := s.cos[k*step], s.sin[k*step]
				a, b := start+k, start+k+half
				tr := wr*s.re[b] - wi*s.im[b]
				ti := wr*s.im[b] + wi*s.re[b]
				s.re[b], s.im[b] = s.re[a]-tr, s.im[a]-ti
				s.re[a], s.im[a] = s.re[a]+tr, s.im[a]+ti
			}
		}
	}
}
