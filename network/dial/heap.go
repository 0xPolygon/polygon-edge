package dial

// The DialQueue is implemented as priority queue which utilizes a heap (standard Go implementation)
// https://golang.org/src/container/heap/heap.go
type dialQueueImpl []*DialTask

// Len returns the length of the queue
func (t dialQueueImpl) Len() int { return len(t) }

// Less compares the priorities of two tasks at the passed in indexes (A < B)
func (t dialQueueImpl) Less(i, j int) bool {
	return t[i].priority < t[j].priority
}

// Swap swaps the places of the tasks at the passed-in indexes
func (t dialQueueImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

// Push adds a new item to the queue
func (t *dialQueueImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*DialTask) //nolint:forcetypeassert
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
