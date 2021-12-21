package txpool

import (
	"container/heap"

	"github.com/0xPolygon/polygon-sdk/types"
)

/* Queue implementations required by the heap interface */

type transactions []*types.Transaction

// transactions sorted by nonce (ascending)
type minNonceQueue struct {
	txs transactions
}

func newMinNonceQueue() minNonceQueue {
	q := minNonceQueue{
		txs: make(transactions, 0),
	}

	heap.Init(&q)
	return q
}

func (q *minNonceQueue) Peek() *types.Transaction {
	if q.Len() == 0 {
		return nil
	}

	return q.txs[0]
}

func (q *minNonceQueue) Len() int {
	return len(q.txs)
}

func (q *minNonceQueue) Swap(i, j int) {
	q.txs[i], q.txs[j] = q.txs[j], q.txs[i]
}

func (q *minNonceQueue) Less(i, j int) bool {
	return q.txs[i].Nonce < q.txs[j].Nonce
}

func (q *minNonceQueue) Push(x interface{}) {
	q.txs = append(q.txs, x.(*types.Transaction))
}

func (q *minNonceQueue) Pop() interface{} {
	old := q.txs
	n := len(old)
	x := old[n-1]
	q.txs = old[0 : n-1]
	return x
}

// transactions sorted by gas price (descending)
type maxPriceQueue struct {
	txs transactions
}

func newMaxPriceQueue() maxPriceQueue {
	q := maxPriceQueue{
		txs: make(transactions, 0),
	}

	heap.Init(&q)
	return q
}

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
	if q.txs[i].From == q.txs[j].From {
		return q.txs[i].Nonce < q.txs[j].Nonce
	}

	return q.txs[i].GasPrice.Uint64() > q.txs[j].GasPrice.Uint64()
}

func (q *maxPriceQueue) Push(x interface{}) {
	q.txs = append(q.txs, x.(*types.Transaction))
}

func (q *maxPriceQueue) Pop() interface{} {
	old := q.txs
	n := len(old)
	x := old[n-1]
	q.txs = old[0 : n-1]
	return x
}
