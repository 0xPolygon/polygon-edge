package txpool

import (
	"container/heap"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
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

type Config struct {
	MaxSlots uint64
	Sealing  bool
}

/* All requests are handled in the main loop */

// addRequest is sent when a transaction
// has gone through addTx successfully
// and is ready to be added to the pool.
type addRequest struct {
	tx *types.Transaction
	// isLocal bool

	// flag indicating the tx is returning
	// to the pool as a recovered one
	returnee bool
}

// promoteRequest is sent from handleAddRequest
// to signal that some account queue is ready
// for promotion.
//
// Occurs when a previously added transaction
// has does not have higher nonce than expected
// (indicated by nonceMap).
type promoteRequest struct {
	account types.Address
}

// resetRequest is handled when Ibft
// calls ResetWithHeader to align the state
// of the pool with the new block.
type resetRequest struct {
	newNonces map[types.Address]uint64
}

// TxPool is a module that handles pending transactions.
// There are fundamentally 2 queues any transaction
// needs to go through:
// - 1. Account queue (enqueued account transactions)
// - 2. Promoted queue (global pending transactions)
//
// The main difference between these queues is that
// account queues make sure a transaction is promoted
// in the correct (nonce) order. Promoted means
// the received transaction's nonce is expected for this account
// queue and can be moved to the promoted queue.
//
// The promoted queue acts as a sink to transactions
// promoted from any account queue sorted by max gasPrice
// where they wait to be inserted in the next block.
type TxPool struct {
	logger     hclog.Logger
	signer     signer
	forks      chain.ForksInTime
	store      store
	idlePeriod time.Duration

	// map of all account queues (accounts transactions)
	accounts accountMap

	// promoted transactions
	promoted *promotedQueue

	// next expected nonce for each account
	nextNonces nonceMap

	// Lookup map keeping track of all
	// transactions present in the pool
	index lookupMap

	// Networking stack
	topic *network.Topic

	// Gauge for measuring pool capacity
	gauge slotGauge

	// Channels on which the pool's event loop
	// does dispatching/handling requests.
	addReqCh     chan addRequest
	promoteReqCh chan promoteRequest
	resetReqCh   chan resetRequest

	// Flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing bool

	// Flag indicating if the current node is running in dev mode (used for testing)
	dev bool

	// Channel used by the dev consensus to be notified
	// anytime an account queue is promoted
	DevNotifyCh chan struct{}

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
		accounts:   accountMap{},
		promoted:   newPromotedQueue(logger.Named("promoted")),
		index:      lookupMap{all: make(map[types.Hash]*statusTx)},
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

	return pool, nil
}

// Start runs the pool's main loop in the background.
// On each request received, the appropriate handler
// is invoked in a separate goroutine.
func (p *TxPool) Start() error {
	go func() {
		for {
			select {
			case req := <-p.addReqCh:
				go p.handleAddRequest(req)
			case req := <-p.promoteReqCh:
				go p.handlePromoteRequest(req)
			case req := <-p.resetReqCh:
				go p.handleResetRequest(req)
			}
		}
	}()

	return nil
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

// Pop() removes the highest priced transaction from the promoted queue.
// Assumes the lock is held.
func (p *TxPool) Pop() *types.Transaction {
	tx := p.promoted.pop()
	if tx == nil {
		return nil
	}

	// update state
	p.gauge.decrease(slotsRequired(tx))

	return tx
}

// Recover is called within ibft for all transactions
// that are valid but couldn't be written to the state
// at the given time. Issues an addRequest to the pool
// indicating a transaction is returning to it.
func (p *TxPool) Recover(tx *types.Transaction) {
	p.addReqCh <- addRequest{tx: tx, returnee: true}
}

// Rollback is called within ibft for any transactions
// deemed unrecoverable during writing to the state.
// This call ensures that any subsequent transactions
// must not be processed before the unrecoverable one
// is re-sent again.
func (p *TxPool) RollbackNonce(tx *types.Transaction) {
	if nextNonce, ok := p.nextNonces.load(tx.From); ok && nextNonce < tx.Nonce {
		// already did rollback
		return
	}

	p.nextNonces.store(tx.From, tx.Nonce)
}

// ResetWithHeader is called from within ibft when the node
// has received a new block from a peer. The pool needs to align
// its own state with the new one so it can correctly process
// further incoming transactions.
func (p *TxPool) ResetWithHeader(h *types.Header) {
	e := &blockchain.Event{
		NewChain: []*types.Header{h},
	}

	// process the txs in the event to make sure the pool is up-to-date
	p.processEvent(e)
}

func (p *TxPool) processEvent(event *blockchain.Event) {
	// transactions collected from OldChain
	oldTransactions := make(map[types.Hash]*types.Transaction)

	// Legacy reorg logic //
	for _, header := range event.OldChain {
		// transactios to be returned to the pool
		block, ok := p.store.GetBlockByHash(header.Hash, true)
		if !ok {
			continue
		}

		for _, tx := range block.Transactions {
			oldTransactions[tx.Hash] = tx
		}
	}

	// Grab the latest state root now that the block has been inserted
	stateRoot := p.store.Header().StateRoot

	// discover latest (next) nonces for all transactions in the new block
	stateNonces := make(map[types.Address]uint64)
	for _, event := range event.NewChain {
		block, ok := p.store.GetBlockByHash(event.Hash, true)
		if !ok {
			continue
		}

		p.index.remove(block.Transactions...)

		// determine latest nonces for all known accounts
		for _, tx := range block.Transactions {
			addr := tx.From

			// skip already processed accounts
			if _, processed := stateNonces[addr]; processed {
				continue
			}

			// fetch latest nonce from the state
			latestNonce := p.store.GetNonce(stateRoot, addr)

			// update the result map
			stateNonces[addr] = latestNonce

			// Legacy reorg logic //
			// Update the addTxns in case of reorgs
			delete(oldTransactions, tx.Hash)
		}
	}

	if len(stateNonces) == 0 {
		return
	}

	// Signal reset request
	p.resetReqCh <- resetRequest{newNonces: stateNonces}
}

// validateTx ensures that the transaction conforms
// to specific constraints before entering the pool.
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

// addTx is the main entry point to the pool
// for all received transactions. If the call to
// addTx() is successful an account queue is created
// for this address (only once) and an addRequest is sent.
func (p *TxPool) addTx(tx *types.Transaction) error {
	// validate recieved transaction
	if err := p.validateTx(tx); err != nil {
		return err
	}

	// check for overflow
	if p.gauge.read()+slotsRequired(tx) > p.gauge.max {
		return ErrTxPoolOverflow
	}

	tx.ComputeHash()

	// check if already known
	if _, ok := p.index.load(tx.Hash); ok {
		return ErrAlreadyKnown
	}

	// initialize account queue for this address once
	p.createAccountOnce(tx.From)

	// send request [BLOCKING]
	p.addReqCh <- addRequest{tx: tx, returnee: false}

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

// handleAddRequest is invoked when a new transaction is received
// (result of a successful addTx() call) or a recovered transaction
// is re-entering the pool.
//
// A transaction handled within this request can either be
// dropped or enqueued, eventually signaling promotion in the former case.
func (p *TxPool) handleAddRequest(req addRequest) {
	tx := req.tx
	addr := req.tx.From

	account := p.lockAccount(addr, true)
	defer p.unlockAccount(addr)

	// fetch store from nonce map
	nextNonce, _ := p.nextNonces.load(addr)
	if tx.Nonce < nextNonce && !req.returnee {
		// reject new txs with nonce
		// lower than expected
		return
	}

	// enqueue tx
	account.enqueue(tx)

	// atomically increase gauge
	p.gauge.increase(slotsRequired(tx))

	// update lookup
	p.index.add(enqueued, tx)

	if tx.Nonce > nextNonce {
		// don't signal promotion
		// for high nonce tx
		return
	}

	// account queue is ready for promotion:
	// 	1. New tx is matching nonce expected
	// 	2. We are promoting a recovered tx
	p.promoteReqCh <- promoteRequest{account: addr} // BLOCKING
}

// handlePromoteRequest handles moving promtable transactions
// from the associated account queue to the promoted queue.
// Can only be invoked by handleAddRequest
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
	addr := req.account

	p.LockPromoted(true)
	defer p.UnlockPromoted()

	account := p.lockAccount(addr, true)
	defer p.unlockAccount(addr)

	// fetch next expected nonce for this account
	nextNonce, _ := p.nextNonces.load(addr)

	// pop promotable txs
	promotables, newNonce := account.promote(nextNonce)

	// push to promotables
	p.promoted.push(promotables...)

	// update index map (enqueued -> promoted)
	p.index.add(promoted, promotables...)

	if p.dev {
		// notify the dev consensus
		select {
		case p.DevNotifyCh <- struct{}{}:
		default:
		}
	}

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	// otherwise it means we just promoted a previously recovered tx
	if newNonce > nextNonce {
		p.nextNonces.store(addr, newNonce)
	}
}

// handleResetRequest is called within ResetWithHeader
// and aligns the pool's state for all accounts by pruning
// stale transactions from all queues in the pool.
func (p *TxPool) handleResetRequest(req resetRequest) {
	p.LockPromoted(true)
	defer p.UnlockPromoted()

	newNonces := req.newNonces

	pruned := p.prunePromoted(newNonces)
	p.logger.Debug(fmt.Sprintf("pruned %d promoted transactions", pruned))

	p.pruneAccounts(newNonces)
}

// prunePromoted cleans out any transactions from the promoted queue
// considered stale by the given nonceMap.
func (p *TxPool) prunePromoted(nonceMap map[types.Address]uint64) uint64 {
	// extract valid and stale txs
	valid, pruned := p.promoted.prune(nonceMap)

	// remove from index
	p.index.remove(pruned...)

	// free up slots
	p.gauge.decrease(slotsRequired(pruned...))

	// reinsert valid txs
	p.promoted.push(valid...)

	return uint64(len(pruned))
}

// pruneAccounts cleans out any transactions from the accouunt queues
// considered stale by the given nonceMap.
func (p *TxPool) pruneAccounts(stateNonces map[types.Address]uint64) {
	var wg sync.WaitGroup
	for addr, nonce := range stateNonces {
		mapNonce, ok := p.nextNonces.load(addr)
		if !ok {
			// unknown addr -> no account to prune
			continue
		}

		if nonce <= mapNonce {
			// only the promoted queue needed
			// to be pruned for this addr
			continue
		}

		wg.Add(1)
		go func(addr types.Address, nonce uint64) {
			defer wg.Done()
			_, _ = p.pruneAccount(addr, nonce)
			// debug log
		}(addr, nonce)
	}

	// wait for all accounts to be cleared
	wg.Wait()
}

// pruneAccount clears all transactions with nonce lower than given
// and updates the nonce map. If when done pruning, the next transaction
// has nonce that matches the newdly expected, a promotion is signaled.
func (p *TxPool) pruneAccount(addr types.Address, nonce uint64) (numPruned, numRemaining uint64) {
	account := p.lockAccount(addr, true)
	defer p.unlockAccount(addr)

	pruned := account.prune(nonce)

	// free up slots
	p.gauge.decrease(slotsRequired(pruned...))

	// remove from index
	p.index.remove(pruned...)

	if tx := account.first(); tx != nil &&
		tx.Nonce == nonce {
		// first tx matches next expected nonce -> signal promotion
		p.promoteReqCh <- promoteRequest{addr}
	}

	// update next nonce
	p.nextNonces.store(addr, nonce)

	numPruned = uint64(len(pruned))
	numRemaining = account.length()
	return
}

// createAccountOnce is used when discovering an address
// of a received transaction for the first time.
// This function ensures that the account queue and its corresponding lock
// are  created safely and only once.
func (p *TxPool) createAccountOnce(newAddr types.Address) {
	a, _ := p.accounts.LoadOrStore(newAddr, &account{})
	account := a.(*account)

	// run only once per queue creation
	account.initFunc.Do(func() {
		account.queue = newMinNonceQueue()
		account.logger = p.logger.Named("account")

		// update nonce map
		stateRoot := p.store.Header().StateRoot
		nextNonce := p.store.GetNonce(stateRoot, newAddr)
		p.nextNonces.store(newAddr, nextNonce)
	})
}

/* QUERY methods (to be revised) */

// GetNonce returns the next nonce for the account
// -> Returns the value from the TxPool if the account is initialized in-memory
// -> Returns the value from the world state otherwise
func (p *TxPool) GetNonce(addr types.Address) uint64 {
	nonce, ok := p.nextNonces.load(addr)
	if !ok {
		stateRoot := p.store.Header().StateRoot
		stateNonce := p.store.GetNonce(stateRoot, addr)

		return stateNonce
	}

	return nonce
}

// GetCapacity returns the current number of slots occupied and the max slot limit
func (p *TxPool) GetCapacity() (uint64, uint64) {
	return p.gauge.read(), p.gauge.max
}

// GetTxs gets pending and queued transactions
func (p *TxPool) GetTxs(inclQueued bool) (map[types.Address]map[uint64]*types.Transaction, map[types.Address]map[uint64]*types.Transaction) {
	return nil, nil
}

// GetPendingTx returns the transaction by hash in the TxPool (pending txn) [Thread-safe]
func (p *TxPool) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	statusTx, ok := p.index.load(txHash)
	if !ok {
		return nil, false
	}

	return statusTx.tx, true
}

/* end of QUERY methods */

// status represents a transaction's pending state.
type status int

const (
	enqueued status = iota
	promoted
)

// Transaction wrapper object inidcating the status of a transaction
// currently present in the pool. Used by lookUpMap.
type statusTx struct {
	tx       *types.Transaction
	promoted bool
}

// Lookup map used to find transactions present in the pool
type lookupMap struct {
	sync.RWMutex
	all map[types.Hash]*statusTx
}

// add adds the given transaction to the lookup map
func (m *lookupMap) add(status status, txs ...*types.Transaction) {
	m.Lock()
	defer m.Unlock()

	switch status {
	case enqueued:
		for _, tx := range txs {
			m.all[tx.Hash] = &statusTx{
				tx:       tx,
				promoted: false,
			}
		}
	case promoted:
		for _, tx := range txs {
			m.all[tx.Hash] = &statusTx{
				tx:       tx,
				promoted: true,
			}
		}
	}
}

// remove clears the lookup map of given txs
func (m *lookupMap) remove(txs ...*types.Transaction) {
	m.Lock()
	defer m.Unlock()

	for _, tx := range txs {
		if _, ok := m.all[tx.Hash]; !ok {
			continue
		}
		delete(m.all, tx.Hash)
	}
}

// load acquires the read lock on the lookup map and returns the requested
// transaction (wrapped in a status object), if it exists
func (m *lookupMap) load(hash types.Hash) (*statusTx, bool) {
	m.RLock()
	defer m.RUnlock()

	statusTx, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return statusTx, true
}

// Map of expected nonces for all (known) accounts
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

// Thread safe map of all account queue registered by the pool
type accountMap struct {
	sync.Map
}

// from returns the account queue of the gives address
func (m *accountMap) from(addr types.Address) *account {
	a, ok := m.Load(addr)
	if !ok {
		return nil
	}

	return a.(*account)
}

// lockAccount locks the account queue of the given address
func (p *TxPool) lockAccount(addr types.Address, write bool) *account {
	account := p.accounts.from(addr)
	if write {
		account.Lock()
		atomic.StoreUint32(&account.wLock, 1)
	} else {
		account.RLock()
		atomic.StoreUint32(&account.wLock, 0)
	}

	return account
}

// unlockAccount unlock the account queue of the given address
func (p *TxPool) unlockAccount(addr types.Address) {
	// Grab the previous lock type and reset it
	account := p.accounts.from(addr)
	if atomic.SwapUint32(&account.wLock, 0) == 1 {
		account.Unlock()
	} else {
		account.RUnlock()
	}
}

/* account (queue) impl */
type account struct {
	sync.RWMutex
	initFunc sync.Once
	logger   hclog.Logger
	wLock    uint32
	queue    minNonceQueue
}

func (a *account) enqueue(txs ...*types.Transaction) {
	for _, tx := range txs {
		heap.Push(&a.queue, tx)
	}
}

func (a *account) length() uint64 {
	return uint64(a.queue.Len())
}

// first peeks at the first transaction
// in the account queue, if any
func (a *account) first() *types.Transaction {
	return a.queue.Peek()
}

func (a *account) promote(nonce uint64) (transactions, uint64) {
	tx := a.queue.Peek()
	if tx == nil ||
		tx.Nonce > nonce {
		return nil, 0
	}

	var promotables transactions
	nextNonce := tx.Nonce
	for {
		tx := a.queue.Peek()
		if tx == nil ||
			tx.Nonce != nextNonce {
			break
		}

		tx = heap.Pop(&a.queue).(*types.Transaction)
		promotables = append(promotables, tx)
		nextNonce += 1
	}

	return promotables, nextNonce
}

func (a *account) prune(nonce uint64) transactions {
	var pruned transactions
	for {
		next := a.queue.Peek()
		if next == nil ||
			next.Nonce >= nonce {
			break
		}

		tx := heap.Pop(&a.queue).(*types.Transaction)
		pruned = append(pruned, tx)
	}

	return pruned
}

/* promoted queue impl */

func (p *TxPool) LockPromoted(write bool) {
	if write {
		p.promoted.Lock()
		atomic.StoreUint32(&p.promoted.wLock, 1)
	} else {
		p.promoted.RLock()
		atomic.StoreUint32(&p.promoted.wLock, 0)
	}
}

func (p *TxPool) UnlockPromoted() {
	// Grab the previous lock type and reset it
	if atomic.SwapUint32(&p.promoted.wLock, 0) == 1 {
		p.promoted.Unlock()
	} else {
		p.promoted.RUnlock()
	}
}

type promotedQueue struct {
	sync.RWMutex
	logger hclog.Logger
	queue  maxPriceQueue
	wLock  uint32
}

func newPromotedQueue(logger hclog.Logger) *promotedQueue {
	q := &promotedQueue{
		queue:  newMaxPriceQueue(),
		logger: logger,
	}

	return q
}

func (q *promotedQueue) push(txs ...*types.Transaction) {
	for _, tx := range txs {
		heap.Push(&q.queue, tx)
	}
}

func (q *promotedQueue) peek() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return q.queue.txs[0]
}

func (q *promotedQueue) pop() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*types.Transaction)
}

func (q *promotedQueue) length() uint64 {
	return uint64(q.queue.Len())
}

func (q *promotedQueue) prune(nonceMap map[types.Address]uint64) (valid, pruned transactions) {
	for {
		next := q.peek()
		if next == nil {
			break
		}

		tx := q.pop()
		nonce := nonceMap[tx.From]

		// prune stale
		if tx.Nonce < nonce {
			pruned = append(pruned, tx)
			continue
		}

		valid = append(valid, tx)
	}

	return
}

// Gauge for measuring pool capacity in slots
type slotGauge struct {
	height uint64
	max    uint64
}

// slotsRequired calculates the number of slots required for given transaction(s)
func slotsRequired(txs ...*types.Transaction) uint64 {
	slots := uint64(0)
	for _, tx := range txs {
		slots += func(tx *types.Transaction) uint64 {
			return (tx.Size() + txSlotSize - 1) / txSlotSize
		}(tx)
	}

	return slots
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
