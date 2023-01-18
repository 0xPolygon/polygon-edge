package blockchain

import (
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/0xPolygon/polygon-edge/blockchain/storage/leveldb"
	"github.com/0xPolygon/polygon-edge/blockchain/storage/memory"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/types/buildroot"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
)

const (
	BlockGasTargetDivisor uint64 = 1024 // The bound divisor of the gas limit, used in update calculations
	defaultCacheSize      int    = 100  // The default size for Blockchain LRU cache structures
)

var (
	ErrNoBlock              = errors.New("no block data passed in")
	ErrParentNotFound       = errors.New("parent block not found")
	ErrInvalidParentHash    = errors.New("parent block hash is invalid")
	ErrParentHashMismatch   = errors.New("invalid parent block hash")
	ErrInvalidBlockSequence = errors.New("invalid block sequence")
	ErrInvalidSha3Uncles    = errors.New("invalid block sha3 uncles root")
	ErrInvalidTxRoot        = errors.New("invalid block transactions root")
	ErrInvalidReceiptsSize  = errors.New("invalid number of receipts")
	ErrInvalidStateRoot     = errors.New("invalid block state root")
	ErrInvalidGasUsed       = errors.New("invalid block gas used")
	ErrInvalidReceiptsRoot  = errors.New("invalid block receipts root")
)

// Blockchain is a blockchain reference
type Blockchain struct {
	logger hclog.Logger // The logger object

	db        storage.Storage // The Storage object (database)
	consensus Verifier
	executor  Executor
	txSigner  TxSigner

	config  *chain.Chain // Config containing chain information
	genesis types.Hash   // The hash of the genesis block

	headersCache    *lru.Cache // LRU cache for the headers
	difficultyCache *lru.Cache // LRU cache for the difficulty

	// We need to keep track of block receipts between the verification phase
	// and the insertion phase of a new block coming in. To avoid having to
	// execute the transactions twice, we save the receipts from the initial execution
	// in a cache, so we can grab it later when inserting the block.
	// This is of course not an optimal solution - a better one would be to add
	// the receipts to the proposed block (like we do with Transactions and Uncles), but
	// that is currently not possible because it would break backwards compatibility due to
	// insane conditionals in the RLP unmarshal methods for the Block structure, which prevent
	// any new fields from being added
	receiptsCache *lru.Cache // LRU cache for the block receipts

	currentHeader     atomic.Value // The current header
	currentDifficulty atomic.Value // The current difficulty of the chain (total difficulty)

	stream *eventStream // Event subscriptions

	gpAverage *gasPriceAverage // A reference to the average gas price

	writeLock sync.Mutex
}

// gasPriceAverage keeps track of the average gas price (rolling average)
type gasPriceAverage struct {
	sync.RWMutex

	price *big.Int // The average gas price that gets queried
	count *big.Int // Param used in the avg. gas price calculation
}

type Verifier interface {
	VerifyHeader(header *types.Header) error
	ProcessHeaders(headers []*types.Header) error
	GetBlockCreator(header *types.Header) (types.Address, error)
	PreCommitState(header *types.Header, txn *state.Transition) error
}

type Executor interface {
	ProcessBlock(parentRoot types.Hash, block *types.Block, blockCreator types.Address) (*state.Transition, error)
}

type TxSigner interface {
	// Sender returns the sender of the transaction
	Sender(tx *types.Transaction) (types.Address, error)
}

type BlockResult struct {
	Root     types.Hash
	Receipts []*types.Receipt
	TotalGas uint64
}

// updateGasPriceAvg updates the rolling average value of the gas price
func (b *Blockchain) updateGasPriceAvg(newValues []*big.Int) {
	b.gpAverage.Lock()
	defer b.gpAverage.Unlock()

	// Sum the values for quick reference
	sum := big.NewInt(0)
	for _, val := range newValues {
		sum = sum.Add(sum, val)
	}

	// There is no previous average data,
	// so this new value set will instantiate it
	if b.gpAverage.count.Uint64() == 0 {
		b.calcArithmeticAverage(newValues, sum)

		return
	}

	// There is existing average data,
	// use it to generate a new average
	b.calcRollingAverage(newValues, sum)
}

// calcArithmeticAverage calculates and sets the arithmetic average
// of the passed in data set
func (b *Blockchain) calcArithmeticAverage(newValues []*big.Int, sum *big.Int) {
	newAverageCount := big.NewInt(int64(len(newValues)))
	newAverage := sum.Div(sum, newAverageCount)

	b.gpAverage.price = newAverage
	b.gpAverage.count = newAverageCount
}

