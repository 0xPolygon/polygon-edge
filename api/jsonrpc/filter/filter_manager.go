package filter

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/types"
	"github.com/google/uuid"
)

type blockSubscription interface {
	Watch() chan blockchain.Event
	Close()
}

type Filter struct {
	id string

	// block filter
	block *headElem

	// log cache
	// TODO: Specify this log object here instead of types
	logs []types.Log

	// log filter
	logFilter *LogFilter

	// index of the filter in the timer array
	index int

	// next time to timeout
	timestamp time.Time
}

func (f *Filter) isLogFilter() bool {
	return f.logFilter != nil
}

func (f *Filter) isBlockFilter() bool {
	return f.block != nil
}

func (f *Filter) match() bool {
	return false
}

// store is the interface with the blockchain required
// by the filter manager
type store interface {
	GetBody(hash types.Hash) (types.Body, error)
}

type FilterManager struct {
	Sub     blockSubscription
	closeCh chan struct{}

	filters map[string]*Filter
	lock    sync.Mutex

	updateCh chan struct{}
	timer    timeHeapImpl

	blockStream *blockStream
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
			if err := f.Uninstall(filter.id); err != nil {
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

func (f *FilterManager) nextTimeoutFilter() (*Filter, time.Time) {
	return nil, time.Time{}
}

func (f *FilterManager) dispatchEvent(evnt blockchain.Event) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	// first include all the new headers in the blockstream for the block filters
	for _, header := range evnt.NewChain {
		f.blockStream.push(header.Hash)
	}

	// check if there is a match for the log filters
	for _, i := range f.filters {
		if i.isLogFilter() {
			// check if any of the current logs is from a removed block
		}
	}

	return nil
}

func (f *FilterManager) GetFilterChanges() {

}

func (f *FilterManager) Uninstall(id string) error {
	f.lock.Lock()

	f.lock.Unlock()
	return nil
}

func (f *FilterManager) NewBlockFilter() (string, error) {
	return f.addFilter(nil)
}

func (f *FilterManager) NewLogFilter(logFilter *LogFilter) (string, error) {
	return f.addFilter(logFilter)
}

type LogFilter struct {
	// TODO: We are going to do only the subscription mechanism
	// and later on we will extrapolate to pending/latest and range logs.
	Addresses []types.Address
	Topics    [][]types.Hash
}

func (f *FilterManager) addFilter(logFilter *LogFilter) (string, error) {
	f.lock.Lock()

	filter := &Filter{
		id: uuid.New().String(),
	}

	if logFilter == nil {
		// block filter
		// take the reference from the stream
		filter.block = f.blockStream.Head()
	} else {
		// log filter
		filter.logFilter = logFilter
	}

	f.filters[filter.id] = filter
	heap.Push(&f.timer, filter)

	f.lock.Unlock()

	select {
	case f.updateCh <- struct{}{}:
	default:
	}

	return filter.id, nil
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

// blockStream is used to keep the stream of new block hashes and allow subscriptions
// of the stream at any point
type blockStream struct {
	lock sync.Mutex
	head *headElem
}

func (b *blockStream) Head() *headElem {
	b.lock.Lock()
	head := b.head
	b.lock.Unlock()
	return head
}

func (b *blockStream) push(newBlock types.Hash) {
	b.lock.Lock()
	newHead := &headElem{
		hash: newBlock,
	}
	if b.head != nil {
		b.head.next = newHead
	}
	b.head = newHead
	b.lock.Unlock()
}

type headElem struct {
	hash types.Hash
	next *headElem
}

func (h *headElem) getUpdates() ([]types.Hash, *headElem) {
	res := []types.Hash{}

	cur := h
	for {
		if cur.next == nil {
			break
		}
		cur = cur.next
		res = append(res, cur.hash)
	}
	return res, cur
}
