package txpool

import (
	"sync/atomic"

	"github.com/armon/go-metrics"

	"github.com/0xPolygon/polygon-edge/types"
)

const (
	highPressureMark = 80 // 80%
)

// Gauge for measuring pool capacity in slots
type slotGauge struct {
	height uint64 // amount of slots currently occupying the pool
	max    uint64 // max limit
}

// read returns the current height of the gauge.
func (g *slotGauge) read() uint64 {
	return atomic.LoadUint64(&g.height)
}

// increase increases the height of the gauge by the specified slots amount.
func (g *slotGauge) increase(slots uint64) {
	newHeight := atomic.AddUint64(&g.height, slots)
	metrics.SetGauge([]string{txPoolMetrics, "slots_used"}, float32(newHeight))
}

// increaseWithinLimit increases the height of the gauge by the specified slots amount only if the increased height is
// less than max. Returns true if the height is increased.
func (g *slotGauge) increaseWithinLimit(slots uint64) (updated bool) {
	for {
		old := g.read()
		newHeight := old + slots

		if newHeight > g.max {
			return false
		}

		if atomic.CompareAndSwapUint64(&g.height, old, newHeight) {
			metrics.SetGauge([]string{txPoolMetrics, "slots_used"}, float32(newHeight))

			return true
		}
	}
}

// decrease decreases the height of the gauge by the specified slots amount.
func (g *slotGauge) decrease(slots uint64) {
	newHeight := atomic.AddUint64(&g.height, ^(slots - 1))
	metrics.SetGauge([]string{txPoolMetrics, "slots_used"}, float32(newHeight))
}

// highPressure checks if the gauge level
// is higher than the 0.8*max threshold
func (g *slotGauge) highPressure() bool {
	return g.read() > (highPressureMark*g.max)/100
}

// free slots returns how many slots are currently available
func (g *slotGauge) freeSlots() uint64 {
	return g.max - g.read()
}

// slotsRequired calculates the number of slots required for given transaction(s).
func slotsRequired(txs ...*types.Transaction) uint64 {
	slots := uint64(0)
	for _, tx := range txs {
		slots += (tx.Size() + txSlotSize - 1) / txSlotSize
	}

	return slots
}
