// Package netmon samples interface byte counters and turns them into rates.
//
// The logic is ported from FluentFlyout.SystemMonitor (NetworkRateCalculator,
// NetworkAdapterSelector, NetworkAdapterClassifier, UnitFormatter).
package netmon

import "time"

// Rate converts monotonic counters into per-second rates.
//
// The first sample of an adapter only records a baseline, so an adapter switch
// cannot produce a spike. A counter that moves backwards (adapter reset)
// rebases and reports 0. Not safe for concurrent use.
type Rate struct {
	luid    uint64
	prevRx  uint64
	prevTx  uint64
	prevAt  time.Time
	hasBase bool
}

// Sample adds one reading. ok is false when no rate is available yet.
func (c *Rate) Sample(luid, rx, tx uint64, at time.Time) (rxPerSec, txPerSec float64, ok bool) {
	if !c.hasBase || c.luid != luid {
		c.luid, c.prevRx, c.prevTx, c.prevAt, c.hasBase = luid, rx, tx, at, true
		return 0, 0, false
	}
	elapsed := at.Sub(c.prevAt).Seconds()
	if elapsed <= 0 {
		return 0, 0, false
	}
	if rx < c.prevRx || tx < c.prevTx {
		c.prevRx, c.prevTx, c.prevAt = rx, tx, at
		return 0, 0, true
	}
	rxPerSec = float64(rx-c.prevRx) / elapsed
	txPerSec = float64(tx-c.prevTx) / elapsed
	c.prevRx, c.prevTx, c.prevAt = rx, tx, at
	return rxPerSec, txPerSec, true
}

// Reset drops the baseline.
func (c *Rate) Reset() { *c = Rate{} }
