package netmon

import (
	"math"
	"strconv"
)

var (
	byteUnits = [...]string{"B/s", "KB/s", "MB/s", "GB/s", "TB/s"}
	bitUnits  = [...]string{"b/s", "Kb/s", "Mb/s", "Gb/s", "Tb/s"}
)

// WidestRates are the widest strings AppendCompact can produce per unit
// family, used to size the fixed value slot.
var (
	WidestByteRates = [...]string{"999 KB/s", "999 MB/s", "999 GB/s"}
	WidestBitRates  = [...]string{"999 Kb/s", "999 Mb/s", "999 Gb/s"}
)

// AppendRate appends a rate such as "12.4 MB/s" to dst, with precision
// decimals and at most three integer digits. Bytes use 1024 steps; bits use
// 1000 steps (input is still bytes per second). Trailing zero decimals are
// dropped. kMin makes KB/s (Kb/s) the smallest unit.
// It does not allocate when dst has room.
func AppendRate(dst []byte, bytesPerSec float64, bits bool, precision int, kMin bool) []byte {
	if math.IsNaN(bytesPerSec) || math.IsInf(bytesPerSec, 0) || bytesPerSec < 0 {
		return append(dst, "—"...)
	}
	units := byteUnits[:]
	step := 1024.0
	v := bytesPerSec
	if bits {
		units = bitUnits[:]
		step = 1000
		v *= 8
	}
	i := 0
	if kMin {
		v /= step
		i = 1
	}
	pow := math.Pow10(precision)
	round := func(x float64) float64 { return math.Round(x*pow) / pow }
	for round(v) >= 1000 && i < len(units)-1 {
		v /= step
		i++
	}
	r := round(v)
	if r == math.Trunc(r) {
		dst = strconv.AppendInt(dst, int64(r), 10)
	} else {
		dst = strconv.AppendFloat(dst, r, 'f', -1, 64)
	}
	dst = append(dst, ' ')
	return append(dst, units[i]...)
}

// AppendCompact is the taskbar format: no decimals, KB/s minimum.
func AppendCompact(dst []byte, bytesPerSec float64, bits bool) []byte {
	return AppendRate(dst, bytesPerSec, bits, 0, true)
}
