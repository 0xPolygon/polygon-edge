package txpool

import (
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

// errors
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
	ErrOversizedData       = errors.New("oversized data")
)

// indicates origin of a transaction
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

/* All requests are passed to the main loop
through their designated channels. */

// An enqueueRequest is created for any transaction
// meant to be enqueued onto some account.
// This request is made on 2 occasions:
//
// 	1. When a transaction is initially discovered with addTx
// 	and passes validation.
//
//	2. When consensus is processing a previously
// 	promoted transaction and decides to return it
// 	to the pool (Demote). These requests have the
// 	demoted flag set to true.
type enqueueRequest struct {
	tx      *types.Transaction
	demoted bool
}

// A promoteRequest is created each time some account
// is eligible for promotion. This request is signaled
// on 2 ocassions:
//
// 	1. When an enqueued transaction's nonce is
// 	not greater than the expected (account's nextNonce).
// 		== 	nextNonce	- transaction is expected (addTx)
// 		<	nextNonce	- transaction was demoted (Demote)
//
// 	2. When an account's nextNonce is updated (during ResetWithHeader)
// 	and the first enqueued transaction matches the new nonce.
type promoteRequest struct {
	account types.Address
}

// TxPool is a module that handles pending transactions.
// All transactions are handled within their respective accounts.
// An account contains 2 queues a transaction needs to go through:
// - 1. Enqueued	(entry point)
// - 2. Promoted	(exit point)
// 	(both queues are min nonce ordered)
//
// When consensus needs to process promoted transactions,
// the pool generates a queue of "executable" transactions. These
// transactions are the first-in-line of some promoted queue,
// ready to be written to the state (primaries).
type TxPool struct {
	logger     hclog.Logger
	signer     signer
	forks      chain.ForksInTime
	store      store
	idlePeriod time.Duration

	// map of all accounts registered by the pool
	accounts accountsMap

	// all the primaries sorted by max gas price
	executables *pricedQueue

	// lookup map keeping track of all
	// transactions present in the pool
	index lookupMap

	// networking stack
	topic *network.Topic

	// gauge for measuring pool capacity
	gauge slotGauge

	// channels on which the pool's event loop
	// does dispatching/handling requests.
	enqueueReqCh chan enqueueRequest
	promoteReqCh chan promoteRequest

	// shutdown channel
	shutdownCh chan struct{}

	// flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing bool

	// flag indicating if the current node is running in dev mode (used for testing)
	dev bool

	// prometheus API
	metrics *Metrics

	// indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer
}

// Returns a new pool for processing incoming transactions.
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
		logger:      logger.Named("txpool"),
		forks:       forks,
		store:       store,
		idlePeriod:  defaultIdlePeriod,
		metrics:     metrics,
		accounts:    accountsMap{},
		executables: newPricedQueue(),
		index:       lookupMap{all: make(map[types.Hash]*types.Transaction)},
		gauge:       slotGauge{height: 0, max: config.MaxSlots},
		sealing:     config.Sealing,
	}

	if network != nil {
		// subscribe to the gossip protocol
		topic, err := network.NewTopic(topicNameV1, &proto.Txn{})
		if err != nil {
			return nil, err
		}
		if subscribeErr := topic.Subscribe(pool.addGossipTx); subscribeErr != nil {
			return nil, fmt.Errorf("unable to subscribe to gossip topic, %v", subscribeErr)
		}

		pool.topic = topic
	}

	if grpcServer != nil {
		proto.RegisterTxnPoolOperatorServer(grpcServer, pool)
	}

	// initialise channels
	pool.enqueueReqCh = make(chan enqueueRequest)
	pool.promoteReqCh = make(chan promoteRequest)
	pool.shutdownCh = make(chan struct{})

	return pool, nil
}

