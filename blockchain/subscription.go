package blockchain

import (
	"math/big"
	"sync"

	"github.com/0xPolygon/minimal/types"
)

type Subscription interface {
	GetEventCh() chan *Event
	GetEvent() *Event
	Close()
}

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

type subscription struct {
	updateCh chan struct{}
	closeCh  chan struct{}
	elem     *eventElem
}

func (s *subscription) GetEventCh() chan *Event {
	eventCh := make(chan *Event)
	go func() {
		evnt := s.GetEvent()
		eventCh <- evnt
	}()
	return eventCh
}

func (s *subscription) GetEvent() *Event {
	for {
		if s.elem.next != nil {
			s.elem = s.elem.next
			evnt := s.elem.event
			return evnt
		}

		// wait for an update
		select {
		case <-s.updateCh:
			continue
		case <-s.closeCh:
			return nil
		}
	}
}

func (s *subscription) Close() {
	close(s.closeCh)
}

type EventType int

const (
	EventHead EventType = iota
	EventReorg
	EventFork
)

type Event struct {
	// Old chain removed if there was a reorg
	OldChain []*types.Header

	// New part of the chain (or a fork)
	NewChain []*types.Header

	// Difficulty is the new difficulty created with this event
	Difficulty *big.Int

	// Type is the type of event
	Type EventType

	// Source is the source that generated the blocks for the event
	// right now it can be either the Sealer or the Syncer. TODO
	Source string
}

func (e *Event) SetDifficulty(b *big.Int) {
	e.Difficulty = new(big.Int).Set(b)
}

func (e *Event) AddNewHeader(h *types.Header) {
	hh := h.Copy()
	if e.NewChain == nil {
		e.NewChain = []*types.Header{}
	}
	e.NewChain = append(e.NewChain, hh)
}

func (e *Event) AddOldHeader(h *types.Header) {
	hh := h.Copy()
	if e.OldChain == nil {
		e.OldChain = []*types.Header{}
	}
	e.OldChain = append(e.OldChain, hh)
}

func (b *Blockchain) SubscribeEvents() Subscription {
	return b.stream.subscribe()
}

type eventElem struct {
	event *Event
	next  *eventElem
}

type eventStream struct {
	lock sync.Mutex
	head *eventElem

	// channel to notify updates
	updateCh []chan struct{}
}

func (e *eventStream) subscribe() *subscription {
	head, updateCh := e.Head()
	s := &subscription{
		elem:     head,
		updateCh: updateCh,
		closeCh:  make(chan struct{}),
	}
	return s
}

func (e *eventStream) Head() (*eventElem, chan struct{}) {
	e.lock.Lock()
	head := e.head

	ch := make(chan struct{})
	if e.updateCh == nil {
		e.updateCh = []chan struct{}{}
	}
	e.updateCh = append(e.updateCh, ch)

	e.lock.Unlock()
	return head, ch
}

func (e *eventStream) push(event *Event) {
	e.lock.Lock()
	newHead := &eventElem{
		event: event,
	}
	if e.head != nil {
		e.head.next = newHead
	}
	e.head = newHead

	// notify the subscriptors
	for _, update := range e.updateCh {
		select {
		case update <- struct{}{}:
		default:
		}
	}
	e.lock.Unlock()
}
