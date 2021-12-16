package txpool

import (
	"container/heap"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

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
	txSlotSize        = 32 * 1024  // 32kB
	txMaxSize         = 128 * 1024 //128Kb
	topicNameV1       = "txpool/0.1"
)

var (
	ErrIntrinsicGas        = errors.New("intrinsic gas too low")
	ErrNegativeValue       = errors.New("negative value")
	ErrNonEncryptedTx      = errors.New("non-encrypted transaction")
	ErrInvalidSender       = errors.New("invalid sender")
	ErrTxPoolOverflow      = errors.New("txpool is full")
	ErrUnderpriced         = errors.New("transaction underpriced")
	ErrNonceTooLow         = errors.New("nonce too low")
	ErrInsufficientFunds   = errors.New("insufficient funds for gas * price + value")
	ErrInvalidAccountState = errors.New("invalid account state")
	ErrAlreadyKnown        = errors.New("already known")
	// ErrOversizedData is returned if size of a transction is greater than the specified limit
	ErrOversizedData = errors.New("oversized data")
)

type TransactionPoolInterface interface {
	ResetWithHeader(h *types.Header)
	WriteTransactions(write WriteTxCallback) (*types.Transaction, func())
}

type WriteTxStatus int

const (
	Recoverable WriteTxStatus = iota
	Unrecoverable
	Abort
	Ok
)

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

// Gauge for measuring pool capacity in slots
type slotGauge struct {
	height uint64
	max    uint64
}

// Returns the current height of the gauge
func (g *slotGauge) read() uint64 {
	return atomic.LoadUint64(&g.height)
}

// Increases the height of the gauge by the specified slots amount
func (g *slotGauge) increase(slots uint64) {
	atomic.AddUint64(&g.height, slots)
}

// Decreases the height of the gauge by the specified slots amount
func (g *slotGauge) decrease(slots uint64) {
	atomic.AddUint64(&g.height, ^(slots - 1))
}

type Config struct {
	MaxSlots uint64
	Sealing  bool
}

/* All requests are handled in the main loop */

// addRequest is sent when a transaction
// has gone through addTx() successfully
// and is ready to be added to the pool.
type addRequest struct {
	tx *types.Transaction
	// isLocal bool
}

// promoteRequest is sent to signal that
// some account queue is ready for promotion.
// Happens when a previously added transaction
// has expected nonce (indicated by nonceMap).
type promoteRequest struct {
	account types.Address
}

// resetRequest is handled when Ibft
// calls ResetWithHeader to align the state
// of the pool with the new block.
type resetRequest struct {
	newNonces map[types.Address]uint64
}

// rollbackRequests are used to handle
// unrecoverable transactions during WriteTransactions
// (called within Ibft) to rollback subsequent
// transactions from the same account.
type rollbackRequest struct {
	demoted transactions
}

// TxPool is module that handles pending transactions.
// There are fundamentally 2 queues any transaction
// needs to go through:
// - 1. Account queue (account specific transactions)
// - 2. Promoted queue (global transactions)
//
// The main difference between these queues is that
// account queues make sure a transaction is promoted
// in the correct (nonce) order. Promoted means
// the received transaction's nonce is expected for this account
// queue and can be moved to the promoted queue.
//
// The promoted queue acts as a sink to transactions
// promoted from any account queue sorted by max gasPrice
// where they wait to be written to the chain.
type TxPool struct {
	logger     hclog.Logger
	signer     signer
	forks      chain.ForksInTime
	store      store
	idlePeriod time.Duration

	// mao of all enqueued transactions by account
	enqueued map[types.Address]*accountQueue

	// map of all account queue locks
	locks accountLocks

	// promoted transactions
	promoted *promotedQueue

	// next expected nonce for each account
	nextNonces nonceMap

	// Lookup map keeping track
	// of all transactions present in the pool
	all lookupMap

	// Networking stack
	topic *network.Topic

	// Gauge for measuring pool capacity
	gauge slotGauge

	// Request channels used to signal various events to the main loop
	addReqCh      chan addRequest
	promoteReqCh  chan promoteRequest
	resetReqCh    chan resetRequest
	rollbackReqCh chan rollbackRequest

	// Flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing bool

	// Flag indicating if the current node is running in dev mode (used for testing)
	dev bool

	// Prometheus API
	metrics *Metrics

	// Indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer
}

