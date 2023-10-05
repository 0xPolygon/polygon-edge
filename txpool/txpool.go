package txpool

import (
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/armon/go-metrics"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/grpc"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	txSlotSize  = 32 * 1024  // 32kB
	txMaxSize   = 128 * 1024 // 128Kb
	topicNameV1 = "txpool/0.1"

	// maximum allowed number of times an account
	// was excluded from block building (ibft.writeTransactions)
	maxAccountDemotions uint64 = 10

	// maximum allowed number of consecutive blocks that don't have the account's transaction
	maxAccountSkips = uint64(10)

	pruningCooldown = 5000 * time.Millisecond

	// txPoolMetrics is a prefix used for txpool-related metrics
	txPoolMetrics = "txpool"
)

// errors
var (
	ErrIntrinsicGas            = errors.New("intrinsic gas too low")
	ErrBlockLimitExceeded      = errors.New("exceeds block gas limit")
	ErrNegativeValue           = errors.New("negative value")
	ErrExtractSignature        = errors.New("cannot extract signature")
	ErrInvalidSender           = errors.New("invalid sender")
	ErrTxPoolOverflow          = errors.New("txpool is full")
	ErrUnderpriced             = errors.New("transaction underpriced")
	ErrNonceTooLow             = errors.New("nonce too low")
	ErrInsufficientFunds       = errors.New("insufficient funds for gas * price + value")
	ErrInvalidAccountState     = errors.New("invalid account state")
	ErrAlreadyKnown            = errors.New("already known")
	ErrOversizedData           = errors.New("oversized data")
	ErrMaxEnqueuedLimitReached = errors.New("maximum number of enqueued transactions reached")
	ErrRejectFutureTx          = errors.New("rejected future tx due to low slots")
	ErrInvalidTxType           = errors.New("invalid tx type")
	ErrTxTypeNotSupported      = types.ErrTxTypeNotSupported
	ErrTipAboveFeeCap          = errors.New("max priority fee per gas higher than max fee per gas")
	ErrTipVeryHigh             = errors.New("max priority fee per gas higher than 2^256-1")
	ErrFeeCapVeryHigh          = errors.New("max fee per gas higher than 2^256-1")
	ErrNonceExistsInPool       = errors.New("tx with the same nonce is already present")
	ErrReplacementUnderpriced  = errors.New("replacement tx underpriced")
	ErrDynamicTxNotAllowed     = errors.New("dynamic tx not allowed currently")
)

// indicates origin of a transaction
type txOrigin int

const (
	local  txOrigin = iota // json-RPC/gRPC endpoints
	gossip                 // gossip protocol
)

func (o txOrigin) String() (s string) {
	switch o {
	case local:
		s = "local"
	case gossip:
		s = "gossip"
	}

	return
}

// store interface defines State helper methods the TxPool should have access to
type store interface {
	Header() *types.Header
	GetNonce(root types.Hash, addr types.Address) uint64
	GetBalance(root types.Hash, addr types.Address) (*big.Int, error)
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
	CalculateBaseFee(parent *types.Header) uint64
}

type signer interface {
	Sender(tx *types.Transaction) (types.Address, error)
}