// calcRollingAverage calculates the new average based on the
// moving average formula:
// new average = old average * (n-len(M))/n + (sum of values in M)/n)
// where n is the old average data count, and M is the new data set
func (b *Blockchain) calcRollingAverage(newValues []*big.Int, sum *big.Int) {
	var (
		// Save references to old counts
		oldCount   = b.gpAverage.count
		oldAverage = b.gpAverage.price

		inputSetCount = big.NewInt(0).SetInt64(int64(len(newValues)))
	)

	// old average * (n-len(M))/n
	newAverage := big.NewInt(0).Div(
		big.NewInt(0).Mul(
			oldAverage,
			big.NewInt(0).Sub(oldCount, inputSetCount),
		),
		oldCount,
	)

	// + (sum of values in M)/n
	newAverage.Add(
		newAverage,
		big.NewInt(0).Div(
			sum,
			oldCount,
		),
	)

	// Update the references
	b.gpAverage.price = newAverage
	b.gpAverage.count = inputSetCount.Add(inputSetCount, b.gpAverage.count)
}

// GetAvgGasPrice returns the average gas price for the chain
func (b *Blockchain) GetAvgGasPrice() *big.Int {
	b.gpAverage.RLock()
	defer b.gpAverage.RUnlock()

	return b.gpAverage.price
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(
	logger hclog.Logger,
	dataDir string,
	config *chain.Chain,
	consensus Verifier,
	executor Executor,
	txSigner TxSigner,
) (*Blockchain, error) {
	b := &Blockchain{
		logger:    logger.Named("blockchain"),
		config:    config,
		consensus: consensus,
		executor:  executor,
		txSigner:  txSigner,
		stream:    &eventStream{},
		gpAverage: &gasPriceAverage{
			price: big.NewInt(0),
			count: big.NewInt(0),
		},
	}

	var (
		db  storage.Storage
		err error
	)

	if dataDir == "" {
		if db, err = memory.NewMemoryStorage(nil); err != nil {
			return nil, err
		}
	} else {
		if db, err = leveldb.NewLevelDBStorage(
			filepath.Join(dataDir, "blockchain"),
			logger,
		); err != nil {
			return nil, err
		}
	}

	b.db = db

	if err := b.initCaches(defaultCacheSize); err != nil {
		return nil, err
	}

	// Push the initial event to the stream
	b.stream.push(&Event{})

	return b, nil
}

// initCaches initializes the blockchain caches with the specified size
func (b *Blockchain) initCaches(size int) error {
	var err error

	b.headersCache, err = lru.New(size)
	if err != nil {
		return fmt.Errorf("unable to create headers cache, %w", err)
	}

	b.difficultyCache, err = lru.New(size)
	if err != nil {
		return fmt.Errorf("unable to create difficulty cache, %w", err)
	}

	b.receiptsCache, err = lru.New(size)
	if err != nil {
		return fmt.Errorf("unable to create receipts cache, %w", err)
	}

	return nil
}

// ComputeGenesis computes the genesis hash, and updates the blockchain reference
func (b *Blockchain) ComputeGenesis() error {
	// try to write the genesis block
	head, ok := b.db.ReadHeadHash()

	if ok {
		// initialized storage
		b.genesis, ok = b.db.ReadCanonicalHash(0)
		if !ok {
			return fmt.Errorf("failed to load genesis hash")
		}

		// validate that the genesis file in storage matches the chain.Genesis
		if b.genesis != b.config.Genesis.Hash() {
			return fmt.Errorf("genesis file does not match current genesis")
		}

		header, ok := b.GetHeaderByHash(head)
		if !ok {
			return fmt.Errorf("failed to get header with hash %s", head.String())
		}

		diff, ok := b.GetTD(head)
		if !ok {
			return fmt.Errorf("failed to read difficulty")
		}

		b.logger.Info(
			"Current header",
			"hash",
			header.Hash.String(),
			"number",
			header.Number,
		)

		b.setCurrentHeader(header, diff)
	} else {
		// empty storage, write the genesis
		if err := b.writeGenesis(b.config.Genesis); err != nil {
			return err
		}
	}

	b.logger.Info("genesis", "hash", b.config.Genesis.Hash())

	return nil
}

func (b *Blockchain) GetConsensus() Verifier {
	return b.consensus
}

// SetConsensus sets the consensus
func (b *Blockchain) SetConsensus(c Verifier) {
	b.consensus = c
}

// setCurrentHeader sets the current header
func (b *Blockchain) setCurrentHeader(h *types.Header, diff *big.Int) {
	// Update the header (atomic)
	header := h.Copy()
	b.currentHeader.Store(header)

	// Update the difficulty (atomic)
	difficulty := new(big.Int).Set(diff)
	b.currentDifficulty.Store(difficulty)
}

// Header returns the current header (atomic)
func (b *Blockchain) Header() *types.Header {
	header, ok := b.currentHeader.Load().(*types.Header)
	if !ok {
		return nil
	}

	return header
}

// CurrentTD returns the current total difficulty (atomic)
func (b *Blockchain) CurrentTD() *big.Int {
	td, ok := b.currentDifficulty.Load().(*big.Int)
	if !ok {
		return nil
	}

	return td
}

// Config returns the blockchain configuration
func (b *Blockchain) Config() *chain.Params {
	return b.config.Params
}

// GetHeader returns the block header using the hash
func (b *Blockchain) GetHeader(hash types.Hash, number uint64) (*types.Header, bool) {
	return b.GetHeaderByHash(hash)
}

// GetBlock returns the block using the hash
func (b *Blockchain) GetBlock(hash types.Hash, number uint64, full bool) (*types.Block, bool) {
	return b.GetBlockByHash(hash, full)
}

// GetParent returns the parent header
func (b *Blockchain) GetParent(header *types.Header) (*types.Header, bool) {
	return b.readHeader(header.ParentHash)
}

// Genesis returns the genesis block
func (b *Blockchain) Genesis() types.Hash {
	return b.genesis
}

// CalculateGasLimit returns the gas limit of the next block after parent
func (b *Blockchain) CalculateGasLimit(number uint64) (uint64, error) {
	parent, ok := b.GetHeaderByNumber(number - 1)
	if !ok {
		return 0, fmt.Errorf("parent of block %d not found", number)
	}

	return b.calculateGasLimit(parent.GasLimit), nil
}

// calculateGasLimit calculates gas limit in reference to the block gas target
func (b *Blockchain) calculateGasLimit(parentGasLimit uint64) uint64 {
	// The gas limit cannot move more than 1/1024 * parentGasLimit
	// in either direction per block
	blockGasTarget := b.Config().BlockGasTarget

	// Check if the gas limit target has been set
	if blockGasTarget == 0 {
		// The gas limit target has not been set,
		// so it should use the parent gas limit
		return parentGasLimit
	}

	// Check if the gas limit is already at the target
	if parentGasLimit == blockGasTarget {
		// The gas limit is already at the target, no need to move it
		return blockGasTarget
	}

	delta := parentGasLimit * 1 / BlockGasTargetDivisor
	if parentGasLimit < blockGasTarget {
		// The gas limit is lower than the gas target, so it should
		// increase towards the target
		return common.Min(blockGasTarget, parentGasLimit+delta)
	}

	// The gas limit is higher than the gas target, so it should
	// decrease towards the target
	return common.Max(blockGasTarget, common.Max(parentGasLimit-delta, 0))
}

// writeGenesis wrapper for the genesis write function
func (b *Blockchain) writeGenesis(genesis *chain.Genesis) error {
	header := genesis.GenesisHeader()
	header.ComputeHash()

	if err := b.writeGenesisImpl(header); err != nil {
		return err
	}

	return nil
}

// writeGenesisImpl writes the genesis file to the DB + blockchain reference
func (b *Blockchain) writeGenesisImpl(header *types.Header) error {
	// Update the reference
	b.genesis = header.Hash

	// Update the DB
	if err := b.db.WriteHeader(header); err != nil {
		return err
	}

	// Advance the head
	if _, err := b.advanceHead(header); err != nil {
		return err
	}

	// Create an event and send it to the stream
	event := &Event{}
	event.AddNewHeader(header)
	b.stream.push(event)

	return nil
}

// Empty checks if the blockchain is empty
func (b *Blockchain) Empty() bool {
	_, ok := b.db.ReadHeadHash()

	return !ok
}

// GetChainTD returns the latest difficulty
func (b *Blockchain) GetChainTD() (*big.Int, bool) {
	header := b.Header()

	return b.GetTD(header.Hash)
}

// GetTD returns the difficulty for the header hash
func (b *Blockchain) GetTD(hash types.Hash) (*big.Int, bool) {
	return b.readTotalDifficulty(hash)
}

// writeCanonicalHeader writes the new header
func (b *Blockchain) writeCanonicalHeader(event *Event, h *types.Header) error {
	parentTD, ok := b.readTotalDifficulty(h.ParentHash)
	if !ok {
		return fmt.Errorf("parent difficulty not found")
	}

	newTD := big.NewInt(0).Add(parentTD, new(big.Int).SetUint64(h.Difficulty))
	if err := b.db.WriteCanonicalHeader(h, newTD); err != nil {
		return err
	}

	event.Type = EventHead
	event.AddNewHeader(h)
	event.SetDifficulty(newTD)

	b.setCurrentHeader(h, newTD)

	return nil
}

// advanceHead Sets the passed in header as the new head of the chain
func (b *Blockchain) advanceHead(newHeader *types.Header) (*big.Int, error) {
	// Write the current head hash into storage
	if err := b.db.WriteHeadHash(newHeader.Hash); err != nil {
		return nil, err
	}

	// Write the current head number into storage
	if err := b.db.WriteHeadNumber(newHeader.Number); err != nil {
		return nil, err
	}

	// Matches the current head number with the current hash
	if err := b.db.WriteCanonicalHash(newHeader.Number, newHeader.Hash); err != nil {
		return nil, err
	}

	// Check if there was a parent difficulty
	parentTD := big.NewInt(0)

	if newHeader.ParentHash != types.StringToHash("") {
		td, ok := b.readTotalDifficulty(newHeader.ParentHash)
		if !ok {
			return nil, fmt.Errorf("parent difficulty not found")
		}

		parentTD = td
	}

	// Calculate the new total difficulty
	newTD := big.NewInt(0).Add(parentTD, big.NewInt(0).SetUint64(newHeader.Difficulty))
	if err := b.db.WriteTotalDifficulty(newHeader.Hash, newTD); err != nil {
		return nil, err
	}

	// Update the blockchain reference
	b.setCurrentHeader(newHeader, newTD)

	return newTD, nil
}

// GetReceiptsByHash returns the receipts by their hash
func (b *Blockchain) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return b.db.ReadReceipts(hash)
}

