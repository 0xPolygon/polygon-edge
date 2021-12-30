package txpool

import (
	"container/heap"
	"errors"
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

type txOrigin int

const (
	local  txOrigin = iota // json-RPC/gRPC endpoints
	gossip                 // gossip protocol
	reorg                  // legacy code
)

func (o txOrigin) String() (s string) {
	switch o {
	case local:
		s = "local"
	case gossip:
		s = "gossip"
	case reorg:
		s = "reorg"
	}

	return
}

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
	// to the pool as a recovered one (see Demote)
	demoted bool
}

// promoteRequest is sent from handleAddRequest
// to signal that some account queue is ready
// for promotion.
//
// Occurs when a transactopn with nonce expected
// is received or a demoted one is re-entering the pool.
type promoteRequest struct {
	account types.Address
}

// TxPool is a module that handles pending transactions.
// There are fundamentally 2 queues any transaction
// needs to go through:
// - 1. Account queue (enqueued transactions for specific address)
// - 2. Promoted queue (global pending transactions)
//
// The main difference between these queues is that
// account queues make sure a transaction is promoted
// in the correct (nonce) order. Promoted means
// the received transaction's nonce is expected for this account
// queue and can be moved to the promoted queue.
//
// The promoted queue acts as a sink for transactions
// promoted from any account queue, sorted by max gasPrice
// where they wait to be inserted in the next block.
type TxPool struct {
	logger     hclog.Logger
	signer     signer
	forks      chain.ForksInTime
	store      store
	idlePeriod time.Duration

	// map of all account queues (accounts transactions)
	accounts accountsMap

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

/// NewTxPool creates a new pool for incoming transactions.
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
		accounts:   accountsMap{},
		promoted:   newPromotedQueue(),
		index:      lookupMap{all: make(map[types.Hash]*types.Transaction)},
		gauge:      slotGauge{height: 0, max: config.MaxSlots},
		sealing:    config.Sealing,
	}

	if network != nil {
		// subscribe to the gossip protocol
		topic, err := network.NewTopic(topicNameV1, &proto.Txn{})
		if err != nil {
			return nil, err
		}

		topic.Subscribe(pool.handleGossipTx)
		pool.topic = topic
	}

	if grpcServer != nil {
		proto.RegisterTxnPoolOperatorServer(grpcServer, pool)
	}

	// initialise channels
	pool.addReqCh = make(chan addRequest)
	pool.promoteReqCh = make(chan promoteRequest)

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
			}
		}
	}()

	return nil
}

func (p *TxPool) AddSigner(s signer) {
	p.signer = s
}

// Enables dev mode so the pool can accept non-ecnrypted transactions.
// Used in testing.
func (p *TxPool) EnableDev() {
	p.dev = true
}

// AddTx adds a new transaction to the pool (sent from json-RPC/gRPC endpoints)
// and broadcasts it if networking is enabled.
func (p *TxPool) AddTx(tx *types.Transaction) error {
	if err := p.addTx(local, tx); err != nil {
		p.logger.Error("failed to add tx", "err", err)
		return err
	}

	// broadcast the transaction only if network is enabled
	// and we are not in dev mode
	if p.topic != nil && !p.dev {
		tx := &proto.Txn{
			Raw: &any.Any{
				Value: tx.MarshalRLP(),
			},
		}

		if err := p.topic.Publish(tx); err != nil {
			p.logger.Error("failed to topic tx", "err", err)
		}
	}

	return nil
}

// Returns the first transaction from the promoted queue
// without removing it. Assumes the lock is held.
func (p *TxPool) Peek() *types.Transaction {
	return p.promoted.peek()
}

// Removes and returns the first transaction from the promoted queue.
// Assumes the lock is held.
func (p *TxPool) Pop() *types.Transaction {
	tx := p.promoted.pop()
	p.logger.Debug("pop tx", "hash", tx.Hash.String())

	// update state
	p.gauge.decrease(slotsRequired(tx))

	// update metrics
	p.metrics.PendingTxs.Set(float64(p.promoted.length()))

	return tx
}

