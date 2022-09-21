package dial

import (
	"container/heap"
	"sync"

	"github.com/0xPolygon/polygon-edge/network/common"

	"github.com/libp2p/go-libp2p/core/peer"
)

// DialQueue is a queue that holds dials tasks for potential peers, implemented as a min-heap
type DialQueue struct {
	sync.Mutex

	heap  dialQueueImpl
	tasks map[peer.ID]*DialTask

	updateCh chan struct{}
	closeCh  chan struct{}
}

// NewDialQueue creates a new DialQueue instance
func NewDialQueue() *DialQueue {
	return &DialQueue{
		heap:     dialQueueImpl{},
		tasks:    map[peer.ID]*DialTask{},
		updateCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
	}
}

// Close closes the running DialQueue
func (d *DialQueue) Close() {
	close(d.closeCh)
}

// PopTask is a loop that handles update and close events [BLOCKING]
func (d *DialQueue) PopTask() *DialTask {
	for {
		task := d.popTaskImpl() // Blocking pop
		if task != nil {
			return task
		}

		select {
		case <-d.updateCh:
		case <-d.closeCh:
			return nil
		}
	}
}

// popTaskImpl is the implementation for task popping from the min-heap
func (d *DialQueue) popTaskImpl() *DialTask {
	d.Lock()
	defer d.Unlock()

	if len(d.heap) != 0 {
		// pop the first value and remove it from the heap
		tt := heap.Pop(&d.heap)

		task, ok := tt.(*DialTask)
		if !ok {
			return nil
		}

		return task
	}

	return nil
}

// DeleteTask deletes a task from the dial queue for the specified peer
func (d *DialQueue) DeleteTask(peer peer.ID) {
	d.Lock()
	defer d.Unlock()

	item, ok := d.tasks[peer]
	if ok {
		// negative index for popped element
		if item.index >= 0 {
			heap.Remove(&d.heap, item.index)
		}

		delete(d.tasks, peer)
	}
}

// AddTask adds a new task to the dial queue
func (d *DialQueue) AddTask(
	addrInfo *peer.AddrInfo,
	priority common.DialPriority,
) {
	d.Lock()
	defer d.Unlock()

	task := &DialTask{
		addrInfo: addrInfo,
		priority: uint64(priority),
	}
	d.tasks[addrInfo.ID] = task
	heap.Push(&d.heap, task)

	select {
	case d.updateCh <- struct{}{}:
	default:
	}
}
