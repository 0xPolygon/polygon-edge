package blockchain

import (
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/blockchain/storage/leveldb"
	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
)

// Blockchain is a blockchain reference
type Blockchain struct {
	logger hclog.Logger // The logger object

	db        storage.Storage // The Storage object (database)
	consensus Verifier
	executor  Executor

	config  *chain.Chain // Config containing chain information
	genesis types.Hash   // The hash of the genesis block

	headersCache    *lru.Cache // LRU cache for the headers
	difficultyCache *lru.Cache // LRU cache for the difficulty

	currentHeader     atomic.Value // The current header
	currentDifficulty atomic.Value // The current difficulty

	stream *eventStream // Event subscriptions

	// Average gas price (rolling average)
	averageGasPrice      *big.Int // The average gas price that gets queried
	averageGasPriceCount *big.Int // Param used in the avg. gas price calculation

	agpMux sync.Mutex // Mutex for the averageGasPrice calculation
}

type Verifier interface {
	VerifyHeader(parent, header *types.Header) error
	GetBlockCreator(header *types.Header) (types.Address, error)
}

type Executor interface {
	ProcessBlock(parentRoot types.Hash, block *types.Block, blockCreator types.Address) (*state.BlockResult, error)
}

// UpdateGasPriceAvg Updates the rolling average value of the gas price
func (b *Blockchain) UpdateGasPriceAvg(newValue *big.Int) {
	b.agpMux.Lock()

	b.averageGasPriceCount.Add(b.averageGasPriceCount, big.NewInt(1))

	differential := big.NewInt(0)
	differential.Div(newValue.Sub(newValue, b.averageGasPrice), b.averageGasPriceCount)

	b.averageGasPrice.Add(b.averageGasPrice, differential)

	b.agpMux.Unlock()
}

