package blockchain

import (
	"math/big"
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
)

type void struct{}

// Subscription is the blockchain subscription interface
type Subscription interface {
	GetEventCh() chan *Event
	GetEvent() *Event
	Close()
}

// FOR TESTING PURPOSES //

type MockSubscription struct {
	eventCh chan *Event
}

func NewMockSubscription() *MockSubscription {
	return &MockSubscription{eventCh: make(chan *Event)}
}

func (m *MockSubscription) Push(e *Event) {
	m.eventCh <- e
}

func (m *MockSubscription) GetEventCh() chan *Event {
	return m.eventCh
}

func (m *MockSubscription) GetEvent() *Event {
	evnt := <-m.eventCh

	return evnt
}

func (m *MockSubscription) Close() {
}

// subscription is the Blockchain event subscription object
type subscription struct {
	updateCh chan *Event // Channel for update information
	closeCh  chan void   // Channel for close signals
}

// GetEventCh creates a new event channel, and returns it
func (s *subscription) GetEventCh() chan *Event {
	eventCh := make(chan *Event)

	go func() {
		for {
			evnt := s.GetEvent()
			if evnt == nil {
				return
			}
			eventCh <- evnt
		}
	}()

	return eventCh
}

// GetEvent returns the event from the subscription (BLOCKING)
func (s *subscription) GetEvent() *Event {
	for {
		// Wait for an update
		select {
		case ev := <-s.updateCh:
			return ev
		case <-s.closeCh:
			return nil
		}
	}
}

// Close closes the subscription
func (s *subscription) Close() {
	close(s.closeCh)
}

type EventType int

const (
	EventHead  EventType = iota // New head event
	EventReorg                  // Chain reorganization event
	EventFork                   // Chain fork event
)

// Event is the blockchain event that gets passed to the listeners
type Event struct {
	// Old chain (removed headers) if there was a reorg
	OldChain []*types.Header

	// New part of the chain (or a fork)
	NewChain []*types.Header

	// Difficulty is the new difficulty created with this event
	Difficulty *big.Int

	// Type is the type of event
	Type EventType

	// Source is the source that generated the blocks for the event
	// right now it can be either the Sealer or the Syncer
	Source string
}

// Header returns the latest block header for the event
func (e *Event) Header() *types.Header {
	return e.NewChain[len(e.NewChain)-1]
}

// SetDifficulty sets the event difficulty
func (e *Event) SetDifficulty(b *big.Int) {
	e.Difficulty = new(big.Int).Set(b)
}

// AddNewHeader appends a header to the event's NewChain array
func (e *Event) AddNewHeader(newHeader *types.Header) {
	header := newHeader.Copy()

	if e.NewChain == nil {
		// Array doesn't exist yet, create it
		e.NewChain = []*types.Header{}
	}

	e.NewChain = append(e.NewChain, header)
}

// AddOldHeader appends a header to the event's OldChain array
func (e *Event) AddOldHeader(oldHeader *types.Header) {
	header := oldHeader.Copy()

	if e.OldChain == nil {
		// Array doesn't exist yet, create it
		e.OldChain = []*types.Header{}
	}

	e.OldChain = append(e.OldChain, header)
}

// SubscribeEvents returns a blockchain event subscription
func (b *Blockchain) SubscribeEvents() Subscription {
	return b.stream.subscribe()
}

// eventStream is the structure that contains the event list,
// as well as the update channel which it uses to notify of updates
type eventStream struct {
	sync.Mutex

	// channel to notify updates
	updateCh []chan *Event
}

// subscribe Creates a new blockchain event subscription
func (e *eventStream) subscribe() *subscription {
	return &subscription{
		updateCh: e.newUpdateCh(),
		closeCh:  make(chan void),
	}
}

// newUpdateCh returns the event update channel
func (e *eventStream) newUpdateCh() chan *Event {
	e.Lock()
	defer e.Unlock()

	ch := make(chan *Event, 1)

	if e.updateCh == nil {
		e.updateCh = make([]chan *Event, 0)
	}

	e.updateCh = append(e.updateCh, ch)

	return ch
}

// push adds a new Event, and notifies listeners
func (e *eventStream) push(event *Event) {
	e.Lock()
	defer e.Unlock()

	// Notify the listeners
	for _, update := range e.updateCh {
		select {
		case update <- event:
		default:
		}
	}
}