// GetBodyByHash returns the body by their hash
func (b *Blockchain) GetBodyByHash(hash types.Hash) (*types.Body, bool) {
	return b.readBody(hash)
}

// GetHeaderByHash returns the header by his hash
func (b *Blockchain) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return b.readHeader(hash)
}

// readHeader Returns the header using the hash
func (b *Blockchain) readHeader(hash types.Hash) (*types.Header, bool) {
	// Try to find a hit in the headers cache
	h, ok := b.headersCache.Get(hash)
	if ok {
		// Hit, return the3 header
		header, ok := h.(*types.Header)
		if !ok {
			return nil, false
		}

		return header, true
	}

	// Cache miss, load it from the DB
	hh, err := b.db.ReadHeader(hash)
	if err != nil {
		return nil, false
	}

	// Compute the header hash and update the cache
	hh.ComputeHash()
	b.headersCache.Add(hash, hh)

	return hh, true
}

// readBody reads the block's body, using the block hash
func (b *Blockchain) readBody(hash types.Hash) (*types.Body, bool) {
	bb, err := b.db.ReadBody(hash)
	if err != nil {
		b.logger.Error("failed to read body", "err", err)

		return nil, false
	}

	// To return from field in the transactions of the past blocks
	if updated := b.recoverFromFieldsInTransactions(bb.Transactions); updated {
		if err := b.db.WriteBody(hash, bb); err != nil {
			b.logger.Warn("failed to write body into storage", "hash", hash, "err", err)
		}
	}

	return bb, true
}

