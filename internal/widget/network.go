package widget

import (
	"time"
	"unicode/utf16"

	"wintoolbar/internal/netmon"
	"wintoolbar/internal/win"
)

// Network strip metrics, ported from FluentFlyout SystemMonitorControl:
// 12 px Segoe UI Variable, regular weight for the two stacked lines, a 2 px
// gap after the arrow, 4 px side padding, value right-aligned in a slot as
// wide as the widest value ("999 MB/s"), so arrows never move.
const (
	netFontPx   = 12
	netArrowGap = 2
	netPad      = 4
	netLineH    = 17
)

type netSection struct {
	mon     netmon.Monitor
	running bool
	zone    *zone
	rect    win.Rect // inside the zone window

	font   uintptr
	arrowW int32
	slotW  int32
	w      int32

	// Reused text buffers (no per-frame allocation).
	txt   []byte
	txt16 []uint16
	arrow [1]uint16
}

func (n *netSection) init() {
	n.txt = make([]byte, 0, 32)
	n.txt16 = make([]uint16, 0, 32)
}

func (n *netSection) start(a *App) {
	n.mon.Want = a.cfg.Network.Adapter
	if n.running {
		return
	}
	n.mon.Reset()
	n.mon.Poll(time.Now()) // baseline
	n.running = true
}

func (n *netSection) stop() {
	if !n.running {
		return
	}
	n.running = false
	n.mon.Reset()
}

// measure rebuilds the font and fixed widths for the current DPI and units.
func (n *netSection) measure(a *App) {
	if n.font != 0 {
		win.DeleteObject(n.font)
	}
	n.font = win.CreateFont(-a.scale(netFontPx), win.FW_NORMAL, "Segoe UI Variable Text")

	dc := win.CreateCompatibleDC(0)
	old := win.SelectObject(dc, n.font)
	n.arrowW = max(win.TextExtent(dc, []uint16{'↓'}).CX, win.TextExtent(dc, []uint16{'↑'}).CX)
	widest := netmon.WidestByteRates[:]
	if a.cfg.Network.Bits {
		widest = netmon.WidestBitRates[:]
	}
	n.slotW = 0
	for _, s := range widest {
		n.slotW = max(n.slotW, win.TextExtent(dc, utf16.Encode([]rune(s))).CX)
	}
	win.SelectObject(dc, old)
	win.DeleteDC(dc)
	n.w = 2*a.scale(netPad) + n.arrowW + a.scale(netArrowGap) + n.slotW
}

// rateText formats a rate into n.txt16 without allocating.
func (n *netSection) rateText(bytesPerSec float64, valid, bits bool) []uint16 {
	n.txt = n.txt[:0]
	if valid {
		n.txt = netmon.AppendCompact(n.txt, bytesPerSec, bits)
	} else {
		n.txt = append(n.txt, "—"...)
	}
	n.txt16 = n.txt16[:0]
	for _, r := range string(n.txt) { // range over string(bytes) does not allocate
		n.txt16 = utf16.AppendRune(n.txt16, r)
	}
	return n.txt16
}

func (n *netSection) enabled(a *App) bool       { return a.cfg.Network.Enabled }
func (n *netSection) width(a *App) int32        { return n.w }
func (n *netSection) place(z *zone, r win.Rect) { n.zone, n.rect = z, r }

// draw renders the section into its zone surface (no present).
func (n *netSection) draw(a *App) {
	r := n.rect
	if n.zone == nil || r.W() <= 0 {
		return
	}
	surf := &n.zone.surf
	surf.clearRect(r)
	old := win.SelectObject(surf.dc, n.font)
	pad := a.scale(netPad)
	line := a.scale(netLineH)
	mid := r.Top + r.H()/2
	arrowL, arrowR := r.Left+pad, r.Left+pad+n.arrowW
	valL, valR := arrowR+a.scale(netArrowGap), r.Right-pad
	const f = win.DT_SINGLELINE | win.DT_VCENTER | win.DT_NOPREFIX
	rows := [2]struct {
		arrow uint16
		rate  float64
		top   int32
	}{{'↓', n.mon.RxPerSec, mid - line}, {'↑', n.mon.TxPerSec, mid}}
	for i := range rows {
		row := &rows[i]
		n.arrow[0] = row.arrow
		ar := win.Rect{Left: arrowL, Top: row.top, Right: arrowR, Bottom: row.top + line}
		win.DrawText(surf.dc, n.arrow[:], &ar, f|win.DT_LEFT)
		vr := win.Rect{Left: valL, Top: row.top, Right: valR, Bottom: row.top + line}
		win.DrawText(surf.dc, n.rateText(row.rate, n.mon.Valid, a.cfg.Network.Bits), &vr, f|win.DT_RIGHT)
	}
	win.SelectObject(surf.dc, old)
	surf.colorize(r, a.textColor())
}

// poll samples counters; true when the displayed values changed.
func (n *netSection) poll() bool {
	return n.running && n.mon.Poll(time.Now())
}