// Drop is called within ibft for any transaction
// deemed unrecoverable during writing to the state.
// This call ensures that any subsequent transactions
// must not be processed before the unrecoverable one
// is re-sent again for that account.
func (p *TxPool) Drop() {
	tx := p.Pop()
	p.logger.Debug("drop tx", "hash", tx.Hash.String())

	// remove from index
	p.index.remove(tx)

	// update metrics
	p.metrics.PendingTxs.Set(float64(p.promoted.length()))

	// rollback nonce
	p.nextNonces.store(tx.From, tx.Nonce)
	p.logger.Debug("rollback next nonce",
		"nonce", tx.Nonce,
		"addr", tx.From.String(),
	)
}

// Demote is called within ibft for all transactions
// that are valid but couldn't be written to the state
// at the given time (recoverable). Issues an addRequest to the pool
// indicating a transaction is returning to it.
func (p *TxPool) Demote() {
	tx := p.promoted.pop()
	p.logger.Debug("demoted transaction", "hash", tx.Hash.String())

	// update metrics
	p.metrics.PendingTxs.Set(float64(p.promoted.length()))

	// signal add request [BLOCKING]
	p.addReqCh <- addRequest{tx: tx, demoted: true}
}

// ResetWithHeaders is called from within ibft when the node
// has received a new block from a peer. The pool needs to align
// its own state with the new one so it can correctly process
// further incoming transactions.
func (p *TxPool) ResetWithHeaders(headers ...*types.Header) {
	e := &blockchain.Event{
		NewChain: headers,
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

	// discover latest (next) nonces for all transactions in the NewChain
	stateNonces := make(map[types.Address]uint64)
	for _, header := range event.NewChain {
		block, ok := p.store.GetBlockByHash(header.Hash, true)
		if !ok {
			p.logger.Error("could not find block in store", "hash", header.Hash.String())
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

	// Legacy reorg logic //
	for _, tx := range oldTransactions {
		p.addTx(reorg, tx)
	}

	if len(stateNonces) == 0 {
		return
	}

	// reset with the new state
	p.resetQueues(stateNonces)
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
// for any newly received transaction. If the call to
// addTx is successful an account is created
// for this address (only once) and an addRequest is signaled.
func (p *TxPool) addTx(origin txOrigin, tx *types.Transaction) error {
	p.logger.Debug("add tx",
		"origin", origin.String(),
		"hash", tx.Hash.String(),
	)

	// validate incoming tx
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
		if origin == gossip {
			// silently drop known tx that is gossiped back
			p.logger.Debug("dropping known gossiped transaction", "hash", tx.Hash.String())
			return nil
		} else {
			return ErrAlreadyKnown
		}
	}

	// initialize account queue for this address once
	p.createAccountOnce(tx.From)

	// send request [BLOCKING]
	p.addReqCh <- addRequest{tx: tx, demoted: false}

	return nil
}

// handleGossipTx handles receiving transactions
// gossiped by the network.
func (p *TxPool) handleGossipTx(obj interface{}) {
	if !p.sealing {
		return
	}

	raw := obj.(*proto.Txn)
	tx := new(types.Transaction)
	if err := tx.UnmarshalRLP(raw.Raw.Value); err != nil {
		p.logger.Error("failed to decode broadcasted tx", "err", err)
		return
	}

	if err := p.addTx(gossip, tx); err != nil {
		p.logger.Error("failed to add broadcasted txn", "err", err)
	}
}

// handleAddRequest is invoked when a new transaction is received
// (result of a successful addTx() call) or a demoted transaction
// is re-entering the pool.
//
// A transaction handled within this request can either be
// dropped or enqueued, eventually signaling promotion in the latter case.
func (p *TxPool) handleAddRequest(req addRequest) {
	tx := req.tx
	addr := req.tx.From

	account := p.accounts.lock(addr, true)
	defer p.accounts.unlock(addr)

	// fetch expected nonce
	nextNonce, _ := p.nextNonces.load(addr)

	// reject new txs with nonce
	// lower than expected
	if tx.Nonce < nextNonce && !req.demoted {
		p.logger.Debug("rejecting low nonce tx",
			"nonce", tx.Nonce,
			"expected", nextNonce,
			"hash", tx.Hash.String(),
		)
		return
	}

	// enqueue tx
	account.enqueue(tx)
	p.logger.Debug("enqueue account",
		"tx_nonce", tx.Nonce,
		"expected_nonce", nextNonce,
		"enqueued", account.length(),
		"addr", addr.String(),
	)

	// update lookup
	p.index.add(tx)

	// demoted transactions never decrease the gauge
	if !req.demoted {
		p.gauge.increase(slotsRequired(tx))
	}

	if tx.Nonce > nextNonce {
		// don't signal promotion
		// for high nonce tx
		return
	}

	// account queue is ready for promotion:
	// 	1. New tx is matching nonce expected
	// 	2. We are promoting a demoted tx
	p.promoteReqCh <- promoteRequest{account: addr} // BLOCKING
}

// handlePromoteRequest handles moving promtable transactions
// from the associated account queue to the promoted queue.
// Can only be invoked by handleAddRequest.
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
	addr := req.account

	p.LockPromoted(true)
	defer p.UnlockPromoted()

	account := p.accounts.lock(addr, true)
	defer p.accounts.unlock(addr)

	if account.length() == 0 {
		// nothing to promote
		// (means a previous handler already did the job)
		return
	}

	// fetch next expected nonce for this account
	nextNonce, _ := p.nextNonces.load(addr)

	// pop promotable txs
	promotables, newNonce := account.promote(nextNonce)

	// push to promotables
	p.promoted.push(promotables...)

	// update metrics
	p.metrics.PendingTxs.Set(float64(p.promoted.length()))

	p.logger.Debug("promote account",
		"promoted", len(promotables),
		"remaining", account.length(),
		"addr", addr.String(),
	)

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	// otherwise it means we just promoted a previously demoted tx
	if newNonce > nextNonce {
		p.nextNonces.store(addr, newNonce)
		p.logger.Debug("increased next nonce", "nonce", newNonce, "addr", addr.String())
	}

	// log updated promoted queue length
	p.logger.Debug("promoted queue", "length", p.promoted.length())
}

// resetQueues is called within ResetWithHeader
// to align the pool's state with the state from the new header.
// Removes any stale transctions from the account queues based on
// the new state.
func (p *TxPool) resetQueues(stateNonces map[types.Address]uint64) {
	p.LockPromoted(true)
	defer p.UnlockPromoted()

	// prune promoted txs
	p.prunePromoted(stateNonces)

	// prune enqueued txs
	p.pruneAccounts(stateNonces)
}

// prunePromoted cleans out any transactions from the promoted queue
// considered stale by the given nonceMap.
func (p *TxPool) prunePromoted(nonceMap map[types.Address]uint64) {
	// extract valid and stale txs
	valid, pruned := p.promoted.prune(nonceMap)

	// reinsert valid txs
	p.promoted.push(valid...)

	if len(pruned) == 0 {
		// promoted queue contains
		// no stale txs
		return
	}

	// remove from index
	p.index.remove(pruned...)

	// free up slots
	p.gauge.decrease(slotsRequired(pruned...))

	// update metrics
	p.metrics.PendingTxs.Set(float64(p.promoted.length()))

	p.logger.Debug("pruned promoted queue", "num", len(pruned))
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
			// stale txs were only
			// in the promoted queue
			continue
		}

		wg.Add(1)
		go func(addr types.Address, nonce uint64) {
			defer wg.Done()
			p.pruneAccount(addr, nonce)

		}(addr, nonce)
	}

	// wait for all accounts to be cleared
	wg.Wait()
}