// Start runs the pool's main loop in the background.
// On each request received, the appropriate handler
// is invoked in a separate goroutine.
func (p *TxPool) Start() error {
	go func() {

		// check if shutdown was called
		select {
		case <-p.shutdownCh:
			return
		default:
		}

		// handle requests
		for {
			select {
			case req := <-p.enqueueReqCh:
				go p.handleEnqueueRequest(req)
			case req := <-p.promoteReqCh:
				go p.handlePromoteRequest(req)
			}
		}
	}()

	return nil
}

// Stops the pool's main loop.
func (p *TxPool) Stop() {
	p.shutdownCh <- struct{}{}
}

// Sets the signer the pool will use
// to check a transaction's signature.
func (p *TxPool) SetSigner(s signer) {
	p.signer = s
}

// Enables dev mode so the pool can accept
// non-encrypted transactions. (used for testing)
func (p *TxPool) EnableDev() {
	p.dev = true
}

// AddTx adds a new transaction to the pool (sent from json-RPC/gRPC endpoints)
// and broadcasts it to the network (if enabled).
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

// Generates all the transactions (primaries)
// ready for execution.
func (p *TxPool) Prepare() {
	// clear from previous round
	if p.executables.length() != 0 {
		p.executables.clear()
	}

	// fetch primary from each account
	primaries := p.accounts.getPrimaries()

	// push primaries to the executables queue
	for _, tx := range primaries {
		p.executables.push(tx)
	}
}

// Returns the highest priced transaction
// from the executables queue.
func (p *TxPool) Peek() *types.Transaction {
	return p.executables.pop()
}

// Pops the given transaction from the
// associated promoted queue (account).
// Will update executables with the next primary
// from that account (if any).
func (p *TxPool) Pop(tx *types.Transaction) {
	// fetch the associated account
	account := p.accounts.get(tx.From)

	account.promoted.lock(true)
	defer account.promoted.unlock()

	// pop the top most promoted tx
	account.promoted.pop()

	// update state
	p.gauge.decrease(slotsRequired(tx))

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}
}

// Drop pops the unrecoverable transaction from
// its associated promoted queue (account) and rolls
// back that account's nextNonce.
// Will update executables with the next primary
// from that account (if any).
func (p *TxPool) Drop(tx *types.Transaction) {
	account := p.accounts.get(tx.From)

	account.promoted.lock(true)
	defer account.promoted.unlock()

	// pop the top most promoted tx
	account.promoted.pop()

	// update state
	p.index.remove(tx)
	p.gauge.decrease(slotsRequired(tx))

	if tx.Nonce < account.getNonce() {
		// rollback nonce
		account.setNonce(tx.Nonce)
	}

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}
}

// Demote pops the recoverable transaction from
// its associated promoted queue (account) and
// issues an enqueueRequest for it.
// Will update executables with the next primary
// from that account (if any).
func (p *TxPool) Demote(tx *types.Transaction) {
	account := p.accounts.get(tx.From)

	account.promoted.lock(true)
	defer account.promoted.unlock()

	// drop the tx from account promoted
	account.promoted.pop()

	// signal enqueue request [BLOCKING]
	p.enqueueReqCh <- enqueueRequest{tx: tx, demoted: true}

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}

	p.logger.Debug("demoted transaction", "hash", tx.Hash.String())
}

// ResetWithHeaders is called from within ibft when the node
// has received a new block from a peer. The pool needs to align
// its own state with the new one so it can correctly process
// further incoming transactions.
func (p *TxPool) ResetWithHeaders(headers ...*types.Header) {
	e := &blockchain.Event{
		NewChain: headers,
	}

	// process the txs in the event
	// to make sure the pool is up-to-date
	p.processEvent(e)
}