/// NewTxPool creates a new pool for transactions
func NewTxPool(
	logger hclog.Logger,
	forks chain.ForksInTime,
	store store,
	grpcServer *grpc.Server,
	network *network.Server,
	metrics *Metrics,
	config *Config,
) (*TxPool, error) {
	pool := &TxPool{
		logger:     logger.Named("txpool"),
		forks:      forks,
		store:      store,
		idlePeriod: defaultIdlePeriod,
		metrics:    metrics,
		enqueued:   make(map[types.Address]*accountQueue),
		promoted:   newPromotedQueue(logger.Named("promoted")),
		gauge:      slotGauge{height: 0, max: config.MaxSlots},
		sealing:    config.Sealing,
	}

	if network != nil {
		// subscribe to the gossip protocol
		topic, err := network.NewTopic(topicNameV1, &proto.Txn{})
		if err != nil {
			return nil, err
		}

		topic.Subscribe(pool.handleGossipTxn)
		pool.topic = topic
	}

	if grpcServer != nil {
		proto.RegisterTxnPoolOperatorServer(grpcServer, pool)
	}

	// initialise channels
	pool.addReqCh = make(chan addRequest)
	pool.promoteReqCh = make(chan promoteRequest)
	pool.resetReqCh = make(chan resetRequest)
	pool.rollbackReqCh = make(chan rollbackRequest)

	// start listening for requests
	// go pool.runLoop()

	return pool, nil
}

func (p *TxPool) Start() {
	go p.runLoop()
}

// Pool's main loop listening to and handling requests
func (p *TxPool) runLoop() {
	for {
		select {
		case req := <-p.addReqCh:
			go p.handleAddRequest(req)
		case req := <-p.promoteReqCh:
			go p.handlePromoteRequest(req)
		case req := <-p.resetReqCh:
			go p.handleResetRequest(req)
		case req := <-p.rollbackReqCh:
			go p.handleRollbackRequest(req)
		}
	}
}

// handleAddRequest enqueues a transaction
// to its designated account queue, if possible
func (p *TxPool) handleAddRequest(req addRequest) {
	tx := req.tx
	addr := tx.From
	tx.ComputeHash()

	p.lockAccount(addr, true)
	defer p.unlockAccount(addr)

	// check if already known
	if _, ok := p.all.load(tx.Hash); ok {
		return
	}

	nextNonce, _ := p.nextNonces.load(addr)
	if tx.Nonce < nextNonce {
		return // reject low nonce
	}

	// push tx onto queue
	queue := p.enqueued[addr]
	queue.push(tx)

	// atomically increase gauge
	p.gauge.increase(slotsRequired(tx))

	if tx.Nonce == nextNonce {
		// account queue is ready for promotion
		p.promoteReqCh <- promoteRequest{account: addr} // BLOCKING
	}
}

// handlePromoteRequest handles promoting transactions
// from the account queue.
// Can only be triggered by an appropriate add request (nonce is expected)
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
	addr := req.account

	p.lockPromoted(true)
	defer p.unlockPromoted()

	p.lockAccount(addr, true)
	defer p.unlockAccount(addr)

	nextNonce, _ := p.nextNonces.load(addr)

	// extract promotables
	promotables := p.enqueued[addr].promote(nextNonce)
	if len(promotables) == 0 {
		// nothing to promote
		return
	}

	// push promotables to promoted
	for _, tx := range promotables {
		p.promoted.push(tx)
	}

	// update next nonce
	latestNonce := promotables[len(promotables)-1].Nonce
	p.nextNonces.store(addr, latestNonce+1)
}

// handleResetRequest is called during ResetWithHeader
// and aligns the pool's state for all accounts by pruning
// stale transactions.
func (p *TxPool) handleResetRequest(req resetRequest) {
	p.lockPromoted(true)
	defer p.unlockPromoted()

	newNonces := req.newNonces

	// p.pruneStalePromoted(newNonces)

	// p.pruneStaleEnqueued(newNonces)

	// prune all stale txs in promoted
	pruned := p.promoted.prune(newNonces)
	for _, tx := range pruned {
		p.gauge.decrease(slotsRequired(tx)) // log
	}

	// reset each account queue
	var wg sync.WaitGroup
	for addr, nonce := range newNonces {
		wg.Add(1)
		go func(addr types.Address, nonce uint64) {
			defer wg.Done()

			p.lockAccount(addr, true)
			defer p.unlockAccount(addr)

			// prune account
			pruned := p.enqueued[addr].prune(nonce)
			for _, tx := range pruned {
				p.gauge.decrease(slotsRequired(tx)) // log
			}

			// update next nonce
			p.nextNonces.store(addr, nonce)

		}(addr, nonce)
	}
	wg.Wait()

}

