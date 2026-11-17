package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// watchDir needs no special privileges: any user can watch their own temp dir.
func TestWatchDebounce(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	hit := make(chan struct{}, 8)
	stop := watchDir(dir, func() { hit <- struct{}{} })
	defer stop()

	select {
	case <-hit:
		t.Fatal("called before the first change")
	case <-time.After(debounceDelay + 100*time.Millisecond):
	}

	for i := range 5 {
		if err := os.WriteFile(file, []byte{byte('0' + i)}, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case <-hit:
	case <-time.After(5 * time.Second):
		t.Fatal("no call after a change")
	}

	select {
	case <-hit:
		t.Fatal("burst not coalesced into one call")
	case <-time.After(debounceDelay + 200*time.Millisecond):
	}

	// A second, separate change must call again: the debounce timer has to be
	// re-armed after every fire.
	if err := os.WriteFile(file, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-hit:
	case <-time.After(5 * time.Second):
		t.Fatal("no call after a later change")
	}
}

func TestWatchStop(t *testing.T) {
	dir := t.TempDir()
	hit := make(chan struct{}, 1)
	stop := watchDir(dir, func() { hit <- struct{}{} })
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(debounceDelay / 3) // let the reader see the change, keep the debounce pending
	stop()
	stop() // twice must not panic or block

	select {
	case <-hit:
		t.Fatal("called after stop")
	case <-time.After(2 * debounceDelay):
	}
}

func TestWatchMissingDir(t *testing.T) {
	stop := watchDir(filepath.Join(t.TempDir(), "absent"), func() { t.Error("called") })
	stop()
}
