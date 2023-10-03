package txpool

import (
	"container/heap"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
)

// A thread-safe wrapper of a minNonceQueue.
// All methods assume the (correct) lock is held.
type accountQueue struct {
	sync.RWMutex
	wLock atomic.Bool
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
	if write {
		q.Lock()
	} else {
		q.RLock()
	}

	q.wLock.Store(write)
}

func (q *accountQueue) unlock() {
	if q.wLock.Swap(false) {
		q.Unlock()
	} else {
		q.RUnlock()
	}
}

// prune removes all transactions from the queue
// with nonce lower than given.
func (q *accountQueue) prune(nonce uint64) (
	pruned []*types.Transaction,
) {
	for {
		if tx := q.peek(); tx == nil || tx.Nonce >= nonce {
			break
		}

		pruned = append(pruned, q.pop())
	}

	return
}

// clear removes all transactions from the queue.
func (q *accountQueue) clear() (removed []*types.Transaction) {
	// store txs
	removed = q.queue

	// clear the underlying queue
	q.queue = q.queue[:0]

	return
}

// push pushes the given transactions onto the queue.
func (q *accountQueue) push(tx *types.Transaction) {
	heap.Push(&q.queue, tx)
}

// peek returns the first transaction from the queue without removing it.
func (q *accountQueue) peek() *types.Transaction {
	return q.queue.Peek()
}

// pop removes the first transactions from the queue and returns it.
func (q *accountQueue) pop() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	transaction, ok := heap.Pop(&q.queue).(*types.Transaction)
	if !ok {
		return nil
	}

	return transaction
}

// length returns the number of transactions in the queue.
func (q *accountQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// transactions sorted by nonce (ascending)
type minNonceQueue []*types.Transaction

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
	// The higher gas price Tx comes first if the nonces are same
	if (*q)[i].Nonce == (*q)[j].Nonce {
		return (*q)[i].GasPrice.Cmp((*q)[j].GasPrice) > 0
	}

	return (*q)[i].Nonce < (*q)[j].Nonce
}

func (q *minNonceQueue) Push(x interface{}) {
	transaction, ok := x.(*types.Transaction)
	if !ok {
		return
	}

	*q = append(*q, transaction)
}

func (q *minNonceQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*q = old[0 : n-1]

	return item
}
