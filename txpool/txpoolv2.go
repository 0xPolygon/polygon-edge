package txpool

import (
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
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

	// map of all account queues (enqueued transactions)
	enqueued accountsMap

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
		enqueued:   accountsMap{},
		promoted:   newPromotedQueue(logger.Named("promoted")),
		index:      lookupMap{all: make(map[types.Hash]statusTx)},
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

func (p *TxPool) Start() {
}

// handleAddRequest is invoked when a new transaction is received
// (result of a successful addTx() call) or a recovered transaction
// is re-entering the pool.
//
// A transaction handled within this request can either be
// dropped or enqueued, eventually signaling promotion in the former case.
func (p *TxPool) handleAddRequest(req addRequest) {
}

// handlePromoteRequest handles moving promtable transactions
// from the associated account queue to the promoted queue.
// Can only be invoked by handleAddRequest
func (p *TxPool) handlePromoteRequest(req promoteRequest) {
}

// handleResetRequest is called within ResetWithHeader
// and aligns the pool's state for all accounts by pruning
// stale transactions from all queues in the pool.
func (p *TxPool) handleResetRequest(req resetRequest) {
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
	return nil
}

// Pop() removes the highest priced transaction from the promoted queue.
// Assumes the lock is held.
func (p *TxPool) Pop() *types.Transaction {
	return nil
}

// Recover is called within ibft for all transactions
// that are valid but couldn't be written to the state
// at the given time. Issues an addRequest to the pool
// indicating a transaction is returning to it.
func (p *TxPool) Recover(tx *types.Transaction) {
}

// Rollback is called within ibft for any transactions
// deemed unrecoverable during writing to the state.
// This call ensures that any subsequent transactions
// must not be processed before the unrecoverable one
// is re-sent again.
func (p *TxPool) RollbackNonce(tx *types.Transaction) {
}

// ResetWithHeader is called from within ibft when the node
// has received a new block from a peer. The pool needs to align
// its own state with the new one so it can correctly process
// further incoming transactions.
func (p *TxPool) ResetWithHeader(h *types.Header) {
}

func (p *TxPool) LockPromoted(write bool) {
}

func (p *TxPool) UnlockPromoted() {
}

// prunePromoted cleans out any transactions from the promoted queue
// considered stale by the given nonceMap.
func (p *TxPool) prunePromoted(nonceMap map[types.Address]uint64) uint64 {
	return 0
}

// pruneEnqueued cleans out any transactions from the accouunt queues
// considered stale by the given nonceMap.
func (p *TxPool) pruneEnqueued(nonceMap map[types.Address]uint64) uint64 {
	return 0
}

// validateTx ensures that the transaction conforms
// to specific constraints before entering the pool.
func (p *TxPool) validateTx(tx *types.Transaction) error {
	return nil
}

// addTx is the main entry point to the pool
// for all received transactions. If the call to
// addTx() is successful an account queue is created
// for this address (only once) and an addRequest is sent.
func (p *TxPool) addTx(tx *types.Transaction) error {
	return nil
}

// handleGossipTxn handles receiving gossiped transactions
func (p *TxPool) handleGossipTxn(obj interface{}) {
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
// This function ensures that the account queue and its corresponding lock
// are  created safely and only once.
func (p *TxPool) createAccountOnce(newAddr types.Address) {
}

// lockAccountQueue locks the account queue of the given address
func (p *TxPool) lockAccountQueue(addr types.Address, write bool) *accountQueue {
	return nil
}

// unlockAccountQueue unlock the account queue of the given address
func (p *TxPool) unlockAccountQueue(addr types.Address) {
}

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
	all map[types.Hash]statusTx
}

func (m *lookupMap) add(status status, txs ...*types.Transaction) {
}

func (m *lookupMap) remove(txs ...*types.Transaction) {
}

func (m *lookupMap) load(hash types.Hash) (*types.Transaction, bool) {
	return nil, false
}

// Map of expected nonces for all (known) accounts
type nonceMap struct {
	sync.Map
}

func (m *nonceMap) load(addr types.Address) (uint64, bool) {
	return 0, false
}

func (m *nonceMap) store(addr types.Address, nonce uint64) {
}

// Thread safe map of all account queue registered by the pool
type accountsMap struct {
	sync.Map
}

// from returns the account queue of the gives address
func (m *accountsMap) from(addr types.Address) *accountQueue {
	return nil
}

// TODO
type accountQueue struct {
	sync.RWMutex
	initFunc sync.Once
	logger   hclog.Logger
	wLock    uint32
	txs      minNonceQueue
}

func (q *accountQueue) push(txs ...*types.Transaction) {
}

func (q *accountQueue) pop() *types.Transaction {
	return nil
}

func (q *accountQueue) peek() *types.Transaction {
	return nil
}

func (q *accountQueue) length() uint64 {
	return 0
}

func (q *accountQueue) promote(nonce uint64) (transactions, uint64) {
	return nil, 0
}

func (q *accountQueue) pruneLowNonce(nonce uint64) transactions {
	return nil
}

type promotedQueue struct {
	sync.RWMutex
	logger hclog.Logger
	txs    maxPriceQueue
	wLock  uint32
}

func newPromotedQueue(logger hclog.Logger) *promotedQueue {
	return nil
}

func (q *promotedQueue) push(txs ...*types.Transaction) {
}

func (q *promotedQueue) peek() *types.Transaction {
	return nil
}

func (q *promotedQueue) pop() *types.Transaction {
	return nil
}

func (q *promotedQueue) length() uint64 {
	return 0
}

/* queue implementations */

type transactions []*types.Transaction

type minNonceQueue transactions

func (q *minNonceQueue) Peek() *types.Transaction {
	return nil
}

func (q *minNonceQueue) Len() int {
	return 0
}

func (q *minNonceQueue) Swap(i, j int) {
}

func (q *minNonceQueue) Less(i, j int) bool {
	return false
}

func (q *minNonceQueue) Push(x interface{}) {
}

func (q *minNonceQueue) Pop() interface{} {
	return nil
}

type maxPriceQueue transactions

func (q *maxPriceQueue) Peek() *types.Transaction {
	return nil
}

func (q *maxPriceQueue) Len() int {
	return 0
}

func (q *maxPriceQueue) Swap(i, j int) {
}

func (q *maxPriceQueue) Less(i, j int) bool {
	return false
}

func (q *maxPriceQueue) Push(x interface{}) {
}

func (q *maxPriceQueue) Pop() interface{} {
	return nil
}
