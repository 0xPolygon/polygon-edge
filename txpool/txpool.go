package txpool

import (
	"container/heap"
	"errors"
	"fmt"
	"math/big"
	"sort"
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
)

var (
	ErrIntrinsicGas        = errors.New("intrinsic gas too low")
	ErrNegativeValue       = errors.New("negative value")
	ErrNonEncryptedTxn     = errors.New("non-encrypted transaction")
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

type TxOrigin = string

const (
	OriginAddTxn TxOrigin = "addTxn"
	OriginReorg  TxOrigin = "reorg"
	OriginGossip TxOrigin = "gossip"
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

// Gauge for measuring pool capacity in slots
type slotGauge struct {
	sync.Mutex
	height uint64
	limit  uint64
}

// Increases the height of the gauge by the specified slots amount
func (g *slotGauge) increase(slots uint64) {
	g.Lock()
	defer g.Unlock()

	g.height += slots
}

// Decreases the height of the gauge by the specified slots amount
func (g *slotGauge) decrease(slots uint64) {
	g.Lock()
	defer g.Unlock()

	g.height -= slots
}

// Returns the current height of the gauge measured in slots
func (g *slotGauge) getHeight() uint64 {
	g.Lock()
	defer g.Unlock()

	return g.height
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
	accountQueuesLock sync.Mutex
	accountQueues     map[types.Address]*accountQueueWrapper

	// Max price heap for all transactions that are valid
	pendingQueue *txPriceHeap

	// Min price heap for all remote transactions
	remoteTxns *txPriceHeap

	// Lookup map that keeps track of txns present in the pool
	txnLookupMap map[types.Hash]*types.Transaction

	// Lock for the txn lookup map
	txnLookupMapLock sync.RWMutex

	// Gauge for measuring pool capacity
	gauge slotGauge

	// Networking stack
	topic *network.Topic

	// Flag indicating if the current node is a sealer,
	// and should therefore gossip transactions
	sealing bool

	// Flag indicating if the current node is running in dev mode
	dev bool

	// Whether local transaction handling should be disabled
	noLocals bool

	// Addresses that should be treated as local
	locals *localAccounts

	// priceLimit is a lower threshold for gas price
	priceLimit uint64

	// Notification channel used so signal added transactions to the pool
	NotifyCh chan struct{}

	// Indicates which txpool operator commands should be implemented
	proto.UnimplementedTxnPoolOperatorServer

	metrics *Metrics
}

// NewTxPool creates a new pool for transactions
func NewTxPool(
	logger hclog.Logger,
	sealing bool,
	locals []types.Address,
	noLocals bool,
	priceLimit uint64,
	maxSlots uint64,
	forks chain.ForksInTime,
	store store,
	grpcServer *grpc.Server,
	network *network.Server,
	metrics *Metrics,
) (*TxPool, error) {
	txPool := &TxPool{
		logger:        logger.Named("txpool"),
		store:         store,
		idlePeriod:    defaultIdlePeriod,
		accountQueues: make(map[types.Address]*accountQueueWrapper),
		pendingQueue:  newMaxTxPriceHeap(),
		remoteTxns:    newMinTxPriceHeap(),
		txnLookupMap:  make(map[types.Hash]*types.Transaction),
		gauge:         slotGauge{height: 0, limit: maxSlots},
		sealing:       sealing,
		locals:        newLocalAccounts(locals),
		noLocals:      noLocals,
		priceLimit:    priceLimit,
		forks:         forks,
		metrics:       metrics,
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

// accountQueueWrapper is the account based queue lock map implementation
type accountQueueWrapper struct {
	lock         sync.RWMutex // lock for accessing the accountQueue
	writeLock    int32        // flag indicating whether a write lock is held
	accountQueue *txHeapWrapper
}

// lockAccountQueue returns the corresponding account queue wrapper object, or creates it
// if it doesn't exist in the account queue map
func (t *TxPool) lockAccountQueue(address types.Address, writer bool) *accountQueueWrapper {
	// Lock the global map
	t.accountQueuesLock.Lock()

	accountQueue, ok := t.accountQueues[address]
	if !ok {
		// Account queue is not initialized yet, initialize it
		stateRoot := t.store.Header().StateRoot

		// Initialize the account based transaction heap
		txnsQueue := newTxHeapWrapper(t.logger.Named("account"))
		txnsQueue.nextNonce = t.store.GetNonce(stateRoot, address)

		accountQueue = &accountQueueWrapper{
			accountQueue: txnsQueue,
		}
		t.accountQueues[address] = accountQueue
	}
	// Unlock the global map, since work is finished
	t.accountQueuesLock.Unlock()

	// Grab the lock for the specific account queue
	if writer {
		accountQueue.lock.Lock()
		atomic.StoreInt32(&accountQueue.writeLock, 1)
	} else {
		accountQueue.lock.RLock()
		atomic.StoreInt32(&accountQueue.writeLock, 0)
	}

	return accountQueue
}

// unlock releases the account specific transaction queue lock.
// Separated out into a function in case there needs to be additional teardown logic.
// Code calling unlock shouldn't need to know the type of lock for the lock (writer / reader) to unlock it
func (a *accountQueueWrapper) unlock() {
	// Grab the previous lock type and reset it
	if atomic.SwapInt32(&a.writeLock, 0) == 1 {
		a.lock.Unlock()
	} else {
		a.lock.RUnlock()
	}
}

// pruneAccountTx is a helper method for making sure the account specific queue
// doesn't have any stale transactions with an invalid nonce [NOT Thread-safe]
func (a *accountQueueWrapper) pruneAccountTx(
	pruneCallback func(txn *types.Transaction), // Callback for additional prune logic
) {
	for {
		// Check if the nonce is lower than what the TxPool is expecting
		lowestNonceTx := a.accountQueue.txs.Peek()

		if lowestNonceTx == nil || // There is nothing in the account specific queue
			lowestNonceTx.Nonce >= a.accountQueue.nextNonce { // The lowest nonce tx is valid
			// All good
			break
		}

		// Remove the transaction
		a.accountQueue.logger.Debug(
			fmt.Sprintf(
				"Dropping txn [%s] due to a low nonce [%d < %d]",
				lowestNonceTx.Hash.String(),
				lowestNonceTx.Nonce,
				a.accountQueue.nextNonce,
			),
		)
		a.accountQueue.txs.Pop()

		pruneCallback(lowestNonceTx)
	}
}

// GetNonce returns the next nonce for the account
// -> Returns the value from the TxPool if the account is initialized in-memory
// -> Returns the value from the world state otherwise
func (t *TxPool) GetNonce(addr types.Address) uint64 {
	// Grab the account queue lock
	wrapper := t.lockAccountQueue(addr, true)
	defer wrapper.unlock()

	return wrapper.accountQueue.nextNonce
}

// GetCapacity returns the current number of slots occupied and the max slot limit
func (t *TxPool) GetCapacity() (uint64, uint64) {
	return t.gauge.getHeight(), t.gauge.limit
}

// NumAccountTxs Returns the number of transactions in the account specific queue
func (t *TxPool) NumAccountTxs(address types.Address) int {
	wrapper := t.lockAccountQueue(address, false)
	defer wrapper.unlock()

	return len(wrapper.accountQueue.txs)
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
		if err := t.addImpl(OriginGossip, txn); err != nil {
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
	if err := t.addImpl(OriginAddTxn, tx); err != nil {
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
func (t *TxPool) addImpl(origin TxOrigin, tx *types.Transaction) error {
	// Since this is a single point of inclusion for new transactions both
	// to the promoted queue and pending queue we use this point to calculate the hash
	tx.ComputeHash()

	if _, isAvailable := t.GetPendingTx(tx.Hash); isAvailable {
		t.logger.Info("Discarding already known transaction", "hash", tx.Hash)
		return ErrAlreadyKnown
	}
	// should treat as local in the following cases
	// (1) noLocals is false and Tx is local transaction
	// (2) from in tx is in locals addresses
	isLocal := (!t.noLocals && origin == OriginAddTxn) || t.locals.containsTxSender(t.signer, tx)
	err := t.validateTx(tx, isLocal)
	if err != nil {
		t.logger.Error("Discarding invalid transaction", "hash", tx.Hash, "err", err)
		return err
	}

	// Reject transactions with lower nonce than expected by the account queue
	if nextNonce := t.GetNonce(tx.From); tx.Nonce < nextNonce {
		t.logger.Debug(
			fmt.Sprintf(
				"Rejecting tx [%s] from account heap due to low nonce",
				tx.Hash.String()),
		)

		return ErrNonceTooLow
	}

	// check for slot overflow and handle accordingly
	if err := t.processSlots(tx, isLocal); err != nil {
		return err
	}

	t.logger.Debug("add txn", "ctx", origin, "hash", tx.Hash, "from", tx.From)

	wrapper := t.lockAccountQueue(tx.From, true)
	defer wrapper.unlock()

	wrapper.accountQueue.Add(tx)

	if !isLocal {
		if pushErr := t.remoteTxns.Push(tx); pushErr != nil {
			t.logger.Error(
				fmt.Sprintf(
					"Unable to push txn [%s] to the remote txns queue",
					tx.Hash.String(),
				),
			)
		}
	}

	// Skip check of GasPrice in the future transactions created by same address when TxPool receives transaction by Gossip or Reorg
	if isLocal && !t.locals.containsAddr(tx.From) {
		t.locals.addAddr(tx.From)
	}

	// Move promotable txs to the pending queue
	for _, promoted := range wrapper.accountQueue.Promote() {
		if pushErr := t.pendingQueue.Push(promoted); pushErr != nil {
			t.logger.Error(fmt.Sprintf("Unable to promote transaction %s, %v", promoted.Hash.String(), pushErr))
		} else {
			t.metrics.PendingTxs.Add(1)
		}
	}

	// Add the transaction to the lookup map
	t.addTxToLookup(tx)

	return nil
}

// DecreaseAccountNonce resets the nonce attached to an account whenever a transaction produce an error which is not
// recoverable, meaning the transaction will be discarded.
//
// Since any discarded transaction should not affect the world state, the nextNonce should be reset to the value
// it was set to before the transaction appeared.
func (t *TxPool) DecreaseAccountNonce(tx *types.Transaction) {
	wrapper := t.lockAccountQueue(tx.From, true)
	defer wrapper.unlock()

	if wrapper.accountQueue.nextNonce > 0 {
		wrapper.accountQueue.nextNonce--
	}
}

// GetTxs gets pending and queued transactions
func (t *TxPool) GetTxs(inclQueued bool) (map[types.Address]map[uint64]*types.Transaction, map[types.Address]map[uint64]*types.Transaction) {
	t.pendingQueue.lock.Lock()
	pendingTxs := make(map[types.Address]map[uint64]*types.Transaction)
	sortedPricedTxs := t.pendingQueue.index
	for _, sortedPricedTx := range sortedPricedTxs {
		if _, ok := pendingTxs[sortedPricedTx.from]; !ok {
			pendingTxs[sortedPricedTx.from] = make(map[uint64]*types.Transaction)
		}
		pendingTxs[sortedPricedTx.from][sortedPricedTx.tx.Nonce] = sortedPricedTx.tx
	}
	t.pendingQueue.lock.Unlock()
	if !inclQueued {
		return pendingTxs, nil
	}

	queuedTxs := make(map[types.Address]map[uint64]*types.Transaction)
	queue := t.accountQueues
	for addr, queuedTxn := range queue {
		wrapper := t.lockAccountQueue(addr, false)
		for _, tx := range queuedTxn.accountQueue.txs {
			if _, ok := queuedTxs[addr]; !ok {
				queuedTxs[addr] = make(map[uint64]*types.Transaction)
			}
			queuedTxs[addr][tx.Nonce] = tx
		}
		wrapper.unlock()
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

	//Update the pending transaction metric
	t.metrics.PendingTxs.Set(float64(t.pendingQueue.Length()))

	slots := slotsRequired(txn.tx)
	// Subtracts tx slots
	t.gauge.decrease(slots)
	ret := func() {
		if pushErr := t.pendingQueue.Push(txn.tx); pushErr != nil {
			t.logger.Error(fmt.Sprintf("Unable to promote transaction %s, %v", txn.tx.Hash.String(), pushErr))
			return
		} else {
			t.metrics.PendingTxs.Add(1)
		}
		t.gauge.increase(slots)
	}
	return txn.tx, ret
}

// GetPendingTx returns the transaction by hash in the TxPool (pending txn) [Thread-safe]
func (t *TxPool) GetPendingTx(txHash types.Hash) (*types.Transaction, bool) {
	t.txnLookupMapLock.RLock()
	defer t.txnLookupMapLock.RUnlock()

	txn, ok := t.txnLookupMap[txHash]
	return txn, ok
}

// addTxToLookup adds a transaction to the lookup map [Thread-safe]
func (t *TxPool) addTxToLookup(tx *types.Transaction) {
	t.txnLookupMapLock.Lock()
	defer t.txnLookupMapLock.Unlock()

	t.txnLookupMap[tx.Hash] = tx
}

// deleteTxFromLookup removes a transaction from the lookup map [Thread-safe]
func (t *TxPool) deleteTxFromLookup(txHash types.Hash) {
	t.txnLookupMapLock.Lock()
	defer t.txnLookupMapLock.Unlock()

	delete(t.txnLookupMap, txHash)
}

// batchDeleteTxFromLookup removes a batch of transactions from the lookup map [Thread-safe]
func (t *TxPool) batchDeleteTxFromLookup(txns []*types.Transaction) {
	t.txnLookupMapLock.Lock()
	defer t.txnLookupMapLock.Unlock()

	for _, txn := range txns {
		delete(t.txnLookupMap, txn.Hash)
	}
}

// ResetWithHeader does basic txpool housekeeping after a block write
func (t *TxPool) ResetWithHeader(h *types.Header) {
	evnt := &blockchain.Event{
		NewChain: []*types.Header{h},
	}

	// Process the txns in the event to make sure the TxPool is up-to-date
	t.ProcessEvent(evnt)
}

// processEventWrapper holds metadata information related
// to a new block insertion event that relate to a specific account
type processEventWrapper struct {
	stateNonce   uint64
	transactions []*types.Transaction
}

// addTxn adds a transaction to the account's process event wrapper
func (p *processEventWrapper) addTxn(txn *types.Transaction) {
	p.transactions = append(p.transactions, txn)
}

// promotedTxnCleanup looks through the promoted queue for any invalid transactions
// made by a specific account, and removes them
func (t *TxPool) promotedTxnCleanup(
	address types.Address, // The address to filter by
	stateNonce uint64, // The valid nonce (reference for pruning)
	cleanupCallback func(txn *types.Transaction), // Additional cleanup logic
) {
	// Prune out all the now possibly low-nonce transactions in the promoted queue
	t.pendingQueue.lock.Lock()

	// Find the txns that correspond to this account
	droppedPendingTxs := 0
	for _, pendingQueueTxn := range t.pendingQueue.index {
		// Check if the txn in the promoted queue matches the search criteria
		if pendingQueueTxn.from == address && // The sender of this txn is the account we're looking for
			pendingQueueTxn.tx.Nonce < stateNonce { // The nonce on this promoted txn is invalid
			// Transaction found, drop it from the pending queue
			if dropped := t.pendingQueue.dropTx(pendingQueueTxn.tx); dropped {
				// Update the log data
				droppedPendingTxs++
				t.logger.Debug(
					fmt.Sprintf(
						"Dropping promoted txn [%s]",
						pendingQueueTxn.tx.Hash.String(),
					),
				)

				cleanupCallback(pendingQueueTxn.tx)
			}
		}
	}

	t.pendingQueue.lock.Unlock()

	// Print out the number of dropped pending txns
	t.logger.Debug(
		fmt.Sprintf(
			"Dropped %d promoted txns for account [%s]",
			droppedPendingTxs,
			address.String(),
		),
	)
}

// extractTransactions Groups the transactions by account and queries the state
// for the latest nonce data
func (t *TxPool) extractTransactions(evnt *blockchain.Event) map[types.Address]*processEventWrapper {
	// Instantiate the account process event wrapper map.
	// It is used for grouping transactions on an account basis
	// and for storing next state nonce information for easy lookup
	// Account address -> processEventWrapper
	eventWrapperMap := make(map[types.Address]*processEventWrapper)

	// Grab the latest state root now that the block has been inserted
	stateRoot := t.store.Header().StateRoot

	// Legacy reorg logic //
	addTxns := map[types.Hash]*types.Transaction{}
	for _, evnt := range evnt.OldChain {
		// reinject these transactions on the pool
		block, ok := t.store.GetBlockByHash(evnt.Hash, true)
		if !ok {
			t.logger.Error("block not found on txn add", "hash", block.Hash())
			continue
		}

		for _, txn := range block.Transactions {
			addTxns[txn.Hash] = txn
		}
	}

	// Keep track of all the transactions
	for _, evnt := range evnt.NewChain {
		// Grab the block that has just been written to state
		block, ok := t.store.GetBlockByHash(evnt.Hash, true)
		if !ok {
			t.logger.Error("block not found on txn del", "hash", block.Hash())
			continue
		}
		// Compile transactions that should be accounted for in the TxPool
		for _, txn := range block.Transactions {
			// Save the transaction
			eventWrapper, wrapperFound := eventWrapperMap[txn.From]
			if !wrapperFound {
				// Initialize the wrapper
				eventWrapper = &processEventWrapper{
					// Grab the latest nonce from state
					stateNonce: t.store.GetNonce(stateRoot, txn.From),
					// Set up the transaction array
					transactions: make([]*types.Transaction, 0),
				}

				eventWrapperMap[txn.From] = eventWrapper
			}

			// Add the transaction to the wrapper
			eventWrapper.addTxn(txn)

			// Legacy reorg logic //
			// Update the addTxns in case of reorgs
			delete(addTxns, txn.Hash)
		}
	}

	// Legacy reorg logic //
	// try to include again the transactions in the pendingQueue list
	for _, txn := range addTxns {
		if err := t.addImpl(OriginReorg, txn); err != nil {
			t.logger.Error("failed to add txn", "err", err)
		}
	}

	return eventWrapperMap
}

// ProcessEvent processes the blockchain event and updates the txpool accordingly
func (t *TxPool) ProcessEvent(evnt *blockchain.Event) {
	// Extract and group the transactions from the new block event
	eventWrapperMap := t.extractTransactions(evnt)

	// txDropCleanup is a helper method for updating the gauge size,
	// as well as removing leftover remote txns
	txnDropCleanup := func(txn *types.Transaction) {
		// Decrease the slots taken up by this txn
		t.gauge.decrease(slotsRequired(txn))

		// Remove the txn from the remote txn queue,
		// if it's present
		t.remoteTxns.Delete(txn)

		// Remove the txn from the lookup map
		t.deleteTxFromLookup(txn.Hash)
	}

	// Remove the txns from the block that were just committed to state
	// from any queues in the TxPool
	for address, accountEventWrapper := range eventWrapperMap {
		// Grab the lock for the account specific queue
		wrapper := t.lockAccountQueue(address, true)

		stateNonce := accountEventWrapper.stateNonce

		// Attempt to update the next account nonce in the TxPool
		// with the one in state if it's greater.
		// Check if the state nonce is greater than the nonce present in the txpool
		// If so -> realign it to the state nonce
		if stateNonce > wrapper.accountQueue.nextNonce {
			t.logger.Debug(
				fmt.Sprintf(
					"World state nonce [%d] for account [%s] is > the TxPool nonce [%d]",
					stateNonce,
					address.String(),
					wrapper.accountQueue.nextNonce,
				),
			)
			wrapper.accountQueue.nextNonce = stateNonce
		}

		// Delete the transactions from the lookup map
		t.batchDeleteTxFromLookup(accountEventWrapper.transactions)

		// Since there have been state changes, the TxPool can still have hanging txns.
		// Prune out all the now possibly low-nonce transactions in the account queue
		wrapper.pruneAccountTx(txnDropCleanup)

		// Release the lock for the specific account queue
		wrapper.unlock()

		// Make sure the promoted queue doesn't have leftover transactions
		t.promotedTxnCleanup(address, stateNonce, txnDropCleanup)
	}

	// update the metrics
	t.metrics.PendingTxs.Set(float64(t.pendingQueue.Length()))
}

// validateTx validates that the transaction conforms to specific constraints to be added to the txpool
func (t *TxPool) validateTx(
	tx *types.Transaction, // The transaction that should be validated
	isLocal bool, // Flag indicating if the transaction is from a local account
) error {
	// Check the transaction size to overcome DOS Attacks
	if uint64(len(tx.MarshalRLP())) > txMaxSize {
		return ErrOversizedData
	}

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

	// Reject non-local transactions whose Gas Price is under priceLimit
	if !isLocal && tx.GasPrice.Cmp(big.NewInt(int64(t.priceLimit))) < 0 {
		return ErrUnderpriced
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

// Underpriced checks whether given tx's price is less than any in remote transactions
func (t *TxPool) Underpriced(tx *types.Transaction) bool {
	lowestTx := t.remoteTxns.Pop()
	if lowestTx == nil {
		return false
	}
	// tx.GasPrice < lowestTx.Price
	underpriced := tx.GasPrice.Cmp(lowestTx.price) < 0
	t.remoteTxns.Push(lowestTx.tx)
	return underpriced
}

func (t *TxPool) Discard(slotsToRemove uint64, force bool) ([]*types.Transaction, bool) {
	dropped := make([]*types.Transaction, 0)
	for slotsToRemove > 0 {
		if t.remoteTxns.Length() == 0 {
			break
		}

		pricedTx := t.remoteTxns.Pop()
		dropped = append(dropped, pricedTx.tx)

		txSlots := slotsRequired(pricedTx.tx)
		if slotsToRemove < txSlots {
			return dropped, true
		}

		slotsToRemove -= txSlots
	}

	// Put back if couldn't make required space
	if slotsToRemove > 0 && !force {
		for _, tx := range dropped {
			t.remoteTxns.Push(tx)
		}
		return nil, false
	}

	return dropped, true
}

// Checks if the incoming tx would cause an overflow
// and attempts to allocate space for it
func (t *TxPool) processSlots(tx *types.Transaction, isLocal bool) error {
	t.gauge.Lock()
	defer t.gauge.Unlock()

	txSlots := slotsRequired(tx)
	if t.gauge.height+txSlots <= t.gauge.limit {
		// no overflow, just increase the height
		t.gauge.height += txSlots
		return nil
	}

	// reject remote tx with lower gasPrice
	// than the min gasPrice tx currently present in remoteTxns
	if !isLocal && t.Underpriced(tx) {
		return ErrUnderpriced
	}

	// try to allocate space
	overflow := t.gauge.height + txSlots - t.gauge.limit
	dropped, success := t.Discard(overflow, isLocal)
	if !isLocal && !success {
		return ErrTxPoolOverflow
	}

	// clear dropped txs and readjust gauge
	for _, tx := range dropped {
		wrapper := t.lockAccountQueue(tx.From, true)
		wrapper.accountQueue.Remove(tx.Hash)
		wrapper.unlock()

		t.pendingQueue.Delete(tx)
		t.gauge.height -= slotsRequired(tx)
	}
	t.gauge.height += txSlots

	t.metrics.PendingTxs.Set(float64(t.pendingQueue.Length()))
	return nil
}

// txHeapWrapper is a wrapper object for account based transactions
type txHeapWrapper struct {
	// txs is the actual min heap (nonce ordered) for account transactions
	txs txHeap

	// nextNonce is a field indicating what should be the next
	// valid nonce for the account transaction
	nextNonce uint64

	// Logger used for account-specific tx activity
	logger hclog.Logger
}

// newTxHeapWrapper creates a new account based tx heap
func newTxHeapWrapper(logger hclog.Logger) *txHeapWrapper {
	return &txHeapWrapper{
		logger: logger,
		txs:    txHeap{},
	}
}

// Add adds a new tx onto the account based tx heap
func (t *txHeapWrapper) Add(tx *types.Transaction) {
	t.Push(tx)
}

// Promote promotes all the new valid transactions
func (t *txHeapWrapper) Promote() []*types.Transaction {
	// Promote elements
	tx := t.Peek()
	if tx == nil || tx.Nonce != t.nextNonce {
		// Nothing to promote
		t.logger.Debug("No txs to promote")
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
			t.logger.Debug(
				fmt.Sprintf(
					"Shelving tx [%s] with higher nonce [%d] for later",
					tx.Hash.String(),
					tx.Nonce,
				),
			)
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

// Remove removes the transaction with given hash
func (t *txHeapWrapper) Remove(hash types.Hash) bool {
	for i, tx := range t.txs {
		if tx.Hash == hash {
			t.txs = append(t.txs[:i], t.txs[i+1:]...)
			return true
		}
	}
	return false
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
type pricedTx struct {
	tx    *types.Transaction
	from  types.Address
	price *big.Int
	index int
}

// helper object for tx price heap
type txPriceHeap struct {
	lock  sync.Mutex
	index map[types.Hash]*pricedTx
	heap  heap.Interface
}

func (t *txPriceHeap) Length() uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return uint64(len(t.index))
}

// dropTx removes a transaction from the priced heap. [Thread-unsafe]
func (t *txPriceHeap) dropTx(tx *types.Transaction) bool {
	item, ok := t.index[tx.Hash]
	if !ok {
		return false
	}

	delete(t.index, tx.Hash)
	heap.Remove(t.heap, item.index)

	return true
}

// Delete removes a transaction from the priced heap. [Thread-safe]
func (t *txPriceHeap) Delete(tx *types.Transaction) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.dropTx(tx)
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
	heap.Push(t.heap, pTx)
	return nil
}

func (t *txPriceHeap) Pop() *pricedTx {
	t.lock.Lock()
	defer t.lock.Unlock()

	if len(t.index) == 0 {
		return nil
	}
	tx := heap.Pop(t.heap).(*pricedTx)
	delete(t.index, tx.tx.Hash)
	return tx
}

func (t *txPriceHeap) Contains(tx *types.Transaction) bool {
	_, ok := t.index[tx.Hash]
	return ok
}

// return new max-price ordered tx heap
func newMaxTxPriceHeap() *txPriceHeap {
	return &txPriceHeap{
		index: make(map[types.Hash]*pricedTx),
		heap:  newMaxTxPriceHeapImpl(),
	}
}

// return new min-price ordered tx heap
func newMinTxPriceHeap() *txPriceHeap {
	return &txPriceHeap{
		index: make(map[types.Hash]*pricedTx),
		heap:  newMinTxPriceHeapImpl(),
	}
}

// Required method definitions for the standard golang heap package
type txPriceHeapImplBase struct {
	txs []*pricedTx
}

func (t txPriceHeapImplBase) Len() int { return len(t.txs) }

func (t txPriceHeapImplBase) Swap(i, j int) {
	t.txs[i], t.txs[j] = t.txs[j], t.txs[i]
	t.txs[i].index = i
	t.txs[j].index = j
}

func (t *txPriceHeapImplBase) Push(x interface{}) {
	n := len(t.txs)
	job := x.(*pricedTx)
	job.index = n
	t.txs = append(t.txs, job)
}

func (t *txPriceHeapImplBase) Pop() interface{} {
	old := *t
	n := len(old.txs)
	job := old.txs[n-1]
	job.index = -1
	t.txs = old.txs[0 : n-1]
	return job
}

func (t txPriceHeapImplBase) Less(i, j int) bool {
	return i < j
}

// max price ordered tx heap implementation
type maxTxPriceHeapImpl struct {
	txPriceHeapImplBase
}

func newMaxTxPriceHeapImpl() heap.Interface {
	return &maxTxPriceHeapImpl{
		txPriceHeapImplBase: txPriceHeapImplBase{
			make([]*pricedTx, 0),
		},
	}
}

func (t maxTxPriceHeapImpl) Less(i, j int) bool {
	if t.txs[i].from == t.txs[j].from {
		return t.txs[i].tx.Nonce < t.txs[j].tx.Nonce
	}

	return t.txs[i].price.Cmp(t.txs[j].price) >= 0
}

type minTxPriceHeapImpl struct {
	txPriceHeapImplBase
}

// min price ordered tx heap implementation
func newMinTxPriceHeapImpl() heap.Interface {
	return &minTxPriceHeapImpl{
		txPriceHeapImplBase: txPriceHeapImplBase{
			make([]*pricedTx, 0),
		},
	}
}

func (t minTxPriceHeapImpl) Less(i, j int) bool {
	if t.txs[i].from == t.txs[j].from {
		return t.txs[i].tx.Nonce < t.txs[j].tx.Nonce
	}

	return t.txs[i].price.Cmp(t.txs[j].price) < 0
}

type localAccounts struct {
	accounts map[types.Address]bool
	mutex    sync.RWMutex
}

func newLocalAccounts(addrs []types.Address) *localAccounts {
	accounts := make(map[types.Address]bool, len(addrs))
	for _, addr := range addrs {
		accounts[addr] = true
	}
	return &localAccounts{
		accounts: accounts,
		mutex:    sync.RWMutex{},
	}
}

func (a *localAccounts) containsAddr(addr types.Address) bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.accounts[addr]
}

func (a *localAccounts) containsTxSender(signer signer, tx *types.Transaction) bool {
	if addr, err := signer.Sender(tx); err == nil {
		return a.containsAddr(addr)
	}
	return false
}

func (a *localAccounts) addAddr(addr types.Address) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.accounts[addr] = true
}

// slotsRequired() calculates the number of slotsRequired for given transaction
func slotsRequired(tx *types.Transaction) uint64 {
	return (tx.Size() + txSlotSize - 1) / txSlotSize
}
