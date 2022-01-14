package txpool

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
)

const (
	txSlotSize  = 32 * 1024  // 32kB
	txMaxSize   = 128 * 1024 //128Kb
	topicNameV1 = "txpool/0.1"
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
	PriceLimit uint64
	MaxSlots   uint64
	Sealing    bool
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

	// flag indicating if the current node is running in dev mode (used for testing)
	dev bool

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
	p.eventManager.close()
	p.shutdownCh <- struct{}{}
}

// SetSigner sets the signer the pool will use
// to validate a transaction's signature.
func (p *TxPool) SetSigner(s signer) {
	p.signer = s
}

// EnableDev enables the pool to accept
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

	// update state
	p.gauge.decrease(slotsRequired(tx))

	// update metrics
	p.metrics.PendingTxs.Add(-1)

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}
}

// Drop removes the given (unrecoverable) transaction from
// its associated promoted queue (account) and rolls
// back the account's nextNonce.
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

	// update metrics
	p.metrics.PendingTxs.Add(-1)

	if tx.Nonce < account.getNonce() {
		// rollback nonce
		account.setNonce(tx.Nonce)
	}

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}

	p.eventManager.fireEvent(&proto.TxPoolEvent{
		Type:   proto.EventType_DROPPED,
		TxHash: tx.Hash.String(),
	})
}

// Demote removes the (recoverable) transaction from
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
	p.eventManager.fireEvent(&proto.TxPoolEvent{
		Type:   proto.EventType_DEMOTED,
		TxHash: tx.Hash.String(),
	})
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
	p.eventManager.fireEvent(&proto.TxPoolEvent{
		Type:   proto.EventType_ADDED,
		TxHash: tx.Hash.String(),
	})

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
	if err := account.enqueue(tx, req.demoted); err != nil {
		p.logger.Error("enqueue request", "err", err)

		return
	}

	p.logger.Debug("enqueue request", "hash", tx.Hash.String())
	p.eventManager.fireEvent(&proto.TxPoolEvent{
		Type:   proto.EventType_ENQUEUED,
		TxHash: tx.Hash.String(),
	})

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
	promoted, promotedTxns := account.promote()
	p.logger.Debug("promote request", "promoted", promoted, "addr", addr.String())

	// update metrics
	p.metrics.PendingTxs.Add(float64(promoted))

	for _, promotable := range promotedTxns {
		p.eventManager.fireEvent(&proto.TxPoolEvent{
			Type:   proto.EventType_PROMOTED,
			TxHash: promotable.Hash.String(),
		})
	}
}

// addGossipTx handles receiving transactions
// gossiped by the network.
func (p *TxPool) addGossipTx(obj interface{}) {
	if !p.sealing {
		return
	}

	raw := obj.(*proto.Txn) // nolint:forcetypeassert
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

// resetAccounts updates existing accounts with the new nonce.
func (p *TxPool) resetAccounts(stateNonces map[types.Address]uint64) {
	for addr, nonce := range stateNonces {
		if !p.accounts.exists(addr) {
			// unknown account
			continue
		}

		p.resetAccount(addr, nonce)
	}
}

// resetAccount aligns the account's state with the given nonce,
// pruning any present stale transaction. If, afterwards, the account
// is eligible for promotion, a promoteRequest is signaled.
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

	// update metrics
	p.metrics.PendingTxs.Add(float64(-1 * len(pruned)))

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
