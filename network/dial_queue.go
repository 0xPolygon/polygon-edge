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

func (d *dialQueue) pop() *dialTask {
POP:
	tt := d.popImpl()
	if tt != nil {
		return tt
	}

	select {
	case <-d.updateCh:
		goto POP
	case <-d.closeCh:
		return nil
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

func (d *dialQueue) add(addr peer.AddrInfo, priority uint64) {
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
	addr peer.AddrInfo

	// priority of the task (the higher the better)
	priority uint64
}

type dialQueueImpl []*dialTask

func (t dialQueueImpl) Len() int { return len(t) }

func (t dialQueueImpl) Less(i, j int) bool {
	return t[i].priority < t[j].priority
}

func (t dialQueueImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

func (t *dialQueueImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*dialTask)
	item.index = n
	*t = append(*t, item)
}

func (t *dialQueueImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*t = old[0 : n-1]
	return item
}