// Collects the latest nonces for each account containted
// in the received event. Resets all accounts with the new nonce.
func (p *TxPool) processEvent(event *blockchain.Event) {
	// txs in the OldCHain
	oldTxs := make(map[types.Hash]*types.Transaction)

	// Legacy reorg logic //
	for _, header := range event.OldChain {
		// transactios to be returned to the pool
		block, ok := p.store.GetBlockByHash(header.Hash, true)
		if !ok {
			continue
		}

		for _, tx := range block.Transactions {
			oldTxs[tx.Hash] = tx
		}
	}

	// Grab the latest state root now that the block has been inserted
	stateRoot := p.store.Header().StateRoot

	// discover latest (next) nonces for all accounts
	stateNonces := make(map[types.Address]uint64)
	for _, header := range event.NewChain {
		block, ok := p.store.GetBlockByHash(header.Hash, true)
		if !ok {
			p.logger.Error("could not find block in store", "hash", header.Hash.String())
			continue
		}

		p.index.remove(block.Transactions...)

		// etract latest nonces
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
			delete(oldTxs, tx.Hash)
		}
	}

	// Legacy reorg logic //
	for _, tx := range oldTxs {
		if err := p.addTx(reorg, tx); err != nil {
			p.logger.Error("add tx", "err", err)
		}
	}

	if len(stateNonces) == 0 {
		return
	}

	// reset with the new state
	p.resetAccounts(stateNonces)
}

// Ensures the transaction conforms to specific
// constraints before entering the pool.
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
// for all new transactions. If the call is
// successful, an account is created for this address
// (only once) and an enqueueRequest is signaled.
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
	if _, ok := p.index.get(tx.Hash); ok {
		if origin == gossip {
			// silently drop known tx
			// that is gossiped back
			p.logger.Debug(
				"dropping known gossiped transaction",
				"hash", tx.Hash.String(),
			)

			return nil
		} else {
			return ErrAlreadyKnown
		}
	}

	// initialize account for this address once
	if !p.accounts.exists(tx.From) {
		p.createAccountOnce(tx.From)
	}

	// send request [BLOCKING]
	p.enqueueReqCh <- enqueueRequest{tx: tx}

	return nil
}

// handleEnqueueRequest is invoked when a new transaction is received
// (result of a successful addTx() call) or a demoted transaction
// is re-entering the pool.
//
// A transaction handled within this request can either be
// dropped or enqueued, potentially signaling promotion in the latter case.
func (p *TxPool) handleEnqueueRequest(req enqueueRequest) {
	tx := req.tx
	addr := req.tx.From

	// fetch account
	account := p.accounts.get(addr)

	// enqueue tx
	if err := account.enqueue(tx, req.demoted); err != nil {
		p.logger.Error("enqueue request", "err", err)
		return
	}

	p.logger.Debug("enqueue request", "hash", tx.Hash.String())

	// update lookup
	p.index.add(tx)

	// demoted transactions never decrease the gauge
	if !req.demoted {
		p.gauge.increase(slotsRequired(tx))
	}

	if tx.Nonce <= account.getNonce() {
		// account queue is ready for promotion:
		// 	1. New tx is matching nonce expected
		// 	2. Demoted tx is eligible for promotion
		p.promoteReqCh <- promoteRequest{account: addr} // BLOCKING
	}
}

// handlePromoteRequest handles moving promotable transactions
// of some account from enqueued to promoted. Can only be
// invoked by handleEnqueueRequest or resetAccount.
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
	addr := req.account
	account := p.accounts.get(addr)

	// promote enqueued txs
	promoted := account.promote()
	p.logger.Debug("promote request", "promoted", promoted, "addr", addr.String())

	// update metrics
	p.metrics.PendingTxs.Add(float64(promoted))
}

// addGossipTx handles receiving transactions
// gossiped by the network.
func (p *TxPool) addGossipTx(obj interface{}) {
	if !p.sealing {
		return
	}

	raw := obj.(*proto.Txn)
	tx := new(types.Transaction)

	// decode tx
	if err := tx.UnmarshalRLP(raw.Raw.Value); err != nil {
		p.logger.Error("failed to decode broadcasted tx", "err", err)
		return
	}

	// add tx
	if err := p.addTx(gossip, tx); err != nil {
		p.logger.Error("failed to add broadcasted txn", "err", err)
	}
}

// Updates existing accounts with the new nonce.
func (p *TxPool) resetAccounts(stateNonces map[types.Address]uint64) {
	for addr, nonce := range stateNonces {
		if !p.accounts.exists(addr) {
			// unknown account
			continue
		}

		p.resetAccount(addr, nonce)
	}
}