// readTotalDifficulty reads the total difficulty associated with the hash
func (b *Blockchain) readTotalDifficulty(headerHash types.Hash) (*big.Int, bool) {
	// Try to find the difficulty in the cache
	foundDifficulty, ok := b.difficultyCache.Get(headerHash)
	if ok {
		// Hit, return the difficulty
		fd, ok := foundDifficulty.(*big.Int)
		if !ok {
			return nil, false
		}

		return fd, true
	}

	// Miss, read the difficulty from the DB
	dbDifficulty, ok := b.db.ReadTotalDifficulty(headerHash)
	if !ok {
		return nil, false
	}

	// Update the difficulty cache
	b.difficultyCache.Add(headerHash, dbDifficulty)

	return dbDifficulty, true
}

// GetHeaderByNumber returns the header using the block number
func (b *Blockchain) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	hash, ok := b.db.ReadCanonicalHash(n)
	if !ok {
		return nil, false
	}

	h, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}

	return h, true
}

// WriteHeaders writes an array of headers
func (b *Blockchain) WriteHeaders(headers []*types.Header) error {
	return b.WriteHeadersWithBodies(headers)
}

// WriteHeadersWithBodies writes a batch of headers
func (b *Blockchain) WriteHeadersWithBodies(headers []*types.Header) error {
	// Check the size
	if len(headers) == 0 {
		return fmt.Errorf("passed in headers array is empty")
	}

	// Validate the chain
	for i := 1; i < len(headers); i++ {
		// Check the sequence
		if headers[i].Number-1 != headers[i-1].Number {
			return fmt.Errorf(
				"number sequence not correct at %d, %d and %d",
				i,
				headers[i].Number,
				headers[i-1].Number,
			)
		}

		// Check if the parent hashes match
		if headers[i].ParentHash != headers[i-1].Hash {
			return fmt.Errorf("parent hash not correct")
		}
	}

	// Write the actual headers
	for _, h := range headers {
		event := &Event{}
		if err := b.writeHeaderImpl(event, h); err != nil {
			return err
		}

		// Notify the event stream
		b.dispatchEvent(event)
	}

	return nil
}

