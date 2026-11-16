package netmon

import (
	"fmt"
	"strings"
	"syscall"
	"time"

	"wintoolbar/internal/win"
)

// virtualFragments marks adapters that are not physical links. Matched
// case-insensitively against alias and description.
var virtualFragments = [...]string{
	"vethernet", "hyper-v", "vmware", "virtualbox", "vbox", "wsl", "docker",
	"vpn", "tap-windows", "tap adapter", "wintun", "wireguard", "tailscale",
	"zerotier", "hamachi", "wfp", "qos packet scheduler", "wan miniport",
	"kernel debug", "loopback", "pseudo-interface",
}

func isVirtual(alias, desc string) bool {
	alias, desc = strings.ToLower(alias), strings.ToLower(desc)
	for _, f := range virtualFragments {
		if strings.Contains(alias, f) || strings.Contains(desc, f) {
			return true
		}
	}
	return false
}

// Candidate is one adapter considered by Select.
type Candidate struct {
	Luid       uint64
	Alias      string
	Desc       string
	Guid       string
	Type       uint32
	Up         bool
	Hardware   bool
	Filter     bool
	HasGateway bool
	Traffic    float64
}

func (c *Candidate) eligible() bool {
	return c.Up && c.Hardware && !c.Filter &&
		c.Type != win.IF_TYPE_SOFTWARE_LOOPBACK && c.Type != win.IF_TYPE_TUNNEL &&
		!isVirtual(c.Alias, c.Desc)
}

func typeRank(t uint32) int {
	switch t {
	case win.IF_TYPE_ETHERNET_CSMACD:
		return 0
	case win.IF_TYPE_IEEE80211:
		return 2
	default:
		return 1
	}
}

// less ranks a before b: gateway, Ethernet over Wi-Fi, preferred (sticky),
// most traffic, lowest LUID.
func less(a, b *Candidate, preferred uint64) bool {
	if a.HasGateway != b.HasGateway {
		return a.HasGateway
	}
	if ra, rb := typeRank(a.Type), typeRank(b.Type); ra != rb {
		return ra < rb
	}
	if preferred != 0 && (a.Luid == preferred) != (b.Luid == preferred) {
		return a.Luid == preferred
	}
	if a.Traffic != b.Traffic {
		return a.Traffic > b.Traffic
	}
	return a.Luid < b.Luid
}

// Select returns the best adapter. If want is non-empty, the adapter whose
// alias or GUID equals it (case-insensitive) wins regardless of state.
func Select(cands []Candidate, want string, preferred uint64) (Candidate, bool) {
	if want != "" {
		for _, c := range cands {
			if strings.EqualFold(c.Alias, want) || strings.EqualFold(c.Guid, want) {
				return c, true
			}
		}
	}
	best := -1
	for i := range cands {
		if !cands[i].eligible() {
			continue
		}
		if best < 0 || less(&cands[i], &cands[best], preferred) {
			best = i
		}
	}
	if best < 0 {
		return Candidate{}, false
	}
	return cands[best], true
}

// Enumerate reads all interfaces and default routes.
func Enumerate(dst []Candidate) []Candidate {
	gw := map[uint64]bool{}
	win.DefaultRouteLuids(func(l uint64) { gw[l] = true })
	dst = dst[:0]
	win.IfTable(func(r *win.MibIfRow2) {
		dst = append(dst, Candidate{
			Luid:       r.InterfaceLuid,
			Alias:      syscall.UTF16ToString(r.Alias[:]),
			Desc:       syscall.UTF16ToString(r.Description[:]),
			Guid:       guidString(r.InterfaceGuid),
			Type:       r.Type,
			Up:         r.OperStatus == win.IfOperStatusUp,
			Hardware:   r.InterfaceAndOperStatusFlags&win.IfFlagHardwareInterface != 0,
			Filter:     r.InterfaceAndOperStatusFlags&win.IfFlagFilterInterface != 0,
			HasGateway: gw[r.InterfaceLuid],
			Traffic:    float64(r.InOctets) + float64(r.OutOctets),
		})
	})
	return dst
}

func guidString(g [16]byte) string {
	d1 := uint32(g[0]) | uint32(g[1])<<8 | uint32(g[2])<<16 | uint32(g[3])<<24
	d2 := uint16(g[4]) | uint16(g[5])<<8
	d3 := uint16(g[6]) | uint16(g[7])<<8
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		d1, d2, d3, g[8], g[9], g[10], g[11], g[12], g[13], g[14], g[15])
}

// Monitor polls the selected adapter. Call Poll once per second from one goroutine.
type Monitor struct {
	Want string // adapter alias or GUID from config; empty = automatic

	rate     Rate
	row      win.MibIfRow2
	cands    []Candidate
	selected uint64
	alias    string
	polls    int

	RxPerSec, TxPerSec float64
	Valid              bool // false until two samples of one adapter exist
}

// reselectEvery is how many polls pass before the adapter choice is re-checked.
const reselectEvery = 15

// Adapter returns the alias of the sampled adapter, or "".
func (m *Monitor) Adapter() string { return m.alias }

// Poll reads counters and updates the rates. It returns true when the
// displayed values changed.
func (m *Monitor) Poll(now time.Time) bool {
	if m.selected == 0 || m.polls%reselectEvery == 0 {
		m.reselect()
	}
	m.polls++
	oldRx, oldTx, oldValid := m.RxPerSec, m.TxPerSec, m.Valid
	if m.selected == 0 {
		m.RxPerSec, m.TxPerSec, m.Valid = 0, 0, false
		return oldValid
	}
	m.row.InterfaceLuid = m.selected
	m.row.InterfaceIndex = 0
	if !win.GetIfEntry2(&m.row) || m.row.OperStatus != win.IfOperStatusUp {
		// Adapter vanished or went down: pick again next poll.
		m.selected = 0
		m.rate.Reset()
		m.RxPerSec, m.TxPerSec, m.Valid = 0, 0, false
		return oldValid
	}
	rx, tx, ok := m.rate.Sample(m.selected, m.row.InOctets, m.row.OutOctets, now)
	m.RxPerSec, m.TxPerSec, m.Valid = rx, tx, ok
	return ok != oldValid || rx != oldRx || tx != oldTx
}

func (m *Monitor) reselect() {
	m.cands = Enumerate(m.cands)
	c, ok := Select(m.cands, m.Want, m.selected)
	if !ok {
		m.selected, m.alias = 0, ""
		return
	}
	if c.Luid != m.selected {
		m.rate.Reset()
	}
	m.selected, m.alias = c.Luid, c.Alias
}

// Reset forgets the adapter and baseline, for example after the widget was hidden.
func (m *Monitor) Reset() {
	m.selected, m.alias, m.polls = 0, "", 0
	m.rate.Reset()
	m.RxPerSec, m.TxPerSec, m.Valid = 0, 0, false
}
