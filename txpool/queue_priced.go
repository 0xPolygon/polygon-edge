package txpool

import (
	"container/heap"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

type pricedQueue struct {
	queue *maxPriceQueue
}

// newPricesQueue creates the priced queue with initial transactions and base fee
func newPricesQueue(baseFee uint64, initialTxs []*types.Transaction) *pricedQueue {
	q := &pricedQueue{
		queue: &maxPriceQueue{
			baseFee: new(big.Int).SetUint64(baseFee),
			txs:     initialTxs,
		},
	}

	heap.Init(q.queue)

	return q
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
func (q *pricedQueue) length() int {
	return q.queue.Len()
}

// transactions sorted by gas price (descending)
type maxPriceQueue struct {
	baseFee *big.Int
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

// @see https://github.com/etclabscore/core-geth/blob/4e2b0e37f89515a4e7b6bafaa40910a296cb38c0/core/txpool/list.go#L458
// for details why is something implemented like it is
func (q *maxPriceQueue) Less(i, j int) bool {
	switch cmp(q.txs[i], q.txs[j], q.baseFee) {
	case -1:
		return false
	case 1:
		return true
	default:
		return q.txs[i].Nonce < q.txs[j].Nonce
	}
}

func cmp(a, b *types.Transaction, baseFee *big.Int) int {
	if baseFee.BitLen() > 0 {
		// Compare effective tips if baseFee is specified
		if c := a.EffectiveGasTip(baseFee).Cmp(b.EffectiveGasTip(baseFee)); c != 0 {
			return c
		}
	}

	// Compare fee caps if baseFee is not specified or effective tips are equal
	if c := a.GetGasFeeCap().Cmp(b.GetGasFeeCap()); c != 0 {
		return c
	}

	// Compare tips if effective tips and fee caps are equal
	return a.GetGasTipCap().Cmp(b.GetGasTipCap())
}