// VerifyPotentialBlock does the minimal block verification without consulting the
// consensus layer. Should only be used if consensus checks are done
// outside the method call
func (b *Blockchain) VerifyPotentialBlock(block *types.Block) error {
	// Do just the initial block verification
	return b.verifyBlock(block)
}

// VerifyFinalizedBlock verifies that the block is valid by performing a series of checks.
// It is assumed that the block status is sealed (committed)
func (b *Blockchain) VerifyFinalizedBlock(block *types.Block) error {
	// Make sure the consensus layer verifies this block header
	if err := b.consensus.VerifyHeader(block.Header); err != nil {
		return fmt.Errorf("failed to verify the header: %w", err)
	}

	// Do the initial block verification
	if err := b.verifyBlock(block); err != nil {
		return err
	}

	return nil
}

// verifyBlock does the base (common) block verification steps by
// verifying the block body as well as the parent information
func (b *Blockchain) verifyBlock(block *types.Block) error {
	// Make sure the block is present
	if block == nil {
		return ErrNoBlock
	}

	// Make sure the block is in line with the parent block
	if err := b.verifyBlockParent(block); err != nil {
		return err
	}

	// Make sure the block body data is valid
	if err := b.verifyBlockBody(block); err != nil {
		return err
	}

	return nil
}

// verifyBlockParent makes sure that the child block is in line
// with the locally saved parent block. This means checking:
// - The parent exists
// - The hashes match up
// - The block numbers match up
// - The block gas limit / used matches up
func (b *Blockchain) verifyBlockParent(childBlock *types.Block) error {
	// Grab the parent block
	parentHash := childBlock.ParentHash()
	parent, ok := b.readHeader(parentHash)

	if !ok {
		b.logger.Error(fmt.Sprintf(
			"parent of %s (%d) not found: %s",
			childBlock.Hash().String(),
			childBlock.Number(),
			parentHash,
		))

		return ErrParentNotFound
	}

	// Make sure the hash is valid
	if parent.Hash == types.ZeroHash {
		return ErrInvalidParentHash
	}

	// Make sure the hashes match up
	if parentHash != parent.Hash {
		return ErrParentHashMismatch
	}

	// Make sure the block numbers are correct
	if childBlock.Number()-1 != parent.Number {
		b.logger.Error(fmt.Sprintf(
			"number sequence not correct at %d and %d",
			childBlock.Number(),
			parent.Number,
		))

		return ErrInvalidBlockSequence
	}

	// Make sure the gas limit is within correct bounds
	if gasLimitErr := b.verifyGasLimit(childBlock.Header, parent); gasLimitErr != nil {
		return fmt.Errorf("invalid gas limit, %w", gasLimitErr)
	}

	return nil
}

// verifyBlockBody verifies that the block body is valid. This means checking:
// - The trie roots match up (state, transactions, receipts, uncles)
// - The receipts match up
// - The execution result matches up
func (b *Blockchain) verifyBlockBody(block *types.Block) error {
	// Make sure the Uncles root matches up
	if hash := buildroot.CalculateUncleRoot(block.Uncles); hash != block.Header.Sha3Uncles {
		b.logger.Error(fmt.Sprintf(
			"uncle root hash mismatch: have %s, want %s",
			hash,
			block.Header.Sha3Uncles,
		))

		return ErrInvalidSha3Uncles
	}

	// Make sure the transactions root matches up
	if hash := buildroot.CalculateTransactionsRoot(block.Transactions); hash != block.Header.TxRoot {
		b.logger.Error(fmt.Sprintf(
			"transaction root hash mismatch: have %s, want %s",
			hash,
			block.Header.TxRoot,
		))

		return ErrInvalidTxRoot
	}

	// Execute the transactions in the block and grab the result
	blockResult, executeErr := b.executeBlockTransactions(block)
	if executeErr != nil {
		return fmt.Errorf("unable to execute block transactions, %w", executeErr)
	}

	// Verify the local execution result with the proposed block data
	if err := blockResult.verifyBlockResult(block); err != nil {
		return fmt.Errorf("unable to verify block execution result, %w", err)
	}

	return nil
}

