// Command wintoolbar shows a small widget inside the Windows 11 taskbar.
package main

//go:generate go run ./tools/mkrsrc

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"wintoolbar/internal/config"
	"wintoolbar/internal/widget"
	"wintoolbar/internal/win"
)

func init() {
	// Win32 windows belong to the thread that created them.
	runtime.LockOSThread()
}

func main() {
	// The steady-state heap is about 1 MB. A low GC target and a soft limit
	// keep garbage from thumbnail decoding from inflating the heap.
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(12 << 20)
	setupLog()
	if !win.SingleInstance(`Local\wintoolbar.single-instance`) {
		log.Print("another instance is running")
		return
	}
	win.SetPerMonitorDpiAware()

	if os.Getenv("WINTOOLBAR_MEMLOG") != "" {
		go memLog()
	}
	cfg, err := config.Load()
	if err != nil {
		log.Printf("config: %v (using defaults)", err)
	}
	if err := widget.Run(cfg); err != nil {
		log.Fatal(err)
	}
}

// setupLog writes to %APPDATA%\wintoolbar\wintoolbar.log. A GUI build has no console.
func setupLog() {
	p, err := config.Path()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := os.OpenFile(filepath.Join(filepath.Dir(p), "wintoolbar.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

// memLog writes Go runtime memory figures every 10 s (WINTOOLBAR_MEMLOG=1).
func memLog() {
	for {
		time.Sleep(10 * time.Second)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		ws, priv := win.ProcessMemory()
		log.Printf("mem: ws=%.1fMB private=%.1fMB | go sys=%.1fMB heapInuse=%.1fMB heapIdle=%.1fMB released=%.1fMB stacks=%.1fMB gcSys=%.1fMB other=%.1fMB goroutines=%d",
			mb(ws), mb(priv), mb(m.Sys), mb(m.HeapInuse), mb(m.HeapIdle), mb(m.HeapReleased), mb(m.StackSys), mb(m.GCSys), mb(m.OtherSys+m.MSpanSys+m.MCacheSys+m.BuckHashSys), runtime.NumGoroutine())
	}
}

func mb(v uint64) float64 { return float64(v) / (1 << 20) }