// handleRollbackRequest handles an unfortunate transition write
// of a transaction during ExecuteTransactions and demotes
// any promoted transactions that were next-in-line for that account.
// Rollback means resetting the expected nonce for that account
// to a fallback value and re-issuing addRequests for the demoted
// transactions.
func (p *TxPool) handleRollbackRequest(req rollbackRequest) {
	demoted := req.demoted

	// reissue add requests [BLOCKING]
	for _, tx := range demoted {
		p.addReqCh <- addRequest{tx: tx}
	}
}

func (p *TxPool) AddSigner(s signer) {
	p.signer = s
}

// EnableDev enables dev mode for the txpool
func (p *TxPool) EnableDev() {
	p.dev = true
}

// AddTx adds a new transaction to the pool (sent from json-RPC/gRPC endpoints)
// and broadcasts it if networking is enabled
func (p *TxPool) AddTx(tx *types.Transaction) error {
	if err := p.addTx(tx); err != nil {
		p.logger.Error("failed to add tx", "err", err)
		return err
	}

	// broadcast the transaction only if network is enabled
	// and we are not in dev mode
	if p.topic != nil && !p.dev {
		txn := &proto.Txn{
			Raw: &any.Any{
				Value: tx.MarshalRLP(),
			},
		}

		if err := p.topic.Publish(txn); err != nil {
			p.logger.Error("failed to topic tx", "err", err)
		}
	}

	return nil
}

// handleGossipTxn handles receiving gossiped transactions
func (p *TxPool) handleGossipTxn(obj interface{}) {
	if !p.sealing {
		return
	}

	raw := obj.(*proto.Txn)
	tx := new(types.Transaction)
	if err := tx.UnmarshalRLP(raw.Raw.Value); err != nil {
		p.logger.Error("failed to decode broadcasted tx", "err", err)
		return
	}

	if err := p.addTx(tx); err != nil {
		p.logger.Error("failed to add broadcasted txn", "err", err)
	}
}

// addTx validates and checks if the tx can be added
// to the pool and sends an addRequest, if successful
func (p *TxPool) addTx(tx *types.Transaction) error {
	// validate recieved transaction
	if err := p.validateTx(tx); err != nil {
		return err
	}

	// check for overflow
	if p.gauge.read()+slotsRequired(tx) > p.gauge.max {
		return ErrTxPoolOverflow
	}

	// initialize account queue for this address [BLOCKING]
	p.createAccountOnce(tx.From)

	// send request [BLOCKING]
	p.addReqCh <- addRequest{tx: tx}

	return nil
}

// callback returning transition write status for given transaction
type WriteTxCallback = func(*types.Transaction) WriteTxStatus

// WriteTransactions is called from within consensus when the node is
// building a block and attempts to execute the provided callback for
// all the transactions currently present in the promoted queue.
// The pool needs to be aware of each execute status to keep
// its state valid.
func (p *TxPool) WriteTransactions(write WriteTxCallback) ([]*types.Transaction, int) {
	return nil, 0
}

// ResetWithHeader is called from within consensus when the node
// has received a new block from a peer. The pool needs to align
// its own state with the new one so it can correctly process
// further incoming transactions.
func (p *TxPool) ResetWithHeader(h *types.Header) {
}

// validateTx validates that the transaction conforms to specific constraints to be added to the txpool
func (p *TxPool) validateTx(tx *types.Transaction) error {
	// Check the transaction size to overcome DOS Attacks
	if uint64(len(tx.MarshalRLP())) > txMaxSize {
		return ErrOversizedData
	}

	// Check if the transaction has a strictly positive value
	if tx.Value.Sign() < 0 {
		return ErrNegativeValue
	}

	if !p.dev && tx.From != types.ZeroAddress {
		// Only if we are in dev mode we can accept
		// a transaction without validation
		return ErrNonEncryptedTx
	}

	// Check if the transaction is signed properly
	if tx.From == types.ZeroAddress {
		from, signerErr := p.signer.Sender(tx)
		if signerErr != nil {
			return ErrInvalidSender
		}

		tx.From = from
	}

	// Grab the state root for the latest block
	stateRoot := p.store.Header().StateRoot

	// Check nonce ordering
	if p.store.GetNonce(stateRoot, tx.From) > tx.Nonce {
		return ErrNonceTooLow
	}

	accountBalance, balanceErr := p.store.GetBalance(stateRoot, tx.From)
	if balanceErr != nil {
		return ErrInvalidAccountState
	}

	// Check if the sender has enough funds to execute the transaction
	if accountBalance.Cmp(tx.Cost()) < 0 {
		return ErrInsufficientFunds
	}

	// Make sure the transaction has more gas than the basic transaction fee
	intrinsicGas, err := state.TransactionGasCost(tx, p.forks.Homestead, p.forks.Istanbul)
	if err != nil {
		return err
	}

	if tx.Gas < intrinsicGas {
		return ErrIntrinsicGas
	}

	return nil
}