type Config struct {
	PriceLimit         uint64
	MaxSlots           uint64
	MaxAccountEnqueued uint64
	ChainID            *big.Int
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
// 1. When an enqueued transaction's nonce is
// not greater than the expected (account's nextNonce).
// == nextNonce - transaction is expected (addTx)
// < nextNonce - transaction was demoted (Demote)
//
// 2. When an account's nextNonce is updated (during ResetWithHeader)
// and the first enqueued transaction matches the new nonce.
type promoteRequest struct {
	account types.Address
}

// TxPool is a module that handles pending transactions.
// All transactions are handled within their respective accounts.
// An account contains 2 queues a transaction needs to go through:
// - 1. Enqueued (entry point)
// - 2. Promoted (exit point)
// (both queues are min nonce ordered)
//
// When consensus needs to process promoted transactions,
// the pool generates a queue of "executable" transactions. These
// transactions are the first-in-line of some promoted queue,
// ready to be written to the state (primaries).
type TxPool struct {
	logger hclog.Logger
	signer signer
	forks  *chain.Forks
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
	promoteReqCh chan promoteRequest
	pruneCh      chan struct{}

	// shutdown channel
	shutdownCh chan struct{}

	// flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing atomic.Bool

	// baseFee is the base fee of the current head.
	// This is needed to sort transactions by price
	baseFee uint64

	// Event manager for txpool events
	eventManager *eventManager

	// indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer

	// pending is the list of pending and ready transactions. This variable
	// is accessed with atomics
	pending int64

	// chain id
	chainID *big.Int
}

// NewTxPool returns a new pool for processing incoming transactions.
func NewTxPool(
	logger hclog.Logger,
	forks *chain.Forks,
	store store,
	grpcServer *grpc.Server,
	network *network.Server,
	config *Config,
) (*TxPool, error) {
	pool := &TxPool{
		logger:      logger.Named("txpool"),
		forks:       forks,
		store:       store,
		executables: newPricesQueue(0, nil),
		accounts:    accountsMap{maxEnqueuedLimit: config.MaxAccountEnqueued},
		index:       lookupMap{all: make(map[types.Hash]*types.Transaction)},
		gauge:       slotGauge{height: 0, max: config.MaxSlots},
		priceLimit:  config.PriceLimit,
		chainID:     config.ChainID,

		//	main loop channels
		promoteReqCh: make(chan promoteRequest),
		pruneCh:      make(chan struct{}),
		shutdownCh:   make(chan struct{}),
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

	return pool, nil
}

func (p *TxPool) updatePending(i int64) {
	newPending := atomic.AddInt64(&p.pending, i)
	metrics.SetGauge([]string{txPoolMetrics, "pending_transactions"}, float32(newPending))
}

// Start runs the pool's main loop in the background.
// On each request received, the appropriate handler
// is invoked in a separate goroutine.
func (p *TxPool) Start() {
	// set default value of txpool pending transactions gauge
	p.updatePending(0)

	//	run the handler for high gauge level pruning
	go func() {
		for {
			select {
			case <-p.shutdownCh:
				return
			case <-p.pruneCh:
				p.pruneAccountsWithNonceHoles()
			}

			//	handler is in cooldown to avoid successive calls
			//	which could be just no-ops
			time.Sleep(pruningCooldown)
		}
	}()

	//	run the handler for the tx pipeline
	go func() {
		for {
			select {
			case <-p.shutdownCh:
				return
			case req := <-p.promoteReqCh:
				go p.handlePromoteRequest(req)
			}
		}
	}()
}

// Close shuts down the pool's main loop.
func (p *TxPool) Close() {
	p.eventManager.Close()
	close(p.shutdownCh)
}

// SetSigner sets the signer the pool will use
// to validate a transaction's signature.
func (p *TxPool) SetSigner(s signer) {
	p.signer = s
}

// SetSealing sets the sealing flag
func (p *TxPool) SetSealing(sealing bool) {
	p.sealing.CompareAndSwap(p.sealing.Load(), sealing)
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
	// fetch primary from each account
	primaries := p.accounts.getPrimaries()

	// create new executables queue with base fee and initial transactions (primaries)
	p.executables = newPricesQueue(p.GetBaseFee(), primaries)
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
	account.nonceToTx.lock()

	defer func() {
		account.nonceToTx.unlock()
		account.promoted.unlock()
	}()

	// pop the top most promoted tx
	account.promoted.pop()

	// update the account nonce -> *tx map
	account.nonceToTx.remove(tx)

	// successfully popping an account resets its demotions count to 0
	account.resetDemotions()

	// update state
	p.gauge.decrease(slotsRequired(tx))

	// update metrics
	p.updatePending(-1)

	// update executables
	if tx := account.promoted.peek(); tx != nil {
		p.executables.push(tx)
	}
}

// Drop clears the entire account associated with the given transaction
// and reverts its next (expected) nonce.
func (p *TxPool) Drop(tx *types.Transaction) {
	account := p.accounts.get(tx.From)
	p.dropAccount(account, tx.Nonce, tx)
}

// dropAccount clears all promoted and enqueued tx from the account
// signals EventType_DROPPED for provided hash, clears all the slots and metrics
// and sets nonce to provided nonce
func (p *TxPool) dropAccount(account *account, nextNonce uint64, tx *types.Transaction) {
	account.promoted.lock(true)
	account.enqueued.lock(true)
	account.nonceToTx.lock()

	defer func() {
		account.nonceToTx.unlock()
		account.enqueued.unlock()
		account.promoted.unlock()
	}()

	// num of all txs dropped
	droppedCount := 0

	// pool resource cleanup
	clearAccountQueue := func(txs []*types.Transaction) {
		p.index.remove(txs...)
		p.gauge.decrease(slotsRequired(txs...))

		// increase counter
		droppedCount += len(txs)
	}

	// rollback nonce
	account.setNonce(nextNonce)

	// reset accounts nonce map
	account.nonceToTx.reset()

	// drop promoted
	dropped := account.promoted.clear()
	clearAccountQueue(dropped)

	// update metrics
	p.updatePending(-1 * int64(len(dropped)))

	// drop enqueued
	dropped = account.enqueued.clear()
	clearAccountQueue(dropped)

	p.eventManager.signalEvent(proto.EventType_DROPPED, tx.Hash)

	if p.logger.IsDebug() {
		p.logger.Debug("dropped account txs",
			"num", droppedCount,
			"next_nonce", nextNonce,
			"address", tx.From.String(),
		)
	}
}

// Demote excludes an account from being further processed during block building
// due to a recoverable error. If an account has been demoted too many times (maxAccountDemotions),
// it is Dropped instead.
func (p *TxPool) Demote(tx *types.Transaction) {
	account := p.accounts.get(tx.From)
	if account.Demotions() >= maxAccountDemotions {
		if p.logger.IsDebug() {
			p.logger.Debug(
				"Demote: threshold reached - dropping account",
				"addr", tx.From.String(),
			)
		}

		p.Drop(tx)

		// reset the demotions counter
		account.resetDemotions()

		return
	}

	account.incrementDemotions()

	p.eventManager.signalEvent(proto.EventType_DEMOTED, tx.Hash)
}

// ResetWithHeaders processes the transactions from the new
// headers to sync the pool with the new state.
func (p *TxPool) ResetWithHeaders(headers ...*types.Header) {
	// process the txs in the event
	// to make sure the pool is up-to-date
	p.processEvent(&blockchain.Event{
		NewChain: headers,
	})
}

// processEvent collects the latest nonces for each account contained
// in the received event. Resets all known accounts with the new nonce.
func (p *TxPool) processEvent(event *blockchain.Event) {
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

		// Extract latest nonces
		for _, tx := range block.Transactions {
			var err error

			addr := tx.From
			if addr == types.ZeroAddress {
				// From field is not set, extract the signer
				if addr, err = p.signer.Sender(tx); err != nil {
					p.logger.Error(
						fmt.Sprintf("unable to extract signer for transaction, %v", err),
					)

					continue
				}
			}

			// skip already processed accounts
			if _, processed := stateNonces[addr]; processed {
				continue
			}

			// fetch latest nonce from the state
			latestNonce := p.store.GetNonce(stateRoot, addr)

			// update the result map
			stateNonces[addr] = latestNonce
		}
	}

	// update base fee
	if ln := len(event.NewChain); ln > 0 {
		p.SetBaseFee(event.NewChain[ln-1])
	}

	// reset accounts with the new state
	p.resetAccounts(stateNonces)

	if !p.sealing.Load() {
		// only non-validator cleanup inactive accounts
		p.updateAccountSkipsCounts(stateNonces)
	}
}

// validateTx ensures the transaction conforms to specific
// constraints before entering the pool.
func (p *TxPool) validateTx(tx *types.Transaction) error {
	// Check the transaction type. State transactions are not expected to be added to the pool
	if tx.Type == types.StateTx {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_tx_type"}, 1)

		return fmt.Errorf("%w: type %d rejected, state transactions are not expected to be added to the pool",
			ErrInvalidTxType, tx.Type)
	}

	// Check the transaction size to overcome DOS Attacks
	if uint64(len(tx.MarshalRLP())) > txMaxSize {
		metrics.IncrCounter([]string{txPoolMetrics, "oversized_data_txs"}, 1)

		return ErrOversizedData
	}

	// Check if the transaction has a strictly positive value
	if tx.Value.Sign() < 0 {
		metrics.IncrCounter([]string{txPoolMetrics, "negative_value_tx"}, 1)

		return ErrNegativeValue
	}

	// Check if the transaction is signed properly

	// Extract the sender
	from, signerErr := p.signer.Sender(tx)
	if signerErr != nil {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_signature_txs"}, 1)

		return ErrExtractSignature
	}

	// If the from field is set, check that
	// it matches the signer
	if tx.From != types.ZeroAddress &&
		tx.From != from {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_sender_txs"}, 1)

		return ErrInvalidSender
	}

	// If no address was set, update it
	if tx.From == types.ZeroAddress {
		tx.From = from
	}

	// Grab current block number
	currentHeader := p.store.Header()
	currentBlockNumber := currentHeader.Number

	// Get forks state for the current block
	forks := p.forks.At(currentBlockNumber)

	// Check if transaction can deploy smart contract
	if tx.IsContractCreation() && forks.EIP158 && len(tx.Input) > state.TxPoolMaxInitCodeSize {
		metrics.IncrCounter([]string{txPoolMetrics, "contract_deploy_too_large_txs"}, 1)

		return runtime.ErrMaxCodeSizeExceeded
	}

	// Grab the state root, and block gas limit for the latest block
	stateRoot := currentHeader.StateRoot
	latestBlockGasLimit := currentHeader.GasLimit
	baseFee := p.GetBaseFee() // base fee is calculated for the next block

	if tx.Type == types.DynamicFeeTx {
		// Reject dynamic fee tx if london hardfork is not enabled
		if !forks.London {
			metrics.IncrCounter([]string{txPoolMetrics, "tx_type"}, 1)

			return fmt.Errorf("%w: type %d rejected, london hardfork is not enabled", ErrTxTypeNotSupported, tx.Type)
		}

		// DynamicFeeTx should be rejected if TxHashWithType fork is registered but not enabled for current block
		blockNumber, err := forkmanager.GetInstance().GetForkBlock(chain.TxHashWithType)
		if err == nil && blockNumber > currentBlockNumber {
			metrics.IncrCounter([]string{txPoolMetrics, "dynamic_tx_not_allowed"}, 1)

			return ErrDynamicTxNotAllowed
		}

		// Check EIP-1559-related fields and make sure they are correct
		if tx.GasFeeCap == nil || tx.GasTipCap == nil {
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrUnderpriced
		}

		if tx.GasFeeCap.BitLen() > 256 {
			metrics.IncrCounter([]string{txPoolMetrics, "fee_cap_too_high_dynamic_tx"}, 1)

			return ErrFeeCapVeryHigh
		}

		if tx.GasTipCap.BitLen() > 256 {
			metrics.IncrCounter([]string{txPoolMetrics, "tip_too_high_dynamic_tx"}, 1)

			return ErrTipVeryHigh
		}

		if tx.GasFeeCap.Cmp(tx.GasTipCap) < 0 {
			metrics.IncrCounter([]string{txPoolMetrics, "tip_above_fee_cap_dynamic_tx"}, 1)

			return ErrTipAboveFeeCap
		}

		// Reject underpriced transactions
		if tx.GasFeeCap.Cmp(new(big.Int).SetUint64(baseFee)) < 0 {
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrUnderpriced
		}
	} else {
		// Legacy approach to check if the given tx is not underpriced when london hardfork is enabled
		if forks.London && tx.GasPrice.Cmp(new(big.Int).SetUint64(baseFee)) < 0 {
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrUnderpriced
		}
	}

	// Check if the given tx is not underpriced
	if tx.GetGasPrice(baseFee).Cmp(new(big.Int).SetUint64(p.priceLimit)) < 0 {
		metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

		return ErrUnderpriced
	}

	// Check nonce ordering
	if p.store.GetNonce(stateRoot, tx.From) > tx.Nonce {
		metrics.IncrCounter([]string{txPoolMetrics, "nonce_too_low_tx"}, 1)

		return ErrNonceTooLow
	}

	accountBalance, balanceErr := p.store.GetBalance(stateRoot, tx.From)
	if balanceErr != nil {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_account_state_tx"}, 1)

		return ErrInvalidAccountState
	}

	// Check if the sender has enough funds to execute the transaction
	if accountBalance.Cmp(tx.Cost()) < 0 {
		metrics.IncrCounter([]string{txPoolMetrics, "insufficient_funds_tx"}, 1)

		return ErrInsufficientFunds
	}

	// Make sure the transaction has more gas than the basic transaction fee
	intrinsicGas, err := state.TransactionGasCost(tx, forks.Homestead, forks.Istanbul)
	if err != nil {
		metrics.IncrCounter([]string{txPoolMetrics, "invalid_intrinsic_gas_tx"}, 1)

		return err
	}

	if tx.Gas < intrinsicGas {
		metrics.IncrCounter([]string{txPoolMetrics, "intrinsic_gas_low_tx"}, 1)

		return ErrIntrinsicGas
	}

	if tx.Gas > latestBlockGasLimit {
		metrics.IncrCounter([]string{txPoolMetrics, "block_gas_limit_exceeded_tx"}, 1)

		return ErrBlockLimitExceeded
	}

	return nil
}

func (p *TxPool) signalPruning() {
	select {
	case p.pruneCh <- struct{}{}:
	default: //	pruning handler is active or in cooldown
	}
}

func (p *TxPool) pruneAccountsWithNonceHoles() {
	p.accounts.Range(
		func(_, value interface{}) bool {
			account, _ := value.(*account)

			account.enqueued.lock(true)
			defer account.enqueued.unlock()

			account.nonceToTx.lock()
			defer account.nonceToTx.unlock()

			firstTx := account.enqueued.peek()

			if firstTx == nil {
				return true
			}

			if firstTx.Nonce == account.getNonce() {
				return true
			}

			removed := account.enqueued.clear()

			account.nonceToTx.remove(removed...)
			p.index.remove(removed...)
			p.gauge.decrease(slotsRequired(removed...))

			return true
		},
	)
}

// addTx is the main entry point to the pool
// for all new transactions. If the call is
// successful, an account is created for this address
// (only once) and an enqueueRequest is signaled.
func (p *TxPool) addTx(origin txOrigin, tx *types.Transaction) error {
	if p.logger.IsDebug() {
		p.logger.Debug("add tx", "origin", origin.String(), "hash", tx.Hash.String())
	}

	// validate incoming tx
	if err := p.validateTx(tx); err != nil {
		return err
	}

	// add chainID to the tx - only dynamic fee tx
	if tx.Type == types.DynamicFeeTx {
		tx.ChainID = p.chainID
	}

	// calculate tx hash
	tx.ComputeHash(p.store.Header().Number)

	// initialize account for this address once or retrieve existing one
	account := p.getOrCreateAccount(tx.From)

	account.promoted.lock(true)
	account.enqueued.lock(true)
	account.nonceToTx.lock()

	defer func() {
		account.nonceToTx.unlock()
		account.enqueued.unlock()
		account.promoted.unlock()
	}()

	accountNonce := account.getNonce()

	//	only accept transactions with expected nonce
	if p.gauge.highPressure() {
		p.signalPruning()

		if tx.Nonce > accountNonce {
			metrics.IncrCounter([]string{txPoolMetrics, "rejected_future_tx"}, 1)

			return ErrRejectFutureTx
		}
	}

	// try to find if there is transaction with same nonce for this account
	oldTxWithSameNonce := account.nonceToTx.get(tx.Nonce)
	if oldTxWithSameNonce != nil {
		if oldTxWithSameNonce.Hash == tx.Hash {
			metrics.IncrCounter([]string{txPoolMetrics, "already_known_tx"}, 1)

			return ErrAlreadyKnown
		} else if oldTxWithSameNonce.GetGasPrice(p.baseFee).Cmp(
			tx.GetGasPrice(p.baseFee)) >= 0 {
			// if tx with same nonce does exist and has same or better gas price -> return error
			metrics.IncrCounter([]string{txPoolMetrics, "underpriced_tx"}, 1)

			return ErrReplacementUnderpriced
		}
	} else {
		if account.enqueued.length() == account.maxEnqueued && tx.Nonce != accountNonce {
			return ErrMaxEnqueuedLimitReached
		}

		// reject low nonce tx
		if tx.Nonce < accountNonce {
			metrics.IncrCounter([]string{txPoolMetrics, "nonce_too_low_tx"}, 1)

			return ErrNonceTooLow
		}
	}

	slotsAllocated := slotsRequired(tx)

	var slotsFreed uint64
	if oldTxWithSameNonce != nil {
		slotsFreed = slotsRequired(oldTxWithSameNonce)
	}

	var slotsIncreased uint64
	if slotsAllocated > slotsFreed {
		slotsIncreased = slotsAllocated - slotsFreed
		if !p.gauge.increaseWithinLimit(slotsIncreased) {
			return ErrTxPoolOverflow
		}
	}

	// add to index
	if ok := p.index.add(tx); !ok {
		metrics.IncrCounter([]string{txPoolMetrics, "already_known_tx"}, 1)

		if slotsIncreased > 0 {
			p.gauge.decrease(slotsIncreased)
		}

		return ErrAlreadyKnown
	}

	if slotsFreed > slotsAllocated {
		p.gauge.decrease(slotsFreed - slotsAllocated)
	}

	if oldTxWithSameNonce != nil {
		p.index.remove(oldTxWithSameNonce)
	} else {
		metrics.SetGauge([]string{txPoolMetrics, "added_tx"}, 1)
	}

	account.enqueue(tx, oldTxWithSameNonce != nil) // add or replace tx into account

	go p.invokePromotion(tx, tx.Nonce <= accountNonce) // don't signal promotion for higher nonce txs

	return nil
}

func (p *TxPool) invokePromotion(tx *types.Transaction, callPromote bool) {
	p.eventManager.signalEvent(proto.EventType_ADDED, tx.Hash)

	if p.logger.IsDebug() {
		p.logger.Debug("enqueue request", "hash", tx.Hash.String())
	}

	p.eventManager.signalEvent(proto.EventType_ENQUEUED, tx.Hash)

	if callPromote {
		select {
		case <-p.shutdownCh:
		case p.promoteReqCh <- promoteRequest{account: tx.From}: // BLOCKING
		}
	}
}

// handlePromoteRequest handles moving promotable transactions
// of some account from enqueued to promoted. Can only be
// invoked by handleEnqueueRequest or resetAccount.
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
	addr := req.account
	account := p.accounts.get(addr)

	// promote enqueued txs
	promoted, pruned := account.promote()
	if p.logger.IsDebug() {
		p.logger.Debug("promote request", "promoted", promoted, "addr", addr.String())
	}

	p.index.remove(pruned...)
	p.gauge.decrease(slotsRequired(pruned...))

	// update metrics
	p.updatePending(int64(len(promoted)))

	p.eventManager.signalEvent(proto.EventType_PROMOTED, toHash(promoted...)...)
}