// verifyBlockResult verifies that the block transaction execution result
// matches up to the expected values
func (br *BlockResult) verifyBlockResult(referenceBlock *types.Block) error {
	// Make sure the number of receipts matches the number of transactions
	if len(br.Receipts) != len(referenceBlock.Transactions) {
		return ErrInvalidReceiptsSize
	}

	// Make sure the world state root matches up
	if br.Root != referenceBlock.Header.StateRoot {
		return ErrInvalidStateRoot
	}

	// Make sure the gas used is valid
	if br.TotalGas != referenceBlock.Header.GasUsed {
		return ErrInvalidGasUsed
	}

	// Make sure the receipts root matches up
	receiptsRoot := buildroot.CalculateReceiptsRoot(br.Receipts)
	if receiptsRoot != referenceBlock.Header.ReceiptsRoot {
		return ErrInvalidReceiptsRoot
	}

	return nil
}

// executeBlockTransactions executes the transactions in the block locally,
// and reports back the block execution result
func (b *Blockchain) executeBlockTransactions(block *types.Block) (*BlockResult, error) {
	header := block.Header

	parent, ok := b.readHeader(header.ParentHash)
	if !ok {
		return nil, ErrParentNotFound
	}

	blockCreator, err := b.consensus.GetBlockCreator(header)
	if err != nil {
		return nil, err
	}

	txn, err := b.executor.ProcessBlock(parent.StateRoot, block, blockCreator)
	if err != nil {
		return nil, err
	}

	if err := b.consensus.PreCommitState(header, txn); err != nil {
		return nil, err
	}

	_, root := txn.Commit()

	// Append the receipts to the receipts cache
	b.receiptsCache.Add(header.Hash, txn.Receipts())

	return &BlockResult{
		Root:     root,
		Receipts: txn.Receipts(),
		TotalGas: txn.TotalGas(),
	}, nil
}

// WriteBlock writes a single block to the local blockchain.
// It doesn't do any kind of verification, only commits the block to the DB
func (b *Blockchain) WriteBlock(block *types.Block, source string) error {
	b.writeLock.Lock()
	defer b.writeLock.Unlock()

	if block.Number() <= b.Header().Number {
		b.logger.Info("block already inserted", "block", block.Number(), "source", source)

		return nil
	}

	header := block.Header

	if err := b.writeBody(block); err != nil {
		return err
	}

	// Write the header to the chain
	evnt := &Event{Source: source}
	if err := b.writeHeaderImpl(evnt, header); err != nil {
		return err
	}

	// Fetch the block receipts
	blockReceipts, receiptsErr := b.extractBlockReceipts(block)
	if receiptsErr != nil {
		return receiptsErr
	}

	// write the receipts, do it only after the header has been written.
	// Otherwise, a client might ask for a header once the receipt is valid,
	// but before it is written into the storage
	if err := b.db.WriteReceipts(block.Hash(), blockReceipts); err != nil {
		return err
	}

	// update snapshot
	if err := b.consensus.ProcessHeaders([]*types.Header{header}); err != nil {
		return err
	}

	b.dispatchEvent(evnt)

	// Update the average gas price
	b.updateGasPriceAvgWithBlock(block)

	logArgs := []interface{}{
		"number", header.Number,
		"txs", len(block.Transactions),
		"hash", header.Hash,
		"parent", header.ParentHash,
	}

	if prevHeader, ok := b.GetHeaderByNumber(header.Number - 1); ok {
		diff := header.Timestamp - prevHeader.Timestamp
		logArgs = append(logArgs, "generation_time_in_seconds", diff)
	}

	b.logger.Info("new block", logArgs...)

	return nil
}

// extractBlockReceipts extracts the receipts from the passed in block
func (b *Blockchain) extractBlockReceipts(block *types.Block) ([]*types.Receipt, error) {
	// Check the cache for the block receipts
	receipts, ok := b.receiptsCache.Get(block.Header.Hash)
	if !ok {
		// No receipts found in the cache, execute the transactions from the block
		// and fetch them
		blockResult, err := b.executeBlockTransactions(block)
		if err != nil {
			return nil, err
		}

		return blockResult.Receipts, nil
	}

	extractedReceipts, ok := receipts.([]*types.Receipt)
	if !ok {
		return nil, errors.New("invalid type assertion for receipts")
	}

	return extractedReceipts, nil
}