// Removes all transactions from the account considered stale by the given nonce.
// Signals a promotion request if first enqueued transaction is eligible (expected nonce).
func (p *TxPool) resetAccount(addr types.Address, nonce uint64) {
	account := p.accounts.get(addr)

	// lock promoted
	account.promoted.lock(true)
	defer account.promoted.unlock()

	// prune promoted
	pruned := account.promoted.prune(nonce)

	// update pool state
	p.index.remove(pruned...)
	p.gauge.decrease(slotsRequired(pruned...))

	if nonce <= account.getNonce() {
		// only the promoted queue needed pruning
		return
	}

	// lock enqueued
	account.enqueued.lock(true)
	defer account.enqueued.unlock()

	// prune enqueued
	pruned = account.enqueued.prune(nonce)

	// update pool state
	p.index.remove(pruned...)
	p.gauge.decrease(slotsRequired(pruned...))

	// update next nonce
	account.setNonce(nonce)

	if first := account.enqueued.peek(); first != nil &&
		first.Nonce == nonce {
		// first enqueued tx is expected -> signal promotion
		p.promoteReqCh <- promoteRequest{account: addr}
	}
}

// createAccountOnce is used when discovering an address
// of a received transaction for the first time.
// This function ensures that the account
// is created atomically and only once.
func (p *TxPool) createAccountOnce(newAddr types.Address) *account {
	// fetch nonce from state
	stateRoot := p.store.Header().StateRoot
	stateNonce := p.store.GetNonce(stateRoot, newAddr)

	// initialize the account
	account := p.accounts.initOnce(newAddr, stateNonce)

	return account
}

/* QUERY methods */

// GetNonce returns the next nonce for the account
// -> Returns the value from the TxPool if the account is initialized in-memory
// -> Returns the value from the world state otherwise
func (p *TxPool) GetNonce(addr types.Address) uint64 {
	account := p.accounts.get(addr)
	if account == nil {
		stateRoot := p.store.Header().StateRoot
		stateNonce := p.store.GetNonce(stateRoot, addr)

		return stateNonce
	}

	return account.getNonce()
}

// GetCapacity returns the current number of slots
// occupied in the pool as well as the max limit
func (p *TxPool) GetCapacity() (uint64, uint64) {
	return p.gauge.read(), p.gauge.max
}

// GetPendingTx returns the transaction by hash in the TxPool (pending txn) [Thread-safe]
func (p *TxPool) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	tx, ok := p.index.get(txHash)
	if !ok {
		return nil, false
	}

	return tx, true
}

// GetTxs gets pending and queued transactions
func (p *TxPool) GetTxs(inclQueued bool) (
	allPromoted, allEnqueued map[types.Address][]*types.Transaction,
) {
	allPromoted, allEnqueued = p.accounts.allTxs(inclQueued)
	return
}

// Thread safe map of all accounts registered by the pool.
type accountsMap struct {
	sync.Map
	count uint64
}

// Intializes an account for the given address.
func (m *accountsMap) initOnce(addr types.Address, nonce uint64) *account {
	a, _ := m.LoadOrStore(addr, &account{})
	newAccount := a.(*account)

	// run only once
	newAccount.init.Do(func() {
		// create queues
		newAccount.enqueued = newAccountQueue()
		newAccount.promoted = newAccountQueue()

		// set the nonce
		newAccount.setNonce(nonce)

		// update global count
		atomic.AddUint64(&m.count, 1)
	})

	return newAccount
}

// Checks if an account exists within the map.
func (m *accountsMap) exists(addr types.Address) bool {
	_, ok := m.Load(addr)
	return ok
}

// Collects the primaries of all accounts.
func (m *accountsMap) getPrimaries() (primaries transactions) {
	m.Range(func(key, value interface{}) bool {
		account := m.get(key.(types.Address))

		account.promoted.lock(false)
		defer account.promoted.unlock()

		// add primary
		if tx := account.promoted.peek(); tx != nil {
			primaries = append(primaries, tx)
		}

		return true
	})

	return primaries
}

