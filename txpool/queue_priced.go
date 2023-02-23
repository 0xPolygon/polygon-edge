package txpool

import (
	"container/heap"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
)

type pricedQueue struct {
	queue *maxPriceQueue
}

func newPricedQueue() *pricedQueue {
	q := pricedQueue{
		queue: &maxPriceQueue{},
	}

	heap.Init(q.queue)

	return &q
}

// clear empties the underlying queue.
func (q *pricedQueue) clear() {
	q.queue.txs = q.queue.txs[:0]
}

// Pushes the given transactions onto the queue.
func (q *pricedQueue) push(tx *types.Transaction) {
	heap.Push(q.queue, tx)
}

// Pop removes the first transaction from the queue
// or nil if the queue is empty.
func (q *pricedQueue) pop() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	transaction, ok := heap.Pop(q.queue).(*types.Transaction)
	if !ok {
		return nil
	}

	return transaction
}

// length returns the number of transactions in the queue.
func (q *pricedQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// transactions sorted by gas price (descending)
type maxPriceQueue struct {
	baseFee uint64
	txs     []*types.Transaction
}

/* Queue methods required by the heap interface */

func (q *maxPriceQueue) Peek() *types.Transaction {
	if q.Len() == 0 {
		return nil
	}

	return q.txs[0]
}

func (q *maxPriceQueue) Len() int {
	return len(q.txs)
}

func (q *maxPriceQueue) Swap(i, j int) {
	q.txs[i], q.txs[j] = q.txs[j], q.txs[i]
}

func (q *maxPriceQueue) Less(i, j int) bool {
	switch q.cmp(q.txs[i], q.txs[j]) {
	case -1:
		return true
	case 1:
		return false
	default:
		return q.txs[i].Nonce > q.txs[j].Nonce
	}
}

func (q *maxPriceQueue) Push(x interface{}) {
	transaction, ok := x.(*types.Transaction)
	if !ok {
		return
	}

	q.txs = append(q.txs, transaction)
}

func (q *maxPriceQueue) Pop() interface{} {
	old := q.txs
	n := len(old)
	x := old[n-1]
	q.txs = old[0 : n-1]

	return x
}

func (q *maxPriceQueue) cmp(a, b *types.Transaction) int {
	baseFee := atomic.LoadUint64(&q.baseFee)

	if baseFee > 0 {
		// Compare effective tips if baseFee is specified
		if c := a.EffectiveTip(baseFee).Cmp(b.EffectiveTip(baseFee)); c != 0 {
			return c
		}
	}

	// Compare fee caps if baseFee is not specified or effective tips are equal
	if c := a.GasFeeCap.Cmp(b.GasFeeCap); c != 0 {
		return c
	}

	// Compare tips if effective tips and fee caps are equal
	return a.GasTipCap.Cmp(b.GasTipCap)
}
