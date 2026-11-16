package netmon

import (
	"testing"
	"time"

	"wintoolbar/internal/win"
)

func TestAppendRate(t *testing.T) {
	cases := []struct {
		in   float64
		bits bool
		want string
	}{
		{0, false, "0 B/s"},
		{999, false, "999 B/s"},
		{999.96, false, "1 KB/s"},
		{1024, false, "1 KB/s"},
		{13_000_000, false, "12.4 MB/s"},
		{1536, false, "1.5 KB/s"},
		{0, true, "0 b/s"},
		{125, true, "1 Kb/s"},
		{187_500, true, "1.5 Mb/s"},
		{-1, false, "—"},
	}
	for _, c := range cases {
		if got := string(AppendRate(nil, c.in, c.bits, 1, false)); got != c.want {
			t.Errorf("AppendRate(%v, %v) = %q, want %q", c.in, c.bits, got, c.want)
		}
	}
	buf := make([]byte, 0, 32)
	if n := testing.AllocsPerRun(100, func() { buf = AppendCompact(buf[:0], 13_000_000, false) }); n != 0 {
		t.Errorf("AppendRate allocates %v times", n)
	}
}

func TestRate(t *testing.T) {
	var r Rate
	t0 := time.Now()
	if _, _, ok := r.Sample(1, 100, 50, t0); ok {
		t.Fatal("first sample must be baseline only")
	}
	rx, tx, ok := r.Sample(1, 1100, 550, t0.Add(2*time.Second))
	if !ok || rx != 500 || tx != 250 {
		t.Fatalf("got %v %v %v", rx, tx, ok)
	}
	// Counter reset: no negative rate.
	rx, _, ok = r.Sample(1, 10, 10, t0.Add(3*time.Second))
	if !ok || rx != 0 {
		t.Fatalf("rollback got %v %v", rx, ok)
	}
	// Adapter switch: baseline again.
	if _, _, ok := r.Sample(2, 5000, 5000, t0.Add(4*time.Second)); ok {
		t.Fatal("adapter switch must rebaseline")
	}
}

func TestSelect(t *testing.T) {
	wifi := Candidate{Luid: 1, Alias: "Wi-Fi", Type: win.IF_TYPE_IEEE80211, Up: true, Hardware: true, HasGateway: true}
	eth := Candidate{Luid: 2, Alias: "Ethernet", Type: win.IF_TYPE_ETHERNET_CSMACD, Up: true, Hardware: true}
	veth := Candidate{Luid: 3, Alias: "vEthernet (WSL)", Type: win.IF_TYPE_ETHERNET_CSMACD, Up: true, Hardware: true, HasGateway: true}
	down := Candidate{Luid: 4, Alias: "Ethernet 2", Type: win.IF_TYPE_ETHERNET_CSMACD, Hardware: true, HasGateway: true}

	c, ok := Select([]Candidate{eth, wifi, veth, down}, "", 0)
	if !ok || c.Luid != 1 {
		t.Fatalf("gateway must win, got %+v", c)
	}
	eth.HasGateway = true
	c, _ = Select([]Candidate{wifi, eth}, "", 0)
	if c.Luid != 2 {
		t.Fatalf("ethernet must beat wifi, got %+v", c)
	}
	c, _ = Select([]Candidate{wifi, eth, down}, "ethernet 2", 0)
	if c.Luid != 4 {
		t.Fatalf("config adapter must win, got %+v", c)
	}
}

func TestMonitorLive(t *testing.T) {
	var m Monitor
	m.Poll(time.Now())
	t.Logf("adapter %q", m.Adapter())
	time.Sleep(1100 * time.Millisecond)
	m.Poll(time.Now())
	t.Logf("valid=%v rx=%s tx=%s", m.Valid, AppendCompact(nil, m.RxPerSec, false), AppendCompact(nil, m.TxPerSec, false))
}

func TestAppendCompact(t *testing.T) {
	cases := []struct {
		in   float64
		bits bool
		want string
	}{
		{0, false, "0 KB/s"},
		{300, false, "0 KB/s"},
		{600, false, "1 KB/s"},
		{4200, false, "4 KB/s"},
		{1023 * 1024, false, "1 MB/s"},
		{13_000_000, false, "12 MB/s"},
		{125_000, true, "1 Mb/s"},
	}
	for _, c := range cases {
		if got := string(AppendCompact(nil, c.in, c.bits)); got != c.want {
			t.Errorf("AppendCompact(%v, %v) = %q, want %q", c.in, c.bits, got, c.want)
		}
	}
}
