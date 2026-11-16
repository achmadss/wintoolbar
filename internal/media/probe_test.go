package media

import (
	"os"
	"testing"
	"time"
)

func TestWatcherLive(t *testing.T) {
	if os.Getenv("WINTOOLBAR_LIVE") == "" {
		t.Skip("needs a media session: set WINTOOLBAR_LIVE=1 to run")
	}
	got := make(chan struct{}, 8)
	w := Start(func() { got <- struct{}{} })
	defer w.Close()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no state within 5 s")
	}
	s := w.State()
	if !s.Active {
		t.Skip("no media session")
	}
	t.Logf("app=%q title=%q artist=%q playing=%v prev=%v next=%v pp=%v stop=%v art=%v",
		s.App, s.Title, s.Artist, s.Playing, s.CanPrev, s.CanNext, s.CanPlayPause, s.CanStop, s.Art != nil)
	if s.Art != nil {
		t.Logf("dominant=%06X has=%v themed(dark)=%06X", s.Art.Peak, s.Art.Has, ThemeColor(s.Art.Peak, true))
	}
}
