package dial

import (
	"container/heap"
	"context"
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
		updateCh: make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
	}
}

// Close closes the running DialQueue
func (d *DialQueue) Close() {
	close(d.closeCh)
}

// Wait waits for closing or updating event or end of context.
// Returns true for closing event or end of the context [BLOCKING].
func (d *DialQueue) Wait(ctx context.Context) bool {
	select {
	case <-ctx.Done(): // blocks until context is done ...
		return true
	case <-d.updateCh: // ... or AddTask is called ...
		return false
	case <-d.closeCh: // ... or dial queue is closed
		return true
	}
}

// PopTask is the implementation for task popping from the min-heap
func (d *DialQueue) PopTask() *DialTask {
	d.Lock()
	defer d.Unlock()

	if len(d.heap) != 0 {
		// pop the first value and remove it from the heap
		task, ok := heap.Pop(&d.heap).(*DialTask)
		if !ok {
			return nil
		}

		delete(d.tasks, task.addrInfo.ID)

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
		heap.Remove(&d.heap, item.index)
		delete(d.tasks, peer)
	}
}

// AddTask adds a new task to the dial queue
func (d *DialQueue) AddTask(addrInfo *peer.AddrInfo, priority common.DialPriority) {
	if d.addTaskImpl(addrInfo, priority) {
		select {
		case d.updateCh <- struct{}{}:
		default:
		}
	}
}

func (d *DialQueue) addTaskImpl(addrInfo *peer.AddrInfo, priority common.DialPriority) bool {
	d.Lock()
	defer d.Unlock()

	// do not spam queue with same addresses
	if item, ok := d.tasks[addrInfo.ID]; ok {
		// if existing priority greater than new one, replace item
		if item.priority > uint64(priority) {
			item.addrInfo = addrInfo
			item.priority = uint64(priority)
			heap.Fix(&d.heap, item.index)

			return true
		}

		return false
	}

	task := &DialTask{
		addrInfo: addrInfo,
		priority: uint64(priority),
	}
	d.tasks[addrInfo.ID] = task
	heap.Push(&d.heap, task)

	return true
}