// GetAvgGasPrice returns the average gas price
func (b *Blockchain) GetAvgGasPrice() *big.Int {
	return b.averageGasPrice
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(
	logger hclog.Logger,
	dataDir string,
	config *chain.Chain,
	consensus Verifier,
	executor Executor,
) (*Blockchain, error) {

	b := &Blockchain{
		logger:    logger.Named("blockchain"),
		config:    config,
		consensus: consensus,
		executor:  executor,
		stream:    &eventStream{},
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

	b.headersCache, _ = lru.New(100)
	b.difficultyCache, _ = lru.New(100)

	// Push the initial event to the stream
	b.stream.push(&Event{})

	// Initialize the average gas price
	b.averageGasPrice = big.NewInt(0)
	b.averageGasPriceCount = big.NewInt(0)

	return b, nil
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
	return b.currentHeader.Load().(*types.Header)
}

// CurrentTD returns the current total difficulty (atomic)
func (b *Blockchain) CurrentTD() *big.Int {
	return b.currentDifficulty.Load().(*big.Int)
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
	return b.readDiff(hash)
}

// writeCanonicalHeader writes the new header
func (b *Blockchain) writeCanonicalHeader(event *Event, h *types.Header) error {
	td, ok := b.readDiff(h.ParentHash)
	if !ok {
		return fmt.Errorf("parent difficulty not found")
	}

	diff := big.NewInt(1).Add(td, new(big.Int).SetUint64(h.Difficulty))
	if err := b.db.WriteCanonicalHeader(h, diff); err != nil {
		return err
	}

	event.Type = EventHead
	event.AddNewHeader(h)
	event.SetDifficulty(diff)

	b.setCurrentHeader(h, diff)

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
	currentDiff := big.NewInt(0)
	if newHeader.ParentHash != types.StringToHash("") {
		td, ok := b.readDiff(newHeader.ParentHash)
		if !ok {
			return nil, fmt.Errorf("parent difficulty not found")
		}
		currentDiff = td
	}

	// Calculate the new difficulty
	diff := big.NewInt(1).Add(currentDiff, new(big.Int).SetUint64(newHeader.Difficulty))
	if err := b.db.WriteDiff(newHeader.Hash, diff); err != nil {
		return nil, err
	}

	// Update the blockchain reference
	b.setCurrentHeader(newHeader, diff)

	return diff, nil
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
		return h.(*types.Header), true
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

	return bb, true
}

// readDiff reads the latest difficulty using the hash
func (b *Blockchain) readDiff(headerHash types.Hash) (*big.Int, bool) {
	// Try to find the difficulty in the cache
	foundDifficulty, ok := b.difficultyCache.Get(headerHash)
	if ok {
		// Hit, return the difficulty
		return foundDifficulty.(*big.Int), true
	}

	// Miss, read the difficulty from the DB
	dbDifficulty, ok := b.db.ReadDiff(headerHash)
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
	size := len(headers)
	if size == 0 {
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

// WriteBlocks writes a batch of blocks
func (b *Blockchain) WriteBlocks(blocks []*types.Block) error {
	// Check the size
	size := len(blocks)
	if size == 0 {
		return fmt.Errorf("the passed in block array is empty")
	}

	// Log the information
	if size == 1 {
		b.logger.Info(
			"write block",
			"num",
			blocks[0].Number(),
			"parent",
			blocks[0].ParentHash(),
		)
	} else {
		b.logger.Info(
			"write blocks",
			"num",
			size,
			"from",
			blocks[0].Number(),
			"to",
			blocks[size-1].Number(),
			"parent",
			blocks[0].ParentHash())
	}

	parent, ok := b.readHeader(blocks[0].ParentHash())
	if !ok {
		return fmt.Errorf(
			"parent of %s (%d) not found: %s",
			blocks[0].Hash().String(),
			blocks[0].Number(),
			blocks[0].ParentHash(),
		)
	}

	if parent.Hash == types.ZeroHash {
		return fmt.Errorf("parent not found")
	}

	// Validate the chain
	for i, block := range blocks {
		// Check the parent numbers
		if block.Number()-1 != parent.Number {
			return fmt.Errorf(
				"number sequence not correct at %d, %d and %d",
				i,
				block.Number(),
				parent.Number,
			)
		}

		// Check the parent hash
		if block.ParentHash() != parent.Hash {
			return fmt.Errorf("parent hash not correct")
		}

		// Verify the header
		if err := b.consensus.VerifyHeader(parent, block.Header); err != nil {
			return fmt.Errorf("failed to verify the header: %v", err)
		}

		// Verify body data
		if hash := buildroot.CalculateUncleRoot(block.Uncles); hash != block.Header.Sha3Uncles {

			return fmt.Errorf(
				"uncle root hash mismatch: have %s, want %s",
				hash,
				block.Header.Sha3Uncles,
			)
		}

		// TODO, the wrapper around transactions
		if hash := buildroot.CalculateTransactionsRoot(block.Transactions); hash != block.Header.TxRoot {

			return fmt.Errorf(
				"transaction root hash mismatch: have %s, want %s",
				hash,
				block.Header.TxRoot,
			)
		}

		parent = block.Header
	}

	// Checks are passed, write the chain
	for indx, block := range blocks {
		header := block.Header

		// Process and validate the block
		res, err := b.processBlock(blocks[indx])
		if err != nil {
			return err
		}

		if err := b.writeBody(block); err != nil {
			return err
		}

		// Write the header to the chain
		evnt := &Event{}
		if err := b.writeHeaderImpl(evnt, header); err != nil {
			return err
		}

		// write the receipts, do it only after the header has been written.
		// Otherwise, a client might ask for a header once the receipt is valid
		// but before it is written into the storage
		if err := b.db.WriteReceipts(block.Hash(), res.Receipts); err != nil {
			return err
		}

		b.dispatchEvent(evnt)

		// Update the average gas price
		b.UpdateGasPriceAvg(new(big.Int).SetUint64(header.GasUsed))

		logArgs := []interface{}{
			"number", header.Number,
			"hash", header.Hash,
			"txns", len(block.Transactions),
		}
		if prevHeader, ok := b.GetHeaderByNumber(header.Number - 1); ok {
			diff := header.Timestamp - prevHeader.Timestamp
			logArgs = append(logArgs, "generation_time_in_sec", diff)
		}
		b.logger.Info("new block", logArgs...)
	}

	b.logger.Info("new head", "hash", b.Header().Hash, "number", b.Header().Number)

	return nil
}

// writeBody writes the block body to the DB.
// Additionally, it also updates the txn lookup, for txnHash -> block lookups
func (b *Blockchain) writeBody(block *types.Block) error {
	body := block.Body()

	// Write the full body (txns + receipts)
	if err := b.db.WriteBody(block.Header.Hash, body); err != nil {
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

// processBlock Processes the block, and does validation
func (b *Blockchain) processBlock(block *types.Block) (*state.BlockResult, error) {
	header := block.Header

	// Process the block
	parent, ok := b.readHeader(header.ParentHash)
	if !ok {
		return nil, fmt.Errorf("unknown ancestor")
	}

	blockCreator, err := b.consensus.GetBlockCreator(header)
	if err != nil {
		return nil, err
	}

	result, err := b.executor.ProcessBlock(parent.StateRoot, block, blockCreator)
	if err != nil {
		return nil, err
	}

	receipts := result.Receipts
	if len(receipts) != len(block.Transactions) {
		return nil, fmt.Errorf("bad size of receipts and transactions")
	}

	// Validate the fields
	if result.Root != header.StateRoot {
		return nil, fmt.Errorf("invalid merkle root")
	}

	if result.TotalGas != header.GasUsed {
		return nil, fmt.Errorf("gas used is different")
	}

	receiptSha := buildroot.CalculateReceiptsRoot(result.Receipts)
	if receiptSha != header.ReceiptsRoot {
		return nil, fmt.Errorf("invalid receipts root")
	}

	return result, nil
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

// WriteBlock writes a block of data
func (b *Blockchain) WriteBlock(block *types.Block) error {
	evnt := &Event{}
	if err := b.writeHeaderImpl(evnt, block.Header); err != nil {
		return err
	}

	b.dispatchEvent(evnt)

	return nil
}

// dispatchEvent pushes a new event to the stream
func (b *Blockchain) dispatchEvent(evnt *Event) {
	b.stream.push(evnt)
}

// writeHeaderImpl writes a block and the data, assumes the genesis is already set
func (b *Blockchain) writeHeaderImpl(evnt *Event, header *types.Header) error {
	head := b.Header()

	// Write the data
	if header.ParentHash == head.Hash {
		// Fast path to save the new canonical header
		return b.writeCanonicalHeader(evnt, header)
	}

	if err := b.db.WriteHeader(header); err != nil {
		return err
	}

	headerDiff, ok := b.readDiff(head.Hash)
	if !ok {
		panic("failed to get header difficulty")
	}

	parentDiff, ok := b.readDiff(header.ParentHash)
	if !ok {
		return fmt.Errorf(
			"parent of %s (%d) not found",
			header.Hash.String(),
			header.Number,
		)
	}

	// Write the difficulty
	if err := b.db.WriteDiff(
		header.Hash,
		big.NewInt(1).Add(
			parentDiff,
			new(big.Int).SetUint64(header.Difficulty),
		),
	); err != nil {
		return err
	}

	// Update the headers cache
	b.headersCache.Add(header.Hash, header)

	incomingDiff := big.NewInt(1).Add(parentDiff, new(big.Int).SetUint64(header.Difficulty))
	if incomingDiff.Cmp(headerDiff) > 0 {
		// new block has higher difficulty, reorg the chain
		if err := b.handleReorg(evnt, head, header); err != nil {
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
		if err == storage.ErrNotFound {
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
		return fmt.Errorf("failed to write the old header as fork: %v", err)
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

	if !full {
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