// addGossipTx handles receiving transactions
// gossiped by the network.
func (p *TxPool) addGossipTx(obj interface{}, _ peer.ID) {
	if !p.sealing.Load() {
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
			if p.logger.IsDebug() {
				p.logger.Debug("rejecting known tx (gossip)", "hash", tx.Hash.String())
			}

			return
		}

		p.logger.Error("failed to add broadcast tx", "err", err, "hash", tx.Hash.String())
	}
}

// resetAccounts updates existing accounts with the new nonce and prunes stale transactions.
func (p *TxPool) resetAccounts(stateNonces map[types.Address]uint64) {
	if len(stateNonces) == 0 {
		return
	}

	var (
		allPrunedPromoted []*types.Transaction
		allPrunedEnqueued []*types.Transaction
	)

	// clear all accounts of stale txs
	for addr, newNonce := range stateNonces {
		account := p.accounts.get(addr)

		if account == nil {
			// no updates for this account
			continue
		}

		prunedPromoted, prunedEnqueued := account.reset(newNonce, p.promoteReqCh)

		// append pruned
		allPrunedPromoted = append(allPrunedPromoted, prunedPromoted...)
		allPrunedEnqueued = append(allPrunedEnqueued, prunedEnqueued...)

		// new state for account -> demotions are reset to 0
		account.resetDemotions()
	}

	// pool cleanup callback
	cleanup := func(stale []*types.Transaction) {
		p.index.remove(stale...)
		p.gauge.decrease(slotsRequired(stale...))
	}

	// prune pool state
	if len(allPrunedPromoted) > 0 {
		cleanup(allPrunedPromoted)

		p.eventManager.signalEvent(
			proto.EventType_PRUNED_PROMOTED,
			toHash(allPrunedPromoted...)...,
		)

		p.updatePending(int64(-1 * len(allPrunedPromoted)))
	}

	if len(allPrunedEnqueued) > 0 {
		cleanup(allPrunedEnqueued)

		p.eventManager.signalEvent(
			proto.EventType_PRUNED_ENQUEUED,
			toHash(allPrunedEnqueued...)...,
		)
	}
}

