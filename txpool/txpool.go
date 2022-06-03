package txpool

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	txSlotSize  = 32 * 1024  // 32kB
	txMaxSize   = 128 * 1024 //128Kb
	topicNameV1 = "txpool/0.1"

	//	maximum allowed number of times an account
	//	was excluded from block building (ibft.writeTransactions)
	maxAccountDemotions = uint(10)
)

// errors
var (
	ErrIntrinsicGas        = errors.New("intrinsic gas too low")
	ErrBlockLimitExceeded  = errors.New("exceeds block gas limit")
	ErrNegativeValue       = errors.New("negative value")
	ErrExtractSignature    = errors.New("cannot extract signature")
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
	PriceLimit uint64
	MaxSlots   uint64
	Sealing    bool
}

/* All requests are passed to the main loop
through their designated channels. */

// An enqueueRequest is created for any transaction
// meant to be enqueued onto some account.
// This request is created for (new) transactions
// that passed validation in addTx.
type enqueueRequest struct {
	tx *types.Transaction
}

// A promoteRequest is created each time some account
// is eligible for promotion. This request is signaled
// on 2 occasions:
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
	logger hclog.Logger
	signer signer
	forks  chain.ForksInTime
	store  store

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

	// priceLimit is a lower threshold for gas price
	priceLimit uint64

	// channels on which the pool's event loop
	// does dispatching/handling requests.
	enqueueReqCh chan enqueueRequest
	promoteReqCh chan promoteRequest

	// shutdown channel
	shutdownCh chan struct{}

	// flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing bool

	// prometheus API
	metrics *Metrics

	// Event manager for txpool events
	eventManager *eventManager

	// indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer
}

