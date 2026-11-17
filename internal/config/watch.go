package config

import (
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"wintoolbar/internal/win"
)

// debounceDelay is how long Watch waits after the last notification: one save
// bursts several notifications, and reloading once per burst is enough.
const debounceDelay = 300 * time.Millisecond

// notifyFilter covers both ways a save changes the file: Save rewrites
// config.json in place (last write), and a rename over it (file name).
const notifyFilter = win.FILE_NOTIFY_CHANGE_LAST_WRITE | win.FILE_NOTIFY_CHANGE_FILE_NAME

// notifyBuf is one ReadDirectoryChangesW buffer. 8 KiB holds a long burst of
// FILE_NOTIFY_INFORMATION records, so a burst does not overflow to an error.
const notifyBuf = 8 << 10

// Watch calls fn when %APPDATA%\wintoolbar\config.json changes on disk, so the
// widget can reload it without a restart.
//
// fn runs on the watcher's own goroutine, so it must not block: post the work
// to the widget loop instead. It is never called before the first change, and
// a burst of changes yields one call debounceDelay after the last one.
//
// stop ends the watch, drops a pending call and closes the directory handle. It
// blocks until the goroutine has finished, is safe to call twice and may be
// called from fn. When the config directory cannot be opened, Watch returns a
// no-op stop and the widget keeps working without live reload.
func Watch(fn func()) (stop func()) {
	p, err := Path()
	if err != nil {
		return func() {}
	}
	return watchDir(filepath.Dir(p), fn)
}

// watchDir is Watch for an explicit directory, so tests avoid the real config
// directory. A no-op stop means watching is unavailable.
func watchDir(dir string, fn func()) func() {
	if fn == nil {
		return func() {}
	}
	h, err := win.OpenDir(dir)
	if err != nil {
		return func() {}
	}
	stopEv := win.CreateEvent()
	if stopEv == 0 {
		win.CloseHandle(h) // no event, so no way to wake the reader
		return func() {}
	}
	w := &watch{
		h:      h,
		stopEv: stopEv,
		fn:     fn,
		hit:    make(chan struct{}, 1),
		halt:   make(chan struct{}),
		gone:   make(chan struct{}),
	}
	go w.read()
	go w.debounce()
	return w.stop
}

type watch struct {
	h      uintptr
	stopEv uintptr
	fn     func()
	hit    chan struct{} // a change arrived; capacity 1 coalesces bursts
	halt   chan struct{} // closed by stop
	gone   chan struct{} // closed once read closed the directory handle
	once   sync.Once
}

// read blocks in ReadDirectoryChangesW, so the process stays idle until the
// file changes. It owns the directory handle and closes it on the way out.
func (w *watch) read() {
	defer close(w.gone)
	defer win.CloseHandle(w.h)
	ev := win.CreateEvent()
	if ev == 0 {
		return
	}
	defer win.CloseHandle(ev)
	// Auto-reset event: the kernel signals it when the pending read completes.
	ov := syscall.Overlapped{HEvent: syscall.Handle(ev)}
	buf := make([]byte, notifyBuf)
	for {
		if err := win.StartChanges(w.h, &ov, buf, notifyFilter); err != nil {
			return
		}
		if win.WaitAny(ev, w.stopEv) != 0 {
			// Stop signalled, or the wait failed: cancel our own read and drain
			// it, because closing a handle under a pending read is undefined.
			win.CancelChanges(w.h)
			win.ChangeResult(w.h, &ov, true)
			return
		}
		if _, err := win.ChangeResult(w.h, &ov, false); err != nil {
			return
		}
		select {
		case w.hit <- struct{}{}:
		default: // a wakeup is already queued
		}
	}
}

// debounce turns a burst of wakeups into one fn call. The timer exists only
// while a burst is pending, so nothing ticks when the file is quiet.
func (w *watch) debounce() {
	var t *time.Timer
	var due <-chan time.Time
	for {
		select {
		case <-w.halt:
			if t != nil {
				t.Stop()
			}
			return
		case <-w.hit:
			if t == nil {
				t = time.NewTimer(debounceDelay)
			} else {
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(debounceDelay)
			}
			due = t.C // re-arm: after a fire, the next burst needs the channel back
		case <-due:
			due = nil
			w.fn()
		}
	}
}

// stop is the handle returned by Watch.
func (w *watch) stop() {
	w.once.Do(func() {
		close(w.halt)
		win.SetEvent(w.stopEv)
		<-w.gone
		win.CloseHandle(w.stopEv)
	})
}
