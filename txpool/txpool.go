package txpool

import (
	"container/heap"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

// stateInterface is the interface required by the txpool to interact
// with the state
type stateInterface interface {
	GetNonce(root types.Hash, addr types.Address)
}

const (
	defaultIdlePeriod = 1 * time.Minute
)

var emptyFrom = types.StringToAddress("0")

// TxPool is a pool of transactions
type TxPool struct {
	signer     crypto.TxSigner
	state      *state.Txn
	blockchain *blockchain.Blockchain
	idlePeriod time.Duration

	pending []*types.Transaction
	queue   map[types.Address]*txQueue
}

// NewTxPool creates a new pool of transactios
func NewTxPool(blockchain *blockchain.Blockchain) *TxPool {
	txPool := &TxPool{
		signer:     crypto.NewEIP155Signer(1),
		blockchain: blockchain,
		idlePeriod: defaultIdlePeriod,
		queue:      make(map[types.Address]*txQueue, 0),
	}
	return txPool
}

// SetSigner changes the signer
func (t *TxPool) SetSigner(signer crypto.TxSigner) {
	t.signer = signer
}

// Add adds a new transaction to the pool
func (t *TxPool) Add(tx *types.Transaction) error {
	err := t.validateTx(tx)
	if err != nil {
		return err
	}

	if tx.From == emptyFrom {
		tx.From, err = t.signer.Sender(tx)
		if err != nil {
			return fmt.Errorf("invalid sender")
		}
	}

	txs, ok := t.queue[tx.From]
	if !ok {
		txs = newTxQueue()
		t.queue[tx.From] = txs
	}
	txs.Add(tx)

	return nil
}

func (t *TxPool) Pending() []*types.Transaction {
	return t.pending
}

func (t *TxPool) Update(b *types.Block, state *state.Txn) error {
	t.state = state
	return nil
}

func (t *TxPool) reset(oldHead, newHead *types.Header) ([]*types.Transaction, error) {
	var reinject []*types.Transaction

	if oldHead != nil && oldHead.Hash != newHead.ParentHash {
		var discarded, included []*types.Transaction

		oldHeader, ok := t.blockchain.GetBlockByHash(oldHead.Hash, true)
		if !ok {
			return nil, fmt.Errorf("block by hash '%s' not found", oldHead.Hash.String())
		}
		newHeader, ok := t.blockchain.GetBlockByHash(newHead.Hash, true)
		if !ok {
			return nil, fmt.Errorf("block by hash '%s' not found", newHead.Hash.String())
		}

		for oldHeader.Number() > newHeader.Number() {
			discarded = append(discarded, oldHeader.Transactions...)
			oldHeader, ok = t.blockchain.GetBlockByHash(oldHeader.ParentHash(), true)
			if !ok {
				return nil, fmt.Errorf("block by hash '%s' not found", oldHeader.ParentHash().String())
			}
		}

		for newHeader.Number() > oldHeader.Number() {
			included = append(included, newHeader.Transactions...)
			newHeader, ok = t.blockchain.GetBlockByHash(newHeader.ParentHash(), true)
			if !ok {
				return nil, fmt.Errorf("block by hash '%s' not found", newHeader.ParentHash().String())
			}
		}

		for oldHeader.Hash() != newHeader.Hash() {
			discarded = append(discarded, oldHeader.Transactions...)
			included = append(included, newHeader.Transactions...)

			oldHeader, ok = t.blockchain.GetBlockByHash(oldHeader.ParentHash(), true)
			if !ok {
				return nil, fmt.Errorf("block by hash '%s' not found", oldHeader.ParentHash().String())
			}
			newHeader, ok = t.blockchain.GetBlockByHash(newHeader.ParentHash(), true)
			if !ok {
				return nil, fmt.Errorf("block by hash '%s' not found", newHeader.ParentHash().String())
			}
		}

		reinject = txDifference(discarded, included)
	}

	// reinject all the transactions into the blocks
	for _, tx := range reinject {
		if err := t.Add(tx); err != nil {
			return nil, err
		}
	}

	promoted := []*types.Transaction{}

	// Get all the pending transactions and update
	for from, list := range t.queue {
		// TODO, filter low txs
		nonce := t.state.GetNonce(from)
		res := list.Promote(nonce)
		promoted = append(promoted, res...)
	}

	return promoted, nil
}

func (t *TxPool) sortTxns(txn *state.Txn, parent *types.Header) (*txPriceHeap, error) {
	t.Update(nil, txn)
	promoted, err := t.reset(nil, parent)
	if err != nil {
		return nil, err
	}

	pricedTxs := newTxPriceHeap()
	for _, tx := range promoted {
		if tx.From == emptyFrom {
			tx.From, err = t.signer.Sender(tx)
			if err != nil {
				return nil, err
			}
		}

		// NOTE, we need to sort with big.Int instead of uint64
		if err := pricedTxs.Push(tx.From, tx, new(big.Int).SetBytes(tx.GetGasPrice())); err != nil {
			return nil, err
		}
	}
	return pricedTxs, nil
}

func (t *TxPool) validateTx(tx *types.Transaction) error {
	/*
		if tx.Size() > 32*1024 {
			return fmt.Errorf("oversize data")
		}
		if tx.Value.Sign() < 0 {
			return fmt.Errorf("negative value")
		}
	*/
	return nil
}

type txQueue struct {
	txs  txHeap
	last time.Time
}

func newTxQueue() *txQueue {
	return &txQueue{
		txs:  txHeap{},
		last: time.Now(),
	}
}

// LastTime returns the last time queried
func (t *txQueue) LastTime() time.Time {
	return t.last
}

// Add adds a new tx into the queue
func (t *txQueue) Add(tx *types.Transaction) {
	t.last = time.Now()
	t.Push(tx)
}

// Promote promotes all the new valid transactions
func (t *txQueue) Promote(nextNonce uint64) []*types.Transaction {

	// Remove elements lower than nonce
	for {
		tx := t.Peek()
		if tx == nil || tx.Nonce >= nextNonce {
			break
		}
		t.Pop()
	}

	// Promote elements
	tx := t.Peek()
	if tx == nil || tx.Nonce != nextNonce {
		return nil
	}

	promote := []*types.Transaction{}
	for {
		promote = append(promote, tx)
		t.Pop()

		tx2 := t.Peek()
		if tx2 == nil || tx.Nonce+1 != tx2.Nonce {
			break
		}
		tx = tx2
	}
	return promote
}

func (t *txQueue) Peek() *types.Transaction {
	return t.txs.Peek()
}

func (t *txQueue) Push(tx *types.Transaction) {
	heap.Push(&t.txs, tx)
}

func (t *txQueue) Pop() *types.Transaction {
	res := heap.Pop(&t.txs)
	if res == nil {
		return nil
	}

	return res.(*types.Transaction)
}

// Nonce ordered heap

type txHeap []*types.Transaction

func (t *txHeap) Peek() *types.Transaction {
	if len(*t) == 0 {
		return nil
	}
	return (*t)[0]
}

func (t *txHeap) Len() int {
	return len(*t)
}

func (t *txHeap) Swap(i, j int) {
	(*t)[i], (*t)[j] = (*t)[j], (*t)[i]
}

func (t *txHeap) Less(i, j int) bool {
	return (*t)[i].Nonce < (*t)[j].Nonce
}

func (t *txHeap) Push(x interface{}) {
	(*t) = append((*t), x.(*types.Transaction))
}

func (t *txHeap) Pop() interface{} {
	old := *t
	n := len(old)
	x := old[n-1]
	*t = old[0 : n-1]
	return x
}

// Price ordered heap

type pricedTx struct {
	tx    *types.Transaction
	from  types.Address
	price *big.Int
	index int
}

type txPriceHeap struct {
	index map[types.Hash]*pricedTx
	heap  txPriceHeapImpl
}

func newTxPriceHeap() *txPriceHeap {
	return &txPriceHeap{
		index: make(map[types.Hash]*pricedTx),
		heap:  make(txPriceHeapImpl, 0),
	}
}

func (t *txPriceHeap) Push(from types.Address, tx *types.Transaction, price *big.Int) error {
	if _, ok := t.index[tx.Hash]; ok {
		return fmt.Errorf("tx %s already exists", tx.Hash)
	}

	pTx := &pricedTx{
		tx:    tx,
		from:  from,
		price: price,
	}
	t.index[tx.Hash] = pTx
	heap.Push(&t.heap, pTx)
	return nil
}

func (t *txPriceHeap) Pop() *pricedTx {
	if len(t.index) == 0 {
		return nil
	}
	tx := heap.Pop(&t.heap).(*pricedTx)
	delete(t.index, tx.tx.Hash)
	return tx
}

func (t *txPriceHeap) Contains(tx *types.Transaction) bool {
	_, ok := t.index[tx.Hash]
	return ok
}

type txPriceHeapImpl []*pricedTx

func (t txPriceHeapImpl) Len() int { return len(t) }

func (t txPriceHeapImpl) Less(i, j int) bool {
	if t[i].from == t[j].from {
		return t[i].tx.Nonce < t[j].tx.Nonce
	}
	return t[i].price.Cmp((t[j].price)) < 0
}

func (t txPriceHeapImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

func (t *txPriceHeapImpl) Push(x interface{}) {
	n := len(*t)
	job := x.(*pricedTx)
	job.index = n
	*t = append(*t, job)
}

func (t *txPriceHeapImpl) Pop() interface{} {
	old := *t
	n := len(old)
	job := old[n-1]
	job.index = -1
	*t = old[0 : n-1]
	return job
}

// TxDifference returns a new set which is the difference between a and b.
func txDifference(a, b []*types.Transaction) []*types.Transaction {
	keep := make([]*types.Transaction, 0, len(a))

	remove := make(map[types.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash]; !ok {
			keep = append(keep, tx)
		}
	}
	return keep
}