/* QUERY methods (to be revised) */

// GetNonce returns the next nonce for the account
// -> Returns the value from the TxPool if the account is initialized in-memory
// -> Returns the value from the world state otherwise
func (p *TxPool) GetNonce(addr types.Address) uint64 {
	return 0
}

// GetCapacity returns the current number of slots occupied and the max slot limit
func (p *TxPool) GetCapacity() (uint64, uint64) {
	return 0, 0
}

// NumAccountTxs Returns the number of transactions in the account specific queue
func (p *TxPool) NumAccountTxs(address types.Address) int {
	return 0
}

// GetTxs gets pending and queued transactions
func (p *TxPool) GetTxs(inclQueued bool) (map[types.Address]map[uint64]*types.Transaction, map[types.Address]map[uint64]*types.Transaction) {
	return nil, nil
}

// GetPendingTx returns the transaction by hash in the TxPool (pending txn) [Thread-safe]
func (t *TxPool) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	return nil, false
}

/* end of QUERY methods */

// createAccountOnce is used when discovering an address
// of a received transaction for the first time.
// This function ensures that account is created safely
// and only once.
func (p *TxPool) createAccountOnce(addr types.Address) {
	lock, _ := p.locks.LoadOrStore(addr, &accountLock{})
	accLock := lock.(*accountLock)

	accLock.initOnce.Do(func() {
		// first time initialize
		stateRoot := p.store.Header().StateRoot

		queue := newAccountQueue(p.logger.Named("account"))
		p.enqueued[addr] = queue

		// update nonce map
		nextNonce := p.store.GetNonce(stateRoot, addr)
		p.nextNonces.store(addr, nextNonce)
	})
}

func (p *TxPool) lockAccount(addr types.Address, write bool) {
	mux := p.locks.load(addr)
	if write {
		mux.Lock()
		atomic.StoreUint32(&mux.wLock, 1)
	} else {
		mux.RLock()
		atomic.StoreUint32(&mux.wLock, 0)
	}
}

func (p *TxPool) unlockAccount(addr types.Address) {
	// Grab the previous lock type and reset it
	mux := p.locks.load(addr)
	if atomic.SwapUint32(&mux.wLock, 0) == 1 {
		mux.Unlock()
	} else {
		mux.RUnlock()
	}
}

func (p *TxPool) lockPromoted(write bool) {
	if write {
		p.promoted.Lock()
		atomic.StoreUint32(&p.promoted.wLock, 1)
	} else {
		p.promoted.RLock()
		atomic.StoreUint32(&p.promoted.wLock, 0)
	}
}

func (p *TxPool) unlockPromoted() {
	// Grab the previous lock type and reset it
	if atomic.SwapUint32(&p.promoted.wLock, 0) == 1 {
		p.promoted.Unlock()
	} else {
		p.promoted.RUnlock()
	}
}

// slotsRequired() calculates the number of slotsRequired for given transaction
func slotsRequired(tx *types.Transaction) uint64 {
	return (tx.Size() + txSlotSize - 1) / txSlotSize
}

type status int

const (
	enqueued status = iota
	promoted
)

// Lookup map used to find transactions present in the pool
// map[types.Hash] -> status (enqueued or promoted)
type lookupMap struct {
	sync.Map
}

func (m *lookupMap) add(tx *types.Transaction, status status) {
	switch status {
	case enqueued:
		m.Store(tx.Hash, enqueued)
	case promoted:
		if _, ok := m.Load(tx.Hash); !ok {
			// not supposed to happen
			return
		}

		m.Store(tx.Hash, promoted)
	}
}

func (m *lookupMap) remove(hash types.Hash) {
	m.Delete(hash)
}

func (m *lookupMap) load(hash types.Hash) (*types.Transaction, bool) {
	tx, ok := m.Load(hash)
	if !ok {
		return nil, false
	}

	return tx.(*types.Transaction), true
}

// Map of expected nonces for all (known) accounts
// map[types.Address] -> nonce
type nonceMap struct {
	sync.Map
}