// updateGasPriceAvgWithBlock extracts the gas price information from the
// block, and updates the average gas price for the chain accordingly
func (b *Blockchain) updateGasPriceAvgWithBlock(block *types.Block) {
	if len(block.Transactions) < 1 {
		// No transactions in the block,
		// so no gas price average to update
		return
	}

	gasPrices := make([]*big.Int, len(block.Transactions))
	for i, transaction := range block.Transactions {
		gasPrices[i] = transaction.GasPrice
	}

	b.updateGasPriceAvg(gasPrices)
}

// writeBody writes the block body to the DB.
// Additionally, it also updates the txn lookup, for txnHash -> block lookups
func (b *Blockchain) writeBody(block *types.Block) error {
	// Recover 'from' field in tx before saving
	// Because the block passed from the consensus layer doesn't have from field in tx,
	// due to missing encoding in RLP
	if err := b.recoverFromFieldsInBlock(block); err != nil {
		return err
	}

	// Write the full body (txns + receipts)
	if err := b.db.WriteBody(block.Header.Hash, block.Body()); err != nil {
		return err
	}

	// Write txn lookups (txHash -> block)
	for _, txn := range block.Transactions {
		if err := b.db.WriteTxLookup(txn.Hash, block.Hash()); err != nil {
			return err
		}
	}

	return nil
}

// ReadTxLookup returns the block hash using the transaction hash
func (b *Blockchain) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	v, ok := b.db.ReadTxLookup(hash)

	return v, ok
}

// recoverFromFieldsInBlock recovers 'from' fields in the transactions of the given block
// return error if the invalid signature found
func (b *Blockchain) recoverFromFieldsInBlock(block *types.Block) error {
	for _, tx := range block.Transactions {
		if tx.From != types.ZeroAddress {
			continue
		}

		sender, err := b.txSigner.Sender(tx)
		if err != nil {
			return err
		}

		tx.From = sender
	}

	return nil
}

// recoverFromFieldsInTransactions recovers 'from' fields in the transactions
// log as warning if failing to recover one address
func (b *Blockchain) recoverFromFieldsInTransactions(transactions []*types.Transaction) bool {
	updated := false

	for _, tx := range transactions {
		if tx.From != types.ZeroAddress {
			continue
		}

		sender, err := b.txSigner.Sender(tx)
		if err != nil {
			b.logger.Warn("failed to recover from address in Tx", "hash", tx.Hash, "err", err)

			continue
		}

		tx.From = sender
		updated = true
	}

	return updated
}

// verifyGasLimit is a helper function for validating a gas limit in a header
func (b *Blockchain) verifyGasLimit(header *types.Header, parentHeader *types.Header) error {
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf(
			"block gas used exceeds gas limit, limit = %d, used=%d",
			header.GasLimit,
			header.GasUsed,
		)
	}

	// Skip block limit difference check for genesis
	if header.Number == 0 {
		return nil
	}

	// Find the absolute delta between the limits
	diff := int64(parentHeader.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}

	limit := parentHeader.GasLimit / BlockGasTargetDivisor
	if uint64(diff) > limit {
		return fmt.Errorf(
			"invalid gas limit, limit = %d, want %d +- %d",
			header.GasLimit,
			parentHeader.GasLimit,
			limit-1,
		)
	}

	return nil
}

// GetHashHelper is used by the EVM, so that the SC can get the hash of the header number
func (b *Blockchain) GetHashHelper(header *types.Header) func(i uint64) (res types.Hash) {
	return func(i uint64) (res types.Hash) {
		num, hash := header.Number-1, header.ParentHash

		for {
			if num == i {
				res = hash

				return
			}

			h, ok := b.GetHeaderByHash(hash)
			if !ok {
				return
			}

			hash = h.ParentHash

			if num == 0 {
				return
			}

			num--
		}
	}
}

// GetHashByNumber returns the block hash using the block number
func (b *Blockchain) GetHashByNumber(blockNumber uint64) types.Hash {
	block, ok := b.GetBlockByNumber(blockNumber, false)
	if !ok {
		return types.Hash{}
	}

	return block.Hash()
}

// dispatchEvent pushes a new event to the stream
func (b *Blockchain) dispatchEvent(evnt *Event) {
	b.stream.push(evnt)
}

