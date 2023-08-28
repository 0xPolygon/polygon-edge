package network

import (
	"context"
)

// Slots is synchronization structure
// A routine can invoke the Take method, which will block until at least one slot becomes available
// The Release method can be called by other routines to increase the number of available slots by one
type Slots chan struct{}

// NewSlots creates Slots object with maximal slots available
func NewSlots(maximal int64) Slots {
	slots := make(Slots, maximal)
	// add slots
	for i := int64(0); i < maximal; i++ {
		slots <- struct{}{}
	}

	return slots
}

// Take takes slot if available or blocks until slot is available or context is done
func (s Slots) Take(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-s:
		return false
	}
}

// Release returns back one slot. If all slots are already released, nothing will happen
func (s Slots) Release() {
	select {
	case s <- struct{}{}:
	default: // No slot available to release, do nothing
	}
}
