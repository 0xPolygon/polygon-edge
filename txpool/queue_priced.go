package txpool

import (
	"container/heap"
	"math/big"
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

// cmp compares the given transactions by their fees and returns:
//   - 0 if they have same fees
//   - 1 if a has higher fees than b
//   - -1 if b has higher fees than a
func (q *maxPriceQueue) cmp(a, b *types.Transaction) int {
	baseFee := atomic.LoadUint64(&q.baseFee)
	effectiveTipA := a.EffectiveTip(baseFee)
	effectiveTipB := b.EffectiveTip(baseFee)

	// Compare effective tips if baseFee is specified
	if c := effectiveTipA.Cmp(effectiveTipB); c != 0 {
		return c
	}

	aGasFeeCap, bGasFeeCap := new(big.Int), new(big.Int)

	if a.GasFeeCap != nil {
		aGasFeeCap = aGasFeeCap.Set(a.GasFeeCap)
	}

	if b.GasFeeCap != nil {
		bGasFeeCap = bGasFeeCap.Set(b.GasFeeCap)
	}

	// Compare fee caps if baseFee is not specified or effective tips are equal
	if c := aGasFeeCap.Cmp(bGasFeeCap); c != 0 {
		return c
	}

	aGasTipCap, bGasTipCap := new(big.Int), new(big.Int)

	if a.GasTipCap != nil {
		aGasTipCap = aGasTipCap.Set(a.GasTipCap)
	}

	if b.GasTipCap != nil {
		bGasTipCap = bGasTipCap.Set(b.GasTipCap)
	}

	// Compare tips if effective tips and fee caps are equal
	return aGasTipCap.Cmp(bGasTipCap)
}