// writeHeaderImpl writes a block and the data, assumes the genesis is already set
func (b *Blockchain) writeHeaderImpl(evnt *Event, header *types.Header) error {
	currentHeader := b.Header()

	// Write the data
	if header.ParentHash == currentHeader.Hash {
		// Fast path to save the new canonical header
		return b.writeCanonicalHeader(evnt, header)
	}

	if err := b.db.WriteHeader(header); err != nil {
		return err
	}

	currentTD, ok := b.readTotalDifficulty(currentHeader.Hash)
	if !ok {
		panic("failed to get header difficulty")
	}

	// parent total difficulty of incoming header
	parentTD, ok := b.readTotalDifficulty(header.ParentHash)
	if !ok {
		return fmt.Errorf(
			"parent of %s (%d) not found",
			header.Hash.String(),
			header.Number,
		)
	}

	// Write the difficulty
	if err := b.db.WriteTotalDifficulty(
		header.Hash,
		big.NewInt(0).Add(
			parentTD,
			big.NewInt(0).SetUint64(header.Difficulty),
		),
	); err != nil {
		return err
	}

	// Update the headers cache
	b.headersCache.Add(header.Hash, header)

	incomingTD := big.NewInt(0).Add(parentTD, big.NewInt(0).SetUint64(header.Difficulty))
	if incomingTD.Cmp(currentTD) > 0 {
		// new block has higher difficulty, reorg the chain
		if err := b.handleReorg(evnt, currentHeader, header); err != nil {
			return err
		}
	} else {
		// new block has lower difficulty, create a new fork
		evnt.AddOldHeader(header)
		evnt.Type = EventFork

		if err := b.writeFork(header); err != nil {
			return err
		}
	}

	return nil
}

// writeFork writes the new header forks to the DB
func (b *Blockchain) writeFork(header *types.Header) error {
	forks, err := b.db.ReadForks()
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			forks = []types.Hash{}
		} else {
			return err
		}
	}

	newForks := []types.Hash{}

	for _, fork := range forks {
		if fork != header.ParentHash {
			newForks = append(newForks, fork)
		}
	}

	newForks = append(newForks, header.Hash)
	if err := b.db.WriteForks(newForks); err != nil {
		return err
	}

	return nil
}

// handleReorg handles a reorganization event
func (b *Blockchain) handleReorg(
	evnt *Event,
	oldHeader *types.Header,
	newHeader *types.Header,
) error {
	newChainHead := newHeader
	oldChainHead := oldHeader

	oldChain := []*types.Header{}
	newChain := []*types.Header{}

	var ok bool

	// Fill up the old headers array
	for oldHeader.Number > newHeader.Number {
		oldHeader, ok = b.readHeader(oldHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", oldHeader.ParentHash.String())
		}

		oldChain = append(oldChain, oldHeader)
	}

	// Fill up the new headers array
	for newHeader.Number > oldHeader.Number {
		newHeader, ok = b.readHeader(newHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", newHeader.ParentHash.String())
		}

		newChain = append(newChain, newHeader)
	}

	for oldHeader.Hash != newHeader.Hash {
		oldHeader, ok = b.readHeader(oldHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", oldHeader.ParentHash.String())
		}

		newHeader, ok = b.readHeader(newHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", newHeader.ParentHash.String())
		}

		oldChain = append(oldChain, oldHeader)
	}

	for _, b := range oldChain[:len(oldChain)-1] {
		evnt.AddOldHeader(b)
	}

	evnt.AddOldHeader(oldChainHead)
	evnt.AddNewHeader(newChainHead)

	for _, b := range newChain {
		evnt.AddNewHeader(b)
	}

	if err := b.writeFork(oldChainHead); err != nil {
		return fmt.Errorf("failed to write the old header as fork: %w", err)
	}

	// Update canonical chain numbers
	for _, h := range newChain {
		if err := b.db.WriteCanonicalHash(h.Number, h.Hash); err != nil {
			return err
		}
	}

	diff, err := b.advanceHead(newChainHead)
	if err != nil {
		return err
	}

	// Set the event type and difficulty
	evnt.Type = EventReorg
	evnt.SetDifficulty(diff)

	return nil
}

// GetForks returns the forks
func (b *Blockchain) GetForks() ([]types.Hash, error) {
	return b.db.ReadForks()
}

// GetBlockByHash returns the block using the block hash
func (b *Blockchain) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	header, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}

	block := &types.Block{
		Header: header,
	}

	if !full || header.Number == 0 {
		return block, true
	}

	// Load the entire block body
	body, ok := b.readBody(hash)
	if !ok {
		return block, false
	}

	// Set the transactions and uncles
	block.Transactions = body.Transactions
	block.Uncles = body.Uncles

	return block, true
}

// GetBlockByNumber returns the block using the block number
func (b *Blockchain) GetBlockByNumber(blockNumber uint64, full bool) (*types.Block, bool) {
	blockHash, ok := b.db.ReadCanonicalHash(blockNumber)
	if !ok {
		return nil, false
	}

	// if blockNumber 0 (genesis block), do not try and get the full block
	if blockNumber == uint64(0) {
		full = false
	}

	return b.GetBlockByHash(blockHash, full)
}

// Close closes the DB connection
func (b *Blockchain) Close() error {
	return b.db.Close()
}