// updateAccountSkipsCounts update the accounts' skips,
// the number of the consecutive blocks that doesn't have the account's transactions
func (p *TxPool) updateAccountSkipsCounts(latestActiveAccounts map[types.Address]uint64) {
	stateRoot := p.store.Header().StateRoot
	p.accounts.Range(
		func(key, value interface{}) bool {
			address, _ := key.(types.Address)
			account, _ := value.(*account)

			if _, ok := latestActiveAccounts[address]; ok {
				account.resetSkips()

				return true
			}

			firstTx := account.getLowestTx()
			if firstTx == nil {
				// no need to increment anything,
				// account has no txs
				return true
			}

			if account.incrementSkips() < maxAccountSkips {
				return true
			}

			// account has been skipped too many times
			nextNonce := p.store.GetNonce(stateRoot, firstTx.From)
			p.dropAccount(account, nextNonce, firstTx)

			account.resetSkips()

			return true
		},
	)
}

// getOrCreateAccount creates an account and
// ensures it is only initialized once.
func (p *TxPool) getOrCreateAccount(newAddr types.Address) *account {
	if account := p.accounts.get(newAddr); account != nil {
		return account
	}

	// fetch nonce from state
	stateRoot := p.store.Header().StateRoot
	stateNonce := p.store.GetNonce(stateRoot, newAddr)

	// initialize the account
	return p.accounts.initOnce(newAddr, stateNonce)
}

// Length returns the total number of all promoted transactions.
func (p *TxPool) Length() uint64 {
	return p.accounts.promoted()
}

// toHash returns the hash(es) of given transaction(s)
func toHash(txs ...*types.Transaction) (hashes []types.Hash) {
	for _, tx := range txs {
		hashes = append(hashes, tx.Hash)
	}

	return
}
