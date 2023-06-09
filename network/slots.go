package network

import (
	"context"
	"sync"
)

// Slots is synchronization structure
// A routine can invoke the TakeSlot method, which will block until at least one slot becomes available
// The ReturnSlot method can be called by other routines to increase the count of available slots by one
type Slots struct {
	ch        chan struct{}
	maximal   int64
	available int64

	lock sync.Mutex
}

// NewSlots creates *Slots object with maximal slots available
func NewSlots(maximal int64) *Slots {
	ch := make(chan struct{}, maximal)
	// add slots
	for i := int64(0); i < maximal; i++ {
		ch <- struct{}{}
	}

	return &Slots{
		ch:        ch,
		maximal:   maximal,
		available: maximal,
		lock:      sync.Mutex{},
	}
}

// TakeSlot takes slot if available or blocks until slot is available or context is done
func (s *Slots) TakeSlot(ctx context.Context) (int64, bool) {
	select {
	case <-ctx.Done():
		return -1, true
	case <-s.ch:
		s.lock.Lock()
		defer s.lock.Unlock()

		s.available--

		return s.available, false
	}
}

// ReturnSlot returns back one slot. There is guarantee that available slots must be <= than maximal slots
func (s *Slots) ReturnSlot() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	// prevent to return more slots than maximal
	if s.available == s.maximal {
		return s.available
	}

	s.ch <- struct{}{}
	s.available++

	return s.available
}

// GetAvailableCount returns currently available slots count
func (s *Slots) GetAvailableCount() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.available
}