// NewTxPool returns a new pool for processing incoming transactions.
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
		metrics:     metrics,
		accounts:    accountsMap{},
		executables: newPricedQueue(),
		index:       lookupMap{all: make(map[types.Hash]*types.Transaction)},
		gauge:       slotGauge{height: 0, max: config.MaxSlots},
		priceLimit:  config.PriceLimit,
		sealing:     config.Sealing,
	}

	// Attach the event manager
	pool.eventManager = newEventManager(pool.logger)

	if network != nil {
		// subscribe to the gossip protocol
		topic, err := network.NewTopic(topicNameV1, &proto.Txn{})
		if err != nil {
			return nil, err
		}

		if subscribeErr := topic.Subscribe(pool.addGossipTx); subscribeErr != nil {
			return nil, fmt.Errorf("unable to subscribe to gossip topic, %w", subscribeErr)
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
func (p *TxPool) Start() {
	// set default value of txpool pending transactions gauge
	p.metrics.PendingTxs.Set(0)

	go func() {
		for {
			select {
			case <-p.shutdownCh:
				return
			case req := <-p.enqueueReqCh:
				go p.handleEnqueueRequest(req)
			case req := <-p.promoteReqCh:
				go p.handlePromoteRequest(req)
			}
		}
	}()
}

// Close shuts down the pool's main loop.
func (p *TxPool) Close() {
	p.eventManager.Close()
	p.shutdownCh <- struct{}{}
}

// SetSigner sets the signer the pool will use
// to validate a transaction's signature.
func (p *TxPool) SetSigner(s signer) {
	p.signer = s
}

// AddTx adds a new transaction to the pool (sent from json-RPC/gRPC endpoints)
// and broadcasts it to the network (if enabled).
func (p *TxPool) AddTx(tx *types.Transaction) error {
	if err := p.addTx(local, tx); err != nil {
		p.logger.Error("failed to add tx", "err", err)

		return err
	}

	// broadcast the transaction only if a topic
	// subscription is present
	if p.topic != nil {
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

// Prepare generates all the transactions
// ready for execution. (primaries)
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

// Peek returns the best-price selected
// transaction ready for execution.
func (p *TxPool) Peek() *types.Transaction {
	// Popping the executables queue
	// does not remove the actual tx
	// from the pool.
	// The executables queue just provides
	// insight into which account has the
	// highest priced tx (head of promoted queue)
	return p.executables.pop()
}

// Pop removes the given transaction from the
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

	//	successfully popping an account resets its demotions count to 0
	account.demotions = 0

	// update state
	p.gauge.decrease(slotsRequired(tx))

	// update metrics
	p.metrics.PendingTxs.Add(-1)

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}
}

// Drop clears the entire account associated with the given transaction
// and reverts its next (expected) nonce.
func (p *TxPool) Drop(tx *types.Transaction) {
	// fetch associated account
	account := p.accounts.get(tx.From)

	account.promoted.lock(true)
	account.enqueued.lock(true)

	// num of all txs dropped
	droppedCount := 0

	// pool resource cleanup
	clearAccountQueue := func(txs []*types.Transaction) {
		p.index.remove(txs...)
		p.gauge.decrease(slotsRequired(txs...))

		// increase counter
		droppedCount += len(txs)
	}

	defer func() {
		account.enqueued.unlock()
		account.promoted.unlock()
	}()

	// rollback nonce
	nextNonce := tx.Nonce
	account.setNonce(nextNonce)

	// drop promoted
	dropped := account.promoted.clear()
	clearAccountQueue(dropped)

	// update metrics
	p.metrics.PendingTxs.Add(float64(-1 * len(dropped)))

	// drop enqueued
	dropped = account.enqueued.clear()
	clearAccountQueue(dropped)

	p.eventManager.signalEvent(proto.EventType_DROPPED, tx.Hash)
	p.logger.Debug("dropped account txs",
		"num", droppedCount,
		"next_nonce", nextNonce,
		"address", tx.From.String(),
	)
}

//	Demote excludes an account from being further processed during block building
//	due to a recoverable error. If an account has been demoted too many times (maxAccountDemotions),
//	it is Dropped instead.
func (p *TxPool) Demote(tx *types.Transaction) {
	account := p.accounts.get(tx.From)
	if account.demotions == maxAccountDemotions {
		p.logger.Debug(
			"Demote: threshold reached - dropping account",
			"addr", tx.From.String(),
		)

		p.Drop(tx)

		//	reset the demotions counter
		account.demotions = 0

		return
	}

	account.demotions++

	p.eventManager.signalEvent(proto.EventType_DEMOTED, tx.Hash)
}

// ResetWithHeaders processes the transactions from the new
// headers to sync the pool with the new state.
func (p *TxPool) ResetWithHeaders(headers ...*types.Header) {
	e := &blockchain.Event{
		NewChain: headers,
	}

	// process the txs in the event
	// to make sure the pool is up-to-date
	p.processEvent(e)
}

// processEvent collects the latest nonces for each account containted
// in the received event. Resets all known accounts with the new nonce.
func (p *TxPool) processEvent(event *blockchain.Event) {
	oldTxs := make(map[types.Hash]*types.Transaction)

	// Legacy reorg logic //
	for _, header := range event.OldChain {
		// transactions to be returned to the pool
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
	stateNonces := make(map[types.Address]uint64)

	// discover latest (next) nonces for all accounts
	for _, header := range event.NewChain {
		block, ok := p.store.GetBlockByHash(header.Hash, true)
		if !ok {
			p.logger.Error("could not find block in store", "hash", header.Hash.String())

			continue
		}

		// remove mined txs from the lookup map
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

	// reset accounts with the new state
	p.resetAccounts(stateNonces)
}

// validateTx ensures the transaction conforms to specific
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

	// Check if the transaction is signed properly

	// Extract the sender
	from, signerErr := p.signer.Sender(tx)
	if signerErr != nil {
		return ErrExtractSignature
	}

	// If the from field is set, check that
	// it matches the signer
	if tx.From != types.ZeroAddress &&
		tx.From != from {
		return ErrInvalidSender
	}

	// If no address was set, update it
	if tx.From == types.ZeroAddress {
		tx.From = from
	}

	// Reject underpriced transactions
	if tx.IsUnderpriced(p.priceLimit) {
		return ErrUnderpriced
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

	// Grab the block gas limit for the latest block
	latestBlockGasLimit := p.store.Header().GasLimit

	if tx.Gas > latestBlockGasLimit {
		return ErrBlockLimitExceeded
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

	//	add to index
	if ok := p.index.add(tx); !ok {
		return ErrAlreadyKnown
	}

	// initialize account for this address once
	if !p.accounts.exists(tx.From) {
		p.createAccountOnce(tx.From)
	}

	// send request [BLOCKING]
	p.enqueueReqCh <- enqueueRequest{tx: tx}
	p.eventManager.signalEvent(proto.EventType_ADDED, tx.Hash)

	return nil
}

// handleEnqueueRequest attempts to enqueue the transaction
// contained in the given request to the associated account.
// If, afterwards, the account is eligible for promotion,
// a promoteRequest is signaled.
func (p *TxPool) handleEnqueueRequest(req enqueueRequest) {
	tx := req.tx
	addr := req.tx.From

	// fetch account
	account := p.accounts.get(addr)

	// enqueue tx
	if err := account.enqueue(tx); err != nil {
		p.logger.Error("enqueue request", "err", err)

		p.index.remove(tx)

		return
	}

	p.logger.Debug("enqueue request", "hash", tx.Hash.String())

	p.gauge.increase(slotsRequired(tx))

	p.eventManager.signalEvent(proto.EventType_ENQUEUED, tx.Hash)

	if tx.Nonce > account.getNonce() {
		// don't signal promotion for
		// higher nonce txs
		return
	}

	p.promoteReqCh <- promoteRequest{account: addr} // BLOCKING
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
	p.metrics.PendingTxs.Add(float64(len(promoted)))
	p.eventManager.signalEvent(proto.EventType_PROMOTED, toHash(promoted...)...)
}

// addGossipTx handles receiving transactions
// gossiped by the network.
func (p *TxPool) addGossipTx(obj interface{}) {
	if !p.sealing {
		return
	}

	raw, ok := obj.(*proto.Txn)
	if !ok {
		p.logger.Error("failed to cast gossiped message to txn")

		return
	}

	// Verify that the gossiped transaction message is not empty
	if raw == nil || raw.Raw == nil {
		p.logger.Error("malformed gossip transaction message received")

		return
	}

	tx := new(types.Transaction)

	// decode tx
	if err := tx.UnmarshalRLP(raw.Raw.Value); err != nil {
		p.logger.Error("failed to decode broadcast tx", "err", err)

		return
	}

	// add tx
	if err := p.addTx(gossip, tx); err != nil {
		if errors.Is(err, ErrAlreadyKnown) {
			p.logger.Debug("rejecting known tx (gossip)", "hash", tx.Hash.String())

			return
		}

		p.logger.Error("failed to add broadcast tx", "err", err, "hash", tx.Hash.String())
	}
}

// resetAccounts updates existing accounts with the new nonce and prunes stale transactions.
func (p *TxPool) resetAccounts(stateNonces map[types.Address]uint64) {
	var (
		allPrunedPromoted []*types.Transaction
		allPrunedEnqueued []*types.Transaction
	)

	//	clear all accounts of stale txs
	for addr, newNonce := range stateNonces {
		if !p.accounts.exists(addr) {
			// no updates for this account
			continue
		}

		account := p.accounts.get(addr)
		prunedPromoted, prunedEnqueued := account.reset(newNonce, p.promoteReqCh)

		//	append pruned
		allPrunedPromoted = append(allPrunedPromoted, prunedPromoted...)
		allPrunedEnqueued = append(allPrunedEnqueued, prunedEnqueued...)

		//	new state for account -> demotions are reset to 0
		account.demotions = 0
	}

	//	pool cleanup callback
	cleanup := func(stale ...*types.Transaction) {
		p.index.remove(stale...)
		p.gauge.decrease(slotsRequired(stale...))
	}

	//	prune pool state
	if len(allPrunedPromoted) > 0 {
		cleanup(allPrunedPromoted...)
		p.eventManager.signalEvent(
			proto.EventType_PRUNED_PROMOTED,
			toHash(allPrunedPromoted...)...,
		)

		p.metrics.PendingTxs.Add(float64(-1 * len(allPrunedPromoted)))
	}

	if len(allPrunedEnqueued) > 0 {
		cleanup(allPrunedEnqueued...)
		p.eventManager.signalEvent(
			proto.EventType_PRUNED_ENQUEUED,
			toHash(allPrunedEnqueued...)...,
		)
	}
}

// createAccountOnce creates an account and
// ensures it is only initialized once.
func (p *TxPool) createAccountOnce(newAddr types.Address) *account {
	// fetch nonce from state
	stateRoot := p.store.Header().StateRoot
	stateNonce := p.store.GetNonce(stateRoot, newAddr)

	// initialize the account
	account := p.accounts.initOnce(newAddr, stateNonce)

	return account
}

// Length returns the total number of all promoted transactions.
func (p *TxPool) Length() uint64 {
	return p.accounts.promoted()
}

//	toHash returns the hash(es) of given transaction(s)
func toHash(txs ...*types.Transaction) (hashes []types.Hash) {
	for _, tx := range txs {
		hashes = append(hashes, tx.Hash)
	}

	return
}
