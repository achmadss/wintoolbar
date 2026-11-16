package audio

import (
	"os"
	"testing"
	"time"
)

func TestCaptureLive(t *testing.T) {
	if os.Getenv("WINTOOLBAR_LIVE") == "" {
		t.Skip("needs an audio endpoint: set WINTOOLBAR_LIVE=1 to run")
	}
	var c Capture
	c.Start()
	time.Sleep(1500 * time.Millisecond)
	var buf [RingSize]float32
	n := c.Latest(&buf)
	rate := c.SampleRate()
	c.Stop()
	if rate == 0 {
		t.Fatal("capture did not start (no sample rate)")
	}
	t.Logf("rate=%d samples=%d peak=%.3f", rate, n, Peak(buf[:]))
}
