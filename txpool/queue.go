package txpool

import (
	"container/heap"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-sdk/types"
)

type transactions []*types.Transaction

type accountQueue struct {
	sync.RWMutex
	wLock uint32
	queue minNonceQueue
}

func newAccountQueue() *accountQueue {
	q := accountQueue{
		queue: make(minNonceQueue, 0),
	}

	heap.Init(&q.queue)
	return &q
}

func (q *accountQueue) lock(write bool) {
	switch write {
	case true:
		q.Lock()
		atomic.StoreUint32(&q.wLock, 1)
	case false:
		q.RLock()
		atomic.StoreUint32(&q.wLock, 0)
	}
}

func (q *accountQueue) unlock() {
	if atomic.SwapUint32(&q.wLock, 0) == 1 {
		q.Unlock()
	} else {
		q.RUnlock()
	}
}

func (q *accountQueue) prune(nonce uint64) (pruned transactions) {
	for {
		tx := q.peek()
		if tx == nil ||
			tx.Nonce >= nonce {
			break
		}

		tx = q.pop()
		pruned = append(pruned, tx)
	}

	return
}

// Pushes the given transactions onto the queue.
func (q *accountQueue) push(tx *types.Transaction) {
	heap.Push(&q.queue, tx)
}

// Returns the first transaction from the queue without removing it.
func (q *accountQueue) peek() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return q.queue.Peek()
}

// Removes the first transactions from the queue and returns it.
func (q *accountQueue) pop() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*types.Transaction)
}

// Returns the number of transactions in the queue.
func (q *accountQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// Empties the underlying queue and
// returns the removed transactions.
func (q *accountQueue) clear() (
	removed transactions,
) {
	for {
		tx := q.pop()
		if tx == nil {
			break
		}

		removed = append(removed, tx)
	}

	return
}

// transactions sorted by nonce (ascending)
type minNonceQueue transactions

/* Queue methods required by the heap interface */

func (q *minNonceQueue) Peek() *types.Transaction {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[0]
}

func (q *minNonceQueue) Len() int {
	return len(*q)
}

func (q *minNonceQueue) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *minNonceQueue) Less(i, j int) bool {
	return (*q)[i].Nonce < (*q)[j].Nonce
}

func (q *minNonceQueue) Push(x interface{}) {
	(*q) = append((*q), x.(*types.Transaction))
}

func (q *minNonceQueue) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]
	return x
}

type pricedQueue struct {
	queue maxPriceQueue
}

func newPricedQueue() *pricedQueue {
	q := pricedQueue{
		queue: make(maxPriceQueue, 0),
	}

	heap.Init(&q.queue)
	return &q
}

// Empties the underlying queue
// and returns the removed transactions.
func (q *pricedQueue) clear() (
	removed transactions,
) {
	for {
		tx := q.pop()
		if tx == nil {
			break
		}

		removed = append(removed, tx)
	}

	return
}

// Pushes the given transactions onto the queue.
func (q *pricedQueue) push(tx *types.Transaction) {
	heap.Push(&q.queue, tx)
}

// Removes the first transactions from the queue and returns it.
func (q *pricedQueue) pop() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*types.Transaction)
}

// Returns the number of transactions in the queue.
func (q *pricedQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// transactions sorted by gas price (descending)
type maxPriceQueue transactions

/* Queue methods required by the heap interface */

func (q *maxPriceQueue) Peek() *types.Transaction {
	if q.Len() == 0 {
		return nil
	}

	return (*q)[0]
}

func (q *maxPriceQueue) Len() int {
	return len(*q)
}

func (q *maxPriceQueue) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *maxPriceQueue) Less(i, j int) bool {
	return (*q)[i].GasPrice.Uint64() > (*q)[j].GasPrice.Uint64()
}

func (q *maxPriceQueue) Push(x interface{}) {
	(*q) = append((*q), x.(*types.Transaction))
}

func (q *maxPriceQueue) Pop() interface{} {
	old := q
	n := len(*old)
	x := (*old)[n-1]
	*q = (*old)[0 : n-1]
	return x
}