// pruneAccount removes all transactions with nonce lower than given
// and updates the nonce map. If when done pruning, the next transaction
// has nonce that matches the newly updated, a promotion is signaled.
func (p *TxPool) pruneAccount(addr types.Address, nonce uint64) {
	account := p.accounts.lock(addr, true)
	defer p.accounts.unlock(addr)

	// update next nonce
	p.nextNonces.store(addr, nonce)
	p.logger.Debug("next nonce updated",
		"nonce", nonce,
		"addr", addr.String(),
	)

	// prune enqueued
	pruned := account.prune(nonce)
	p.logger.Debug("pruned account",
		"num", len(pruned),
		"addr", addr.String(),
	)

	// free up slots
	p.gauge.decrease(slotsRequired(pruned...))

	// remove from index
	p.index.remove(pruned...)

	// check if account is promotable after pruning
	if tx := account.first(); tx != nil &&
		tx.Nonce == nonce {
		// first tx matches next (expected) nonce -> signal promotion
		p.promoteReqCh <- promoteRequest{addr}
	}
}

// createAccountOnce is used when discovering an address
// of a received transaction for the first time.
// This function ensures that the account queue (and its corresponding lock)
// is created atomically and only once.
func (p *TxPool) createAccountOnce(newAddr types.Address) {
	a, _ := p.accounts.LoadOrStore(newAddr, &account{})
	newAccount := a.(*account)

	// run only once per account creation
	newAccount.initFunc.Do(func() {
		newAccount.queue = newMinNonceQueue()

		// update nonce map
		stateRoot := p.store.Header().StateRoot
		nextNonce := p.store.GetNonce(stateRoot, newAddr)
		p.nextNonces.store(newAddr, nextNonce)

		p.logger.Debug("created new account", "addr", newAddr.String(), "next_nonce", nextNonce)
	})
}

