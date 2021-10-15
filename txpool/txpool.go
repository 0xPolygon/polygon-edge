package txpool

import (
	"container/heap"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

const (
	defaultIdlePeriod = 1 * time.Minute
)

var (
	ErrIntrinsicGas        = errors.New("intrinsic gas too low")
	ErrNegativeValue       = errors.New("negative value")
	ErrNonEncryptedTxn     = errors.New("non-encrypted transaction")
	ErrInvalidSender       = errors.New("invalid sender")
	ErrNonceTooLow         = errors.New("nonce too low")
	ErrInsufficientFunds   = errors.New("insufficient funds for gas * price + value")
	ErrInvalidAccountState = errors.New("invalid account state")
	ErrAlreadyKnown        = errors.New("already known")
)

var topicNameV1 = "txpool/0.1"

// store interface defines State helper methods the Txpool should have access to
type store interface {
	Header() *types.Header
	GetNonce(root types.Hash, addr types.Address) uint64
	GetBalance(root types.Hash, addr types.Address) (*big.Int, error)
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
}

type signer interface {
	Sender(tx *types.Transaction) (types.Address, error)
}

// TxPool is module that handles pending transactions.
//
// There are fundamentally 2 queues in the txpool module:
// - Account based transactions (accountQueues)
// - Global valid transactions, from any account (pendingQueue)
type TxPool struct {
	logger     hclog.Logger
	signer     signer
	forks      chain.ForksInTime
	store      store
	idlePeriod time.Duration

	// Unsorted min heap of transactions per account.
	// The heap is min nonce based
	accountQueues map[types.Address]*txHeapWrapper

	// Max price heap for all transactions that are valid
	pendingQueue *txPriceHeap

	// Networking stack
	topic *network.Topic

	// Flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing bool

	// Flag indicating if the current node is running in dev mode
	dev bool

	// Notification channel used so signal added transactions to the pool
	NotifyCh chan struct{}

	// Indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer
}

// NewTxPool creates a new pool for transactions
func NewTxPool(
	logger hclog.Logger,
	sealing bool,
	forks chain.ForksInTime,
	store store,
	grpcServer *grpc.Server,
	network *network.Server,
) (*TxPool, error) {
	txPool := &TxPool{
		logger:        logger.Named("txpool"),
		store:         store,
		idlePeriod:    defaultIdlePeriod,
		accountQueues: make(map[types.Address]*txHeapWrapper),
		pendingQueue:  newTxPriceHeap(),
		sealing:       sealing,
		forks:         forks,
	}

	if network != nil {
		// subscribe to the gossip protocol
		topic, err := network.NewTopic(topicNameV1, &proto.Txn{})
		if err != nil {
			return nil, err
		}
		topic.Subscribe(txPool.handleGossipTxn)
		txPool.topic = topic
	}

	if grpcServer != nil {
		proto.RegisterTxnPoolOperatorServer(grpcServer, txPool)
	}
	return txPool, nil
}

// GetNonce returns the next nonce for the account, based on the txpool
func (t *TxPool) GetNonce(addr types.Address) (uint64, bool) {
	q, ok := t.accountQueues[addr]
	if !ok {
		return 0, false
	}
	return q.nextNonce, true
}

func (t *TxPool) AddSigner(s signer) {
	t.signer = s
}

func (t *TxPool) handleGossipTxn(obj interface{}) {
	if !t.sealing {
		return
	}

	raw := obj.(*proto.Txn)
	txn := new(types.Transaction)
	if err := txn.UnmarshalRLP(raw.Raw.Value); err != nil {
		t.logger.Error("failed to decode broadcasted txn", "err", err)
	} else {
		if err := t.addImpl("gossip", txn); err != nil {
			t.logger.Error("failed to add broadcasted txn", "err", err)
		}
	}
}

// EnableDev enables dev mode for the txpool
func (t *TxPool) EnableDev() {
	t.dev = true
}

// AddTx adds a new transaction to the pool and broadcasts it if networking is enabled
func (t *TxPool) AddTx(tx *types.Transaction) error {
	if err := t.addImpl("addTxn", tx); err != nil {
		return err
	}

	// broadcast the transaction only if network is enabled
	// and we are not in dev mode
	if t.topic != nil && !t.dev {
		txn := &proto.Txn{
			Raw: &any.Any{
				Value: tx.MarshalRLP(),
			},
		}
		if err := t.topic.Publish(txn); err != nil {
			t.logger.Error("failed to topic txn", "err", err)
		}
	}

	if t.NotifyCh != nil {
		select {
		case t.NotifyCh <- struct{}{}:
		default:
		}
	}
	return nil
}

// addImpl validates the tx and adds it to the appropriate account transaction queue.
// Additionally, it updates the global valid transactions queue
func (t *TxPool) addImpl(ctx string, tx *types.Transaction) error {
	// Since this is a single point of inclusion for new transactions both
	// to the promoted queue and pending queue we use this point to calculate the hash
	tx.ComputeHash()

	err := t.validateTx(tx)
	if err != nil {
		t.logger.Error("Discarding invalid transaction", "hash", tx.Hash, "err", err)
		return err
	}

	t.logger.Debug("add txn", "ctx", ctx, "hash", tx.Hash, "from", tx.From)

	txnsQueue, ok := t.accountQueues[tx.From]
	if !ok {
		stateRoot := t.store.Header().StateRoot

		// Initialize the account based transaction heap
		txnsQueue = newTxHeapWrapper()
		txnsQueue.nextNonce = t.store.GetNonce(stateRoot, tx.From)
		t.accountQueues[tx.From] = txnsQueue
	}
	txnsQueue.Add(tx)

	for _, promoted := range txnsQueue.Promote() {
		if pushErr := t.pendingQueue.Push(promoted); pushErr != nil {
			t.logger.Error(fmt.Sprintf("Unable to promote transaction %s, %v", promoted.Hash.String(), pushErr))
		}
	}
	return nil
}

// GetTxs gets both pending and queued transactions
func (t *TxPool) GetTxs() (map[types.Address]map[uint64]*types.Transaction, map[types.Address]map[uint64]*types.Transaction) {

	pendingTxs := make(map[types.Address]map[uint64]*types.Transaction)
	sortedPricedTxs := t.pendingQueue.index
	for _, sortedPricedTx := range sortedPricedTxs {
		if _, ok := pendingTxs[sortedPricedTx.from]; !ok {
			pendingTxs[sortedPricedTx.from] = make(map[uint64]*types.Transaction)
		}
		pendingTxs[sortedPricedTx.from][sortedPricedTx.tx.Nonce] = sortedPricedTx.tx
	}

	queuedTxs := make(map[types.Address]map[uint64]*types.Transaction)
	queue := t.accountQueues
	for addr, queuedTxn := range queue {
		for _, tx := range queuedTxn.txs {
			if _, ok := queuedTxs[addr]; !ok {
				queuedTxs[addr] = make(map[uint64]*types.Transaction)
			}
			queuedTxs[addr][tx.Nonce] = tx
		}
	}

	return pendingTxs, queuedTxs
}

// Length returns the size of the valid transactions in the txpool
func (t *TxPool) Length() uint64 {
	return t.pendingQueue.Length()
}

// Pop returns the max priced transaction from the
// valid transactions heap in txpool
func (t *TxPool) Pop() (*types.Transaction, func()) {
	txn := t.pendingQueue.Pop()
	if txn == nil {
		return nil, nil
	}
	ret := func() {
		if pushErr := t.pendingQueue.Push(txn.tx); pushErr != nil {
			t.logger.Error(fmt.Sprintf("Unable to promote transaction %s, %v", txn.tx.Hash.String(), pushErr))
		}
	}
	return txn.tx, ret
}

// ResetWithHeader does basic txpool housekeeping after a block write
func (t *TxPool) ResetWithHeader(h *types.Header) {
	evnt := &blockchain.Event{
		NewChain: []*types.Header{h},
	}
	t.ProcessEvent(evnt)
}

// ProcessEvent processes the blockchain event and resets the txpool accordingly
func (t *TxPool) ProcessEvent(evnt *blockchain.Event) {
	addTxns := map[types.Hash]*types.Transaction{}
	for _, evnt := range evnt.OldChain {
		// reinject these transactions on the pool
		block, ok := t.store.GetBlockByHash(evnt.Hash, true)
		if !ok {
			t.logger.Error("block not found on txn add", "hash", block.Hash())
		} else {
			for _, txn := range block.Transactions {
				addTxns[txn.Hash] = txn
			}
		}
	}

	delTxns := map[types.Hash]*types.Transaction{}
	for _, evnt := range evnt.NewChain {
		// remove these transactions from the pool
		block, ok := t.store.GetBlockByHash(evnt.Hash, true)
		if !ok {
			t.logger.Error("block not found on txn del", "hash", block.Hash())
		} else {
			for _, txn := range block.Transactions {
				delete(addTxns, txn.Hash)
				delTxns[txn.Hash] = txn
			}
		}
	}

	// try to include again the transactions in the pendingQueue list
	for _, txn := range addTxns {
		if err := t.addImpl("reorg", txn); err != nil {
			t.logger.Error("failed to add txn", "err", err)
		}
	}

	// remove the mined transactions from the pendingQueue list
	for _, txn := range delTxns {
		t.pendingQueue.Delete(txn)
	}
}

// validateTx validates that the transaction conforms to specific constraints to be added to the txpool
func (t *TxPool) validateTx(tx *types.Transaction) error {
	// Check if the transaction has a strictly positive value
	if tx.Value.Sign() < 0 {
		return ErrNegativeValue
	}

	if !t.dev && tx.From != types.ZeroAddress {
		// Only if we are in dev mode we can accept
		// a transaction without validation
		return ErrNonEncryptedTxn
	}

	// Check if the transaction is signed properly
	var signerErr error
	if tx.From == types.ZeroAddress {
		tx.From, signerErr = t.signer.Sender(tx)
		if signerErr != nil {
			return ErrInvalidSender
		}
	}

	// Grab the state root for the latest block
	stateRoot := t.store.Header().StateRoot

	// Check nonce ordering
	if t.store.GetNonce(stateRoot, tx.From) > tx.Nonce {
		return ErrNonceTooLow
	}

	accountBalance, balanceErr := t.store.GetBalance(stateRoot, tx.From)
	if balanceErr != nil {
		return ErrInvalidAccountState
	}

	// Check if the sender has enough funds to execute the transaction
	if accountBalance.Cmp(tx.Cost()) < 0 {
		return ErrInsufficientFunds
	}

	// Make sure the transaction has more gas than the basic transaction fee
	intrinsicGas, err := state.TransactionGasCost(tx, t.forks.Homestead, t.forks.Istanbul)
	if err != nil {
		return err
	}

	if tx.Gas < intrinsicGas {
		return ErrIntrinsicGas
	}

	return nil
}

// txHeapWrapper is a wrapper object for account based transactions
type txHeapWrapper struct {
	// txs is the actual min heap (nonce ordered) for account transactions
	txs txHeap

	// nextNonce is a field indicating what should be the next
	// valid nonce for the account transaction
	nextNonce uint64
}

// newTxHeapWrapper creates a new account based tx heap
func newTxHeapWrapper() *txHeapWrapper {
	return &txHeapWrapper{
		txs: txHeap{},
	}
}

// Add adds a new tx onto the account based tx heap
func (t *txHeapWrapper) Add(tx *types.Transaction) {
	t.Push(tx)
}

// pruneLowNonceTx removes any transactions from the account tx queue
// that have a lower nonce than the current account nonce in state
func (t *txHeapWrapper) pruneLowNonceTx() {
	for {
		// Grab the min-nonce transaction from the heap
		tx := t.Peek()
		if tx == nil || tx.Nonce >= t.nextNonce {
			break
		}

		// Drop it from the heap
		t.Pop()
	}
}

// Promote promotes all the new valid transactions
func (t *txHeapWrapper) Promote() []*types.Transaction {
	// Remove elements lower than nonce
	t.pruneLowNonceTx()

	// Promote elements
	tx := t.Peek()
	if tx == nil || tx.Nonce != t.nextNonce {
		// Nothing to promote
		return nil
	}

	promote := []*types.Transaction{}
	higherNonceTxs := []*types.Transaction{}

	reinsertFunc := func() {
		// Reinsert the tx back to the account specific transaction queue
		for _, highNonceTx := range higherNonceTxs {
			t.Push(highNonceTx)
		}
	}

	for {
		promote = append(promote, tx)
		t.Pop()

		var nextTx *types.Transaction
		if nextTx = t.Peek(); nextTx == nil {
			break
		}

		if tx.Nonce+1 != nextTx.Nonce {
			// Tx that have a higher nonce are shelved for later
			// when they can actually be parsed
			higherNonceTxs = append(higherNonceTxs, nextTx)
			break
		}

		tx = nextTx
	}

	// Find the last transaction to be promoted
	lastTxn := promote[len(promote)-1]
	// Grab its nonce value and set it as the reference next nonce
	t.nextNonce = lastTxn.Nonce + 1

	reinsertFunc()

	return promote
}

// Peek returns the lowest nonce transaction in the account based heap
func (t *txHeapWrapper) Peek() *types.Transaction {
	return t.txs.Peek()
}

// Push adds a transaction to the account based heap
func (t *txHeapWrapper) Push(tx *types.Transaction) {
	// Check if the current transaction has a higher or equal nonce
	// than all the current transactions in the account based heap
	i := sort.Search(len(t.txs), func(i int) bool {
		return t.txs[0].Nonce >= tx.Nonce
	})

	// If sort.Search found something, it will return the index
	// of the first found element for which func(i int) was true
	if i < len(t.txs) && t.txs[i].Nonce == tx.Nonce {
		// i is an index corresponding to an element in the
		// account based heap, and the nonces match up, so this tx is discarded
		return
	}

	// All checks have passed, add the tx to the account based heap
	heap.Push(&t.txs, tx)
}

// Pop removes the min-nonce transaction from the account based heap
func (t *txHeapWrapper) Pop() *types.Transaction {
	res := heap.Pop(&t.txs)
	if res == nil {
		return nil
	}

	return res.(*types.Transaction)
}

// Account based heap implementation //
// The heap is min-nonce ordered //

type txHeap []*types.Transaction

// Required method definitions for the standard golang heap package

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

// Price based heap implementation //
// The heap is max-price ordered //

type pricedTx struct {
	tx    *types.Transaction
	from  types.Address
	price *big.Int
	index int
}

type txPriceHeap struct {
	lock  sync.Mutex
	index map[types.Hash]*pricedTx
	heap  txPriceHeapImpl
}

func newTxPriceHeap() *txPriceHeap {
	return &txPriceHeap{
		index: make(map[types.Hash]*pricedTx),
		heap:  make(txPriceHeapImpl, 0),
	}
}

func (t *txPriceHeap) Length() uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return uint64(len(t.index))
}

func (t *txPriceHeap) Delete(tx *types.Transaction) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if item, ok := t.index[tx.Hash]; ok {
		heap.Remove(&t.heap, item.index)
		delete(t.index, tx.Hash)
	}
}

func (t *txPriceHeap) Push(tx *types.Transaction) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	price := new(big.Int).Set(tx.GasPrice)

	if _, ok := t.index[tx.Hash]; ok {
		return ErrAlreadyKnown
	}

	pTx := &pricedTx{
		tx:    tx,
		from:  tx.From,
		price: price,
	}
	t.index[tx.Hash] = pTx
	heap.Push(&t.heap, pTx)
	return nil
}

func (t *txPriceHeap) Pop() *pricedTx {
	t.lock.Lock()
	defer t.lock.Unlock()

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

// Required method definitions for the standard golang heap package

func (t txPriceHeapImpl) Len() int { return len(t) }

func (t txPriceHeapImpl) Less(i, j int) bool {
	if t[i].from == t[j].from {
		return t[i].tx.Nonce < t[j].tx.Nonce
	}

	return t[i].price.Cmp(t[j].price) >= 0
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