// Returns the account associated with the given address.
func (m *accountsMap) get(addr types.Address) *account {
	a, ok := m.Load(addr)
	if !ok {
		return nil
	}

	return a.(*account)
}

// Returns the number of all promoted transactons.
func (m *accountsMap) promoted() (total uint64) {
	m.Range(func(key, value interface{}) bool {
		account := m.get(key.(types.Address))

		account.promoted.lock(false)
		defer account.promoted.unlock()

		total += account.promoted.length()
		return true
	})

	return
}

// Returns all promoted and enqueued transactions (if the flag is set to true).
func (m *accountsMap) allTxs(includeEnqueued bool) (
	allPromoted, allEnqueued map[types.Address][]*types.Transaction,
) {
	allPromoted = make(map[types.Address][]*types.Transaction)
	allEnqueued = make(map[types.Address][]*types.Transaction)
	m.Range(func(key, value interface{}) bool {
		addr := key.(types.Address)
		account := m.get(addr)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		allPromoted[addr] = account.promoted.queue

		if includeEnqueued {
			account.enqueued.lock(false)
			defer account.enqueued.unlock()

			allEnqueued[addr] = account.enqueued.queue
		}

		return true
	})

	return
}

/* account impl */

// An account is the core structure for processing
// transactions from a specific address. The nextNonce
// field is what separetes the enqueued from promoted:
//
// 	1. enqueued - transactions higher than the nextNonce
// 	2. promoted - transactions lower than the nextNonce
//
// If an enqueued transaction matches the nextNonce,
// a promoteRequest is signaled for this account
// indicating the account's enqueued transaction(s)
// are ready to be moved to the promoted queue.
type account struct {
	init               sync.Once
	enqueued, promoted *accountQueue
	nextNonce          uint64
}

// Returns the next expected nonce for this account.
func (a *account) getNonce() uint64 {
	return atomic.LoadUint64(&a.nextNonce)
}

// Sets the next expected nonce for this account.
func (a *account) setNonce(nonce uint64) {
	atomic.StoreUint64(&a.nextNonce, nonce)
}

// Pushes the transaction onto the enqueued queue.
func (a *account) enqueue(tx *types.Transaction, demoted bool) error {
	a.enqueued.lock(true)
	defer a.enqueued.unlock()

	// only accept low nonce if
	// tx was demoted
	if tx.Nonce < a.getNonce() &&
		!demoted {
		return ErrNonceTooLow
	}

	// enqueue tx
	a.enqueued.push(tx)

	return nil
}

// Promote moves eligible transactions from enqueued to promoted.
//
// Eligible transactions are all sequential in order of nonce
// and the first one has to have nonce less (or equal) to the account's
// nextNonce.
func (a *account) promote() uint64 {
	a.promoted.lock(true)
	a.enqueued.lock(true)
	defer func() {
		a.promoted.unlock()
		a.enqueued.unlock()
	}()

	currentNonce := a.getNonce()
	if a.enqueued.length() == 0 ||
		a.enqueued.peek().Nonce > currentNonce {
		// nothing to promote
		return 0
	}

	promoted := uint64(0)
	nextNonce := a.enqueued.peek().Nonce
	for {
		tx := a.enqueued.peek()
		if tx == nil ||
			tx.Nonce != nextNonce {
			break
		}

		// pop from enqueued
		tx = a.enqueued.pop()

		// push to promoted
		a.promoted.push(tx)

		// update counters
		nextNonce += 1
		promoted += 1
	}

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	if nextNonce > currentNonce {
		a.setNonce(nextNonce)
	}

	return promoted
}

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
func (m *lookupMap) get(hash types.Hash) (*types.Transaction, bool) {
	m.RLock()
	defer m.RUnlock()

	tx, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return tx, true
}

// Gauge for measuring pool capacity in slots
type slotGauge struct {
	height uint64 // amount of slots currently occupying the pool
	max    uint64 // max limit
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