/* QUERY methods */

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

// GetCapacity returns the current number of slots
// occupied in the pool as well as the max limit
func (p *TxPool) GetCapacity() (uint64, uint64) {
	return p.gauge.read(), p.gauge.max
}

// GetPendingTx returns the transaction by hash in the TxPool (pending txn) [Thread-safe]
func (p *TxPool) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	tx, ok := p.index.load(txHash)
	if !ok {
		return nil, false
	}

	return tx, true
}

// GetTxs gets pending and queued transactions
func (p *TxPool) GetTxs(inclQueued bool) (
	promoted, enqueued map[types.Address][]*types.Transaction,
) {
	// lock the promoted queue to prevent
	// promotion handlers from mutating it
	// and the account queues
	p.LockPromoted(false)
	defer p.UnlockPromoted()

	// collect promoted
	promoted = p.parsePromoted()
	if !inclQueued {
		return promoted, nil
	}

	// collect enqueued
	enqueued = p.parseEnqueued()

	return
}

// parsePromoted parses the promoted queue into a map collection of
// {k: address} -> {v: transactions} where transactions are sorted by nonce.
func (p *TxPool) parsePromoted() (
	parsed map[types.Address][]*types.Transaction,
) {
	promoted := make(map[types.Address]*minNonceQueue)

	// push onto a temporary queue
	// so txs can be sorted
	for _, tx := range p.promoted.queue.txs {
		if _, ok := promoted[tx.From]; !ok {
			ptr := new(minNonceQueue)
			(*ptr) = newMinNonceQueue()
			promoted[tx.From] = ptr
		}

		promoted[tx.From].Push(tx)
	}

	parsed = make(map[types.Address][]*types.Transaction)
	for addr, queue := range promoted {
		parsed[addr] = queue.txs
	}

	return
}

// parseEnqueued parses all account queues into a map collection of
// {k: address} -> {v: transactions} where transactions are sorted by nonce.
func (p *TxPool) parseEnqueued() (
	parsed map[types.Address][]*types.Transaction,
) {
	parsed = make(map[types.Address][]*types.Transaction)

	// range across all accounts
	// and collect enqueued txs
	p.accounts.Range(func(key, value interface{}) bool {
		addr := key.(types.Address)

		account := p.accounts.lock(addr, false)
		defer p.accounts.unlock(addr)

		parsed[addr] = account.queue.txs

		return true
	})

	return
}

/* end of QUERY methods */

// Lookup map used to find transactions present in the pool
type lookupMap struct {
	sync.RWMutex
	all map[types.Hash]*types.Transaction
}

// Adds the given transaction to the map. [thread-safe]
func (m *lookupMap) add(txs ...*types.Transaction) {
	m.Lock()
	defer m.Unlock()

	for _, tx := range txs {
		m.all[tx.Hash] = tx
	}
}

// Removes the given transactions from the map. [thread-safe]
func (m *lookupMap) remove(txs ...*types.Transaction) {
	m.Lock()
	defer m.Unlock()

	for _, tx := range txs {
		delete(m.all, tx.Hash)
	}
}

// Returns the transaction associated with the given hash. [thread-safe]
func (m *lookupMap) load(hash types.Hash) (*types.Transaction, bool) {
	m.RLock()
	defer m.RUnlock()

	tx, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return tx, true
}

// Map of expected nonces for all accounts (known by the pool).
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

// Thread safe map of all accounts registered by the pool.
type accountsMap struct {
	sync.Map
}

// Returns the account associated with the given address.
func (m *accountsMap) from(addr types.Address) *account {
	a, ok := m.Load(addr)
	if !ok {
		return nil
	}

	return a.(*account)
}