func (m *nonceMap) load(addr types.Address) (uint64, bool) {
	nonce, ok := m.Load(addr)
	if !ok {
		return 0, false
	}

	return nonce.(uint64), ok
}

func (m *nonceMap) store(addr types.Address, nonce uint64) {
	m.Store(addr, nonce)
}

// Read/write lock for each account queue
type accountLock struct {
	sync.RWMutex
	initOnce sync.Once
	wLock    uint32
}

// Thread safe map of all account locks
type accountLocks struct {
	sync.Map
}

func (m *accountLocks) load(addr types.Address) *accountLock {
	lock, ok := m.Load(addr)
	if !ok {
		return nil
	}

	return lock.(*accountLock)
}

func (m *accountLocks) store(addr types.Address, lock *accountLock) {
	m.Store(addr, lock)
}

type accountQueue struct {
	logger hclog.Logger
	txs    minNonceQueue
}

func newAccountQueue(logger hclog.Logger) *accountQueue {
	q := &accountQueue{
		txs:    minNonceQueue{},
		logger: logger,
	}

	heap.Init(&q.txs)
	return q
}

func (q *accountQueue) push(tx *types.Transaction) {
	// log
	heap.Push(&q.txs, tx)
}

func (q *accountQueue) pop() *types.Transaction {
	// log
	tx := heap.Pop(&q.txs)
	if tx == nil {
		return nil
	}

	return tx.(*types.Transaction)
}

func (q *accountQueue) peek() *types.Transaction {
	if len(q.txs) == 0 {
		return nil
	}

	return q.txs[0]
}

func (q *accountQueue) length() int {
	return q.txs.Len()
}

func (q *accountQueue) promote(nonce uint64) transactions {
	if tx := q.peek(); tx == nil ||
		tx.Nonce != nonce {
		return nil
	}

	var promotables transactions
	for {
		next := q.peek()
		if next == nil || next.Nonce != nonce {
			break
		}

		nonce += 1
		tx := q.pop()
		promotables = append(promotables, tx)
	}

	return promotables
}

func (q *accountQueue) prune(nonce uint64) transactions {
	var pruned transactions
	for {
		if next := q.peek(); next == nil ||
			next.Nonce > nonce {
			break
		}

		tx := q.pop()
		pruned = append(pruned, tx)
	}

	return pruned
}

type promotedQueue struct {
	sync.RWMutex
	logger hclog.Logger
	txs    maxPriceQueue
	wLock  uint32
}

func newPromotedQueue(logger hclog.Logger) *promotedQueue {
	q := &promotedQueue{
		txs:    maxPriceQueue{},
		logger: logger,
	}

	heap.Init(&q.txs)
	return q
}

func (q *promotedQueue) push(tx *types.Transaction) {
	// log
	heap.Push(&q.txs, tx)
}

func (q *promotedQueue) peek() *types.Transaction {
	if len(q.txs) == 0 {
		return nil
	}

	return q.txs[0]
}

func (q *promotedQueue) pop() *types.Transaction {
	// log
	tx := heap.Pop(&q.txs)
	if tx == nil {
		return nil
	}

	return tx.(*types.Transaction)
}

func (q *promotedQueue) length() int {
	return q.txs.Len()
}

// func (p *TxPool) prunteStalePromoted(nonceMap map[types.Address]uint64) transactions
func (q *promotedQueue) prune(nonceMap map[types.Address]uint64) transactions {
	var pruned transactions   // removed txs
	var reinsert transactions // valid txs

	for {
		tx := q.peek()
		if tx == nil {
			break
		}

		nonce, ok := nonceMap[tx.From]
		if !ok {
			// pool knows nothing of this address
			// so there can't be any promoted txs belonging to it
			continue
		}

		popped := q.pop()
		if tx.Nonce <= nonce {
			pruned = append(pruned, popped)
		} else {
			reinsert = append(reinsert, popped)
		}
	}

	// reinsert valid txs
	for _, tx := range reinsert {
		q.push(tx)
	}

	return pruned
}

/* queue implementations */

type transactions []*types.Transaction

type minNonceQueue transactions

func (q *minNonceQueue) Peek() *types.Transaction {
	if len(*q) == 0 {
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
	old := *q
	n := len(old)
	x := old[n-1]
	*q = old[0 : n-1]
	return x
}

type maxPriceQueue transactions

func (q *maxPriceQueue) Peek() *types.Transaction {
	if len(*q) == 0 {
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
	old := *q
	n := len(old)
	x := old[n-1]
	*q = old[0 : n-1]
	return x
}
