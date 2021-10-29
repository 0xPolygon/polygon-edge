package network

import (
	"container/heap"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
)

// dialQueue is a queue where we store all the possible peer targets that
// we can connect to.
type dialQueue struct {
	heap     dialQueueImpl
	lock     sync.Mutex
	items    map[peer.ID]*dialTask
	updateCh chan struct{}
	closeCh  chan struct{}
}

// newDialQueue creates a new DialQueue
func newDialQueue() *dialQueue {
	return &dialQueue{
		heap:     dialQueueImpl{},
		items:    map[peer.ID]*dialTask{},
		updateCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
	}
}

func (d *dialQueue) Close() {
	close(d.closeCh)
}

// pop is a loop that handles update and close events
func (d *dialQueue) pop() *dialTask {
	for {
		tt := d.popImpl() // Blocking pop
		if tt != nil {
			return tt
		}

		select {
		case <-d.updateCh:
		case <-d.closeCh:
			return nil
		}
	}
}

func (d *dialQueue) popImpl() *dialTask {
	d.lock.Lock()

	if len(d.heap) != 0 {
		// pop the first value and remove it from the heap
		tt := heap.Pop(&d.heap)
		d.lock.Unlock()
		return tt.(*dialTask)
	}

	d.lock.Unlock()

	return nil
}

func (d *dialQueue) del(peer peer.ID) {
	d.lock.Lock()
	defer d.lock.Unlock()

	item, ok := d.items[peer]
	if ok {
		// negative index for popped element
		if item.index >= 0 {
			heap.Remove(&d.heap, item.index)
		}
		delete(d.items, peer)
	}
}

func (d *dialQueue) add(addr *peer.AddrInfo, priority uint64) {
	d.lock.Lock()
	defer d.lock.Unlock()

	task := &dialTask{
		addr:     addr,
		priority: priority,
	}
	d.items[addr.ID] = task
	heap.Push(&d.heap, task)

	select {
	case d.updateCh <- struct{}{}:
	default:
	}
}

type dialTask struct {
	index int

	// info of the task
	addr *peer.AddrInfo

	// priority of the task (the higher the better)
	priority uint64
}

// The DialQueue is implemented as priority queue which utilizes a heap (standard Go implementation)
// https://golang.org/src/container/heap/heap.go
type dialQueueImpl []*dialTask

// GO HEAP EXTENSION //

// Len returns the length of the queue
func (t dialQueueImpl) Len() int { return len(t) }

// Less compares the priorities of two items at the passed in indexes (A < B)
func (t dialQueueImpl) Less(i, j int) bool {
	return t[i].priority < t[j].priority
}

// Swap swaps the places of the items at the passed-in indexes
func (t dialQueueImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

// Push adds a new item to the queue
func (t *dialQueueImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*dialTask)
	item.index = n
	*t = append(*t, item)
}

// Pop removes an item from the queue
func (t *dialQueueImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*t = old[0 : n-1]

	return item
}