// Locks and returns the account associated with addr.
// Write flag sets the lock mode.
func (m *accountsMap) lock(addr types.Address, write bool) *account {
	account := m.from(addr)
	if write {
		account.Lock()
		atomic.StoreUint32(&account.wLock, 1)
	} else {
		account.RLock()
		atomic.StoreUint32(&account.wLock, 0)
	}

	return account
}

// Unlocks the account associated with addr.
func (m *accountsMap) unlock(addr types.Address) {
	account := m.from(addr)
	if atomic.SwapUint32(&account.wLock, 0) == 1 {
		account.Unlock()
	} else {
		account.RUnlock()
	}
}

/* account (queue) impl */

// account is a thread-safe wrapper object
// around an account queue (enqueued transactions).
//
// Accounts are only ever initalized once and stored
// atomically in the accountsMap.
//
// All methods assume the approriate lock is held
// and at the right time (context).
type account struct {
	sync.RWMutex
	initFunc sync.Once
	wLock    uint32
	queue    minNonceQueue
}

// Pushes the transactions onto to the account queue.
func (a *account) enqueue(txs ...*types.Transaction) {
	for _, tx := range txs {
		heap.Push(&a.queue, tx)
	}
}

// Returns the number of enqueued transactions for this account.
func (a *account) length() uint64 {
	return uint64(a.queue.Len())
}

// Returns the first enqueued transaction, if present.
func (a *account) first() *types.Transaction {
	return a.queue.Peek()
}

// Promote tries to pop transactions from the account queue
// that can be considered promotable.
//
// Promotable transactions are all sequential in the order of nonce
// and the first one has to have nonce not greater than the one passed in
// as argument.
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

// Removes all transactions with nonce lower than given.
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

// Locks the promoted queue in read/write mode
// depending on the write flag.
func (p *TxPool) LockPromoted(write bool) {
	if write {
		p.promoted.Lock()
		atomic.StoreUint32(&p.promoted.wLock, 1)
	} else {
		p.promoted.RLock()
		atomic.StoreUint32(&p.promoted.wLock, 0)
	}
}

// Unlocks the promoted queue.
func (p *TxPool) UnlockPromoted() {
	// Grab the previous lock type and reset it
	if atomic.SwapUint32(&p.promoted.wLock, 0) == 1 {
		p.promoted.Unlock()
	} else {
		p.promoted.RUnlock()
	}
}

/* promoted queue impl */

type promotedQueue struct {
	sync.RWMutex
	queue maxPriceQueue
	wLock uint32
}

// Creates and returns a new promoted (max price) queue.
func newPromotedQueue() *promotedQueue {
	q := &promotedQueue{
		queue: newMaxPriceQueue(),
	}

	return q
}

/* all methods assume the appropriate lock is held. */

// Pushes the given transactions onto the queue.
func (q *promotedQueue) push(txs ...*types.Transaction) {
	for _, tx := range txs {
		heap.Push(&q.queue, tx)
	}
}

// Returns the first transaction from the queue without removing it.
func (q *promotedQueue) peek() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return q.queue.txs[0]
}

// Removes the first transactions from the queue and returns it.
func (q *promotedQueue) pop() *types.Transaction {
	if q.length() == 0 {
		return nil
	}

	return heap.Pop(&q.queue).(*types.Transaction)
}

// Returns the number of transactions in the queue.
func (q *promotedQueue) length() uint64 {
	return uint64(q.queue.Len())
}

// Returns valid and pruned transactions after filtering the queue.
// Only PrunePromoted calls this method.
//
// Valid - transactions not considered stale.
//
// Pruned - transactions considered as stale by the nonceMao.
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
	height uint64 // amount of slots currently occupying the pool
	max    uint64 // max limit
}

// Calculates the number of slots required for given transaction(s).
func slotsRequired(txs ...*types.Transaction) uint64 {
	slots := uint64(0)
	for _, tx := range txs {
		slots += func(tx *types.Transaction) uint64 {
			return (tx.Size() + txSlotSize - 1) / txSlotSize
		}(tx)
	}

	return slots
}

// Returns the current height of the gauge.
func (g *slotGauge) read() uint64 {
	return atomic.LoadUint64(&g.height)
}

// Increases the height of the gauge by the specified slots amount.
func (g *slotGauge) increase(slots uint64) {
	atomic.AddUint64(&g.height, slots)
}

// Decreases the height of the gauge by the specified slots amount.
func (g *slotGauge) decrease(slots uint64) {
	atomic.AddUint64(&g.height, ^(slots - 1))
}
