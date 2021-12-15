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
	txMaxSize         = 128 * 1024 // 128Kb
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
	tx      *types.Transaction
	isLocal bool
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
	nonce   uint64
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

	// Lookup map keeping track of all
	// transactions present in the pool
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

// NewTxPool creates a new pool for transactions
func NewTxPool(
	logger hclog.Logger,
	forks chain.ForksInTime,
	store store,
	grpcServer *grpc.Server,
	network *network.Server,
	metrics *Metrics,
	config *Config,
) (*TxPool, error) {
	return nil, nil
}

// Pool's main loop listening to and handling requests
func (p *TxPool) runLoop() {
	for {
		select {
		case req := <-p.addReqCh:
			p.handleAddRequest(req)
		case req := <-p.promoteReqCh:
			p.handlePromoteRequest(req)
		case req := <-p.resetReqCh:
			p.handleResetRequest(req)
		case req := <-p.rollbackReqCh:
			p.handleRollbackRequest(req)
		}
	}
}

// handler threads output result
type handlerEvent int

const (
	// add
	txRejected handlerEvent = iota
	txEnqueued
	signalPromotion

	// promote
	accountPromoted
	nothingToPromote

	// reset
	resetDone

	// rollback
	rollbackDone
)

// handleAddRequest enqueues a transaction
// to its designated account queue, if possible
func (p *TxPool) handleAddRequest(req addRequest) <-chan handlerEvent {
	done := make(chan handlerEvent, 1)

	go func(tx *types.Transaction) {

	}(req.tx)

	return done
}

// handlePromoteRequest handles promoting transactions
// from the account queue.
// Can only be triggered by an appropriate add request (nonce is expected)
func (p *TxPool) handlePromoteRequest(req promoteRequest) <-chan handlerEvent {
	done := make(chan handlerEvent, 1)

	go func(addr types.Address) {

	}(req.account)

	return done
}

// handleResetRequest is called during ResetWithHeader
// and aligns the pool's state for all accounts by pruning
// stale transactions.
func (p *TxPool) handleResetRequest(req resetRequest) <-chan handlerEvent {
	done := make(chan handlerEvent, 1)

	go func(newNonces map[types.Address]uint64) {

	}(req.newNonces)

	return done
}

// handleRollbackRequest handles an unfortunate transition write
// of a transaction during ExecuteTransactions and demotes
// any promoted transactions that were next-in-line for that account.
// Rollback means resetting the expected nonce for that account
// to a fallback value and re-issuing addRequests for the demoted
// transactions.
func (p *TxPool) handleRollbackRequest(req rollbackRequest) <-chan handlerEvent {
	done := make(chan handlerEvent, 1)

	go func(demoted transactions) {

	}(req.demoted)

	return done
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

// addTx validates and checks if the tx can be added
// to the pool and sends an addRequest, if successful
func (p *TxPool) addTx(tx *types.Transaction) error {
	return nil
}

// handleGossipTxn handles receiving gossiped transactions
func (p *TxPool) handleGossipTxn(obj interface{}) {
}

// validateTx validates that the transaction conforms to specific constraints to be added to the txpool
func (p *TxPool) validateTx(tx *types.Transaction) error {
	return nil
}

// Execute callback returning execution status for given transaction
type WriteTxCallback = func(*types.Transaction) WriteTxStatus

// ExecuteTransactions is called from within consensus when the node is
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
}

func (p *TxPool) lockAccount(addr types.Address, write bool) {
}

func (p *TxPool) unlockAccount(addr types.Address) {
}

func (p *TxPool) lockPromoted(write bool) {
}

func (p *TxPool) unlockPromoted() {
}

// slotsRequired() calculates the number of slotsRequired for given transaction
func slotsRequired(tx *types.Transaction) uint64 {
	return 0
}

// Status of a transaction in the pool
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
}

func (m *lookupMap) remove(hash types.Hash) {
}

func (m *lookupMap) load(hash types.Hash) *types.Transaction {
	return nil
}

// Map of expected nonces for all (known) accounts
// map[types.Address] -> nonce
type nonceMap struct {
	sync.Map
}

func (m *nonceMap) load(addr types.Address) (uint64, bool) {
	return 0, false
}

func (m *nonceMap) store(addr types.Address, nonce uint64) {
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
	return nil
}

func (m *accountLocks) store(addr types.Address, lock *accountLock) {
}

// Read/write protected min nonce queue (account)
type accountQueue struct {
	logger hclog.Logger
	txs    minNonceQueue
}

func newAccountQueue(logger hclog.Logger) *accountQueue {
	return nil
}

// promote moves all promotable transactions from the account queue
// to the promoted queue, if the first transaction has nonce equal expectedNonce
func (q *accountQueue) promote(expectedNonce uint64) transactions {
	return nil
}

// prune clears any enqueued transactions
// with nonce lower than currentNonce
func (q *accountQueue) prune(currentNonce uint64) transactions {
	return nil
}

func (q *accountQueue) push(tx *types.Transaction) {
}

func (q *accountQueue) pop() *types.Transaction {
	return nil
}

func (q *accountQueue) peek() *types.Transaction {
	return nil
}

func (q *accountQueue) length() int {
	return 0
}

// Read/write protected max price queue
type promotedQueue struct {
	sync.RWMutex
	txs    maxPriceQueue
	logger hclog.Logger
	wLock  uint32
}

func newPromotedQueue(logger hclog.Logger) *promotedQueue {
	return nil
}

func (q *promotedQueue) push(tx *types.Transaction) {
}

func (q *promotedQueue) peek() *types.Transaction {
	return nil
}

func (q *promotedQueue) pop() *types.Transaction {
	return nil
}

func (q *promotedQueue) length() int {
	return 0
}

// prune removes all stale transactions from the promoted queue
// for any account, as indicated by nonceMap
func (q *promotedQueue) prune(nonceMap map[types.Address]uint64) transactions {
	return nil
}

/* Queue implementations */

type transactions []*types.Transaction

// Queue of transactions sorted by min nonce
type minNonceQueue transactions

/* Methods required by heap interface */

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

// Queue of transactions sorted by max gasPrice
type maxPriceQueue transactions

/* Methods required by heap interface */

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
