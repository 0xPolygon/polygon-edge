package jsonrpc

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
)

type blockSubscription interface {
	Watch() chan blockchain.Event
	Close()
}

type Filter struct {
	id string

	// index of the filter in the timer array
	index int

	// time to timeout
	timestamp time.Time
}

func (f *Filter) Match() bool {
	return false
}

type FilterManager struct {
	Sub     blockSubscription
	closeCh chan struct{}

	filters map[string]*Filter
	lock    sync.Mutex

	updateCh chan struct{}
	timer    timeHeapImpl
}

func (f *FilterManager) Run() {
	f.closeCh = make(chan struct{})
	f.updateCh = make(chan struct{})

	// watch for new events in the blockchain
	evntCh := f.Sub.Watch()

	var timeoutCh <-chan time.Time
	for {
		// check for the next filter to be removed
		filter, timeout := f.nextTimeoutFilter()
		if filter == nil {
			timeoutCh = nil
		} else {
			timeoutCh = time.After(timeout.Sub(time.Now()))
		}

		select {
		case evnt := <-evntCh:
			// new blockchain event
			if err := f.dispatchEvent(evnt); err != nil {
				fmt.Println(err)
			}

		case <-timeoutCh:
			// timeout for filter
			if err := f.removeFilter(filter.id); err != nil {
				fmt.Println(err)
			}

		case <-f.updateCh:
			// there is a new filter, reset the loop to start the timeout timer

		case <-f.closeCh:
			// stop the filter manager
			f.Sub.Close()
			return
		}
	}
}

func (f *FilterManager) dispatchEvent(evnt blockchain.Event) error {
	// TODO: use worker pool
	return nil
}

func (f *FilterManager) removeFilter(id string) error {
	return nil
}

func (f *FilterManager) nextTimeoutFilter() (*Filter, time.Time) {
	return nil, time.Time{}
}

func (f *FilterManager) findNextID() string {
	return "1"
}

func (f *FilterManager) AddFilter() (string, error) {
	f.lock.Lock()

	id := f.findNextID()
	filter := &Filter{
		id: id,
	}

	f.filters[id] = filter
	heap.Push(&f.timer, filter)

	f.lock.Unlock()

	select {
	case f.updateCh <- struct{}{}:
	default:
	}

	return id, nil
}

func (f *FilterManager) Close() {
	close(f.closeCh)
}

type timeHeapImpl []*Filter

func (t timeHeapImpl) Len() int { return len(t) }

func (t timeHeapImpl) Less(i, j int) bool {
	return t[i].timestamp.Before(t[j].timestamp)
}

func (t timeHeapImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

func (t *timeHeapImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*Filter)
	// 1 minute timeouts
	item.timestamp = time.Now().Add(1 * time.Minute)
	item.index = n
	*t = append(*t, item)
}

func (t *timeHeapImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*t = old[0 : n-1]
	return item
}
