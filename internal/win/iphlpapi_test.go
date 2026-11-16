package win

import (
	"testing"
	"unsafe"
)

func TestMibLayout(t *testing.T) {
	var r MibIfRow2
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"sizeof MIB_IF_ROW2", unsafe.Sizeof(r), 1352},
		{"Alias", unsafe.Offsetof(r.Alias), 28},
		{"Description", unsafe.Offsetof(r.Description), 542},
		{"Type", unsafe.Offsetof(r.Type), 1128},
		{"flags", unsafe.Offsetof(r.InterfaceAndOperStatusFlags), 1152},
		{"OperStatus", unsafe.Offsetof(r.OperStatus), 1156},
		{"InOctets", unsafe.Offsetof(r.InOctets), 1208},
		{"OutOctets", unsafe.Offsetof(r.OutOctets), 1280},
		{"sizeof MIB_IPFORWARD_ROW2", unsafe.Sizeof(MibIpForwardRow2{}), 104},
		{"PrefixLength", unsafe.Offsetof(MibIpForwardRow2{}.PrefixLength), 40},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestIfTableLive(t *testing.T) {
	n := 0
	if !IfTable(func(r *MibIfRow2) { n++ }) {
		t.Fatal("GetIfTable2 failed")
	}
	if n == 0 {
		t.Fatal("no interfaces")
	}
}
