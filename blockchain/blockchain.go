package blockchain

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"

	mapset "github.com/deckarep/golang-set"
	lru "github.com/hashicorp/golang-lru"
)

// TODO, fix testing with genesis
// TODO, cleanup block transition function
// TODO, add cache to uncles computing
// TODO, after header chain we can figure out a better semantic relation
// about how the blocks are processed.

var (
	errDuplicateUncle  = errors.New("duplicate uncle")
	errUncleIsAncestor = errors.New("uncle is ancestor")
	errDanglingUncle   = errors.New("uncle's parent is not ancestor")
)

// Blockchain is a blockchain reference
type Blockchain struct {
	db        storage.Storage
	consensus consensus.Consensus
	executor  *state.Executor

	genesis types.Hash

	// TODO: Unify in a single event
	sidechainCh chan *types.Header
	listeners   []chan *types.Header

	headersCache    *lru.Cache
	bodiesCache     *lru.Cache
	difficultyCache *lru.Cache

	config *chain.Params
}

var ripemd = types.StringToAddress("0000000000000000000000000000000000000003")

var ripemdFailedTxn = types.StringToHash("0xcf416c536ec1a19ed1fb89e4ec7ffb3cf73aa413b3aa9b77d60e4fd81a4296ba")

// NewBlockchain creates a new blockchain object
func NewBlockchain(db storage.Storage, config *chain.Params, consensus consensus.Consensus, executor *state.Executor) *Blockchain {
	b := &Blockchain{
		db:          db,
		config:      config,
		consensus:   consensus,
		sidechainCh: make(chan *types.Header, 10),
		listeners:   []chan *types.Header{},
		executor:    executor,
	}

	b.headersCache, _ = lru.New(100)
	b.bodiesCache, _ = lru.New(100)
	b.difficultyCache, _ = lru.New(100)
	return b
}

func (b *Blockchain) Config() *chain.Params {
	return b.config
}

func (b *Blockchain) CurrentHeader() (*types.Header, bool) {
	return b.Header()
}

func (b *Blockchain) GetHeader(hash types.Hash, number uint64) (*types.Header, bool) {
	return b.GetHeaderByHash(hash)
}

func (b *Blockchain) GetBlock(hash types.Hash, number uint64, full bool) (*types.Block, bool) {
	return b.GetBlockByHash(hash, full)
}

func (b *Blockchain) ReadTransactionBlockHash(hash types.Hash) (types.Hash, bool) {
	// TODO
	return types.Hash{}, false
}

func (b *Blockchain) GetConsensus() consensus.Consensus {
	return b.consensus
}

func (b *Blockchain) Executor() *state.Executor {
	return b.executor
}

func (b *Blockchain) Subscribe() chan *types.Header {
	ch := make(chan *types.Header, 5)
	b.listeners = append(b.listeners, ch)
	return ch
}

// GetParent return the parent
func (b *Blockchain) GetParent(header *types.Header) (*types.Header, bool) {
	return b.readHeader(header.ParentHash)
}

// Genesis returns the genesis block
func (b *Blockchain) Genesis() types.Hash {
	return b.genesis
}

// WriteGenesis writes the genesis block if not present
func (b *Blockchain) WriteGenesis(genesis *chain.Genesis) error {
	_, ok := b.db.ReadHeadHash()
	if ok {
		b.genesis, ok = b.db.ReadCanonicalHash(0)
		if !ok {
			return fmt.Errorf("failed to load genesis hash")
		}
		return nil
	}

	root := b.executor.WriteGenesis(genesis.Alloc)

	header := genesis.ToBlock()
	header.StateRoot = root
	header.ComputeHash()

	b.genesis = header.Hash

	// add genesis block
	if err := b.addHeader(header); err != nil {
		return err
	}
	if err := b.advanceHead(header); err != nil {
		return err
	}

	diff := new(big.Int).SetUint64(header.Difficulty)
	if err := b.db.WriteDiff(header.Hash, diff); err != nil {
		return err
	}
	b.difficultyCache.Add(header.Hash, diff)

	return nil
}

// WriteHeaderGenesis writes the genesis without any state allocation
// TODO, remove
func (b *Blockchain) WriteHeaderGenesis(header *types.Header) error {
	// The chain is not empty
	if !b.Empty() {
		return nil
	}

	// add genesis block
	if err := b.addHeader(header); err != nil {
		return err
	}
	if err := b.advanceHead(header); err != nil {
		return err
	}

	b.genesis = header.Hash
	if err := b.db.WriteDiff(header.Hash, big.NewInt(1)); err != nil {
		return err
	}
	return nil
}

// Empty checks if the blockchain is empty
func (b *Blockchain) Empty() bool {
	_, ok := b.db.ReadHeadHash()
	return !ok
}

func (b *Blockchain) GetChainTD() (*big.Int, bool) {
	header, ok := b.Header()
	if !ok {
		return nil, false
	}
	return b.GetTD(header.Hash)
}

func (b *Blockchain) GetTD(hash types.Hash) (*big.Int, bool) {
	return b.readDiff(hash)
}

func (b *Blockchain) writeCanonicalHeader(h *types.Header) error {
	td, ok := b.readDiff(h.ParentHash)
	if !ok {
		return fmt.Errorf("parent difficulty not found 2")
	}

	diff := big.NewInt(1).Add(td, new(big.Int).SetUint64(h.Difficulty))
	if err := b.db.WriteCanonicalHeader(h, diff); err != nil {
		return err
	}

	// notify the listeners
	for _, ch := range b.listeners {
		select {
		case ch <- h:
		default:
		}
	}

	return nil
}

func (b *Blockchain) advanceHead(h *types.Header) error {
	if err := b.db.WriteHeadHash(h.Hash); err != nil {
		return err
	}
	if err := b.db.WriteHeadNumber(h.Number); err != nil {
		return err
	}
	if err := b.db.WriteCanonicalHash(h.Number, h.Hash); err != nil {
		return err
	}

	if h.ParentHash != types.StringToHash("") {
		// Dont write difficulty for genesis
		td, ok := b.readDiff(h.ParentHash)
		if !ok {
			return fmt.Errorf("parent difficulty not found 1")
		}

		if err := b.db.WriteDiff(h.Hash, big.NewInt(1).Add(td, new(big.Int).SetUint64(h.Difficulty))); err != nil {
			return err
		}
	}

	for _, ch := range b.listeners {
		select {
		case ch <- h:
		default:
		}
	}

	return nil
}

// Header returns the header of the blockchain
func (b *Blockchain) Header() (*types.Header, bool) {
	// TODO, We may get better insight about the error if we know in which specific
	// step it failed. Not sure yet if this expression will be a storage interface in itself
	// in the future.
	hash, ok := b.db.ReadHeadHash()
	if !ok {
		return nil, false
	}
	header, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}
	return header, true
}

// CommitBodies writes the bodies
func (b *Blockchain) CommitBodies(headers []types.Hash, bodies []*types.Body) error {
	if len(headers) != len(bodies) {
		return fmt.Errorf("lengths dont match %d and %d", len(headers), len(bodies))
	}

	for indx, hash := range headers {
		if err := b.db.WriteBody(hash, bodies[indx]); err != nil {
			return err
		}
	}
	return nil
}

// CommitReceipts writes the receipts
func (b *Blockchain) CommitReceipts(headers []types.Hash, receipts [][]*types.Receipt) error {
	if len(headers) != len(receipts) {
		return fmt.Errorf("lengths dont match %d and %d", len(headers), len(receipts))
	}
	for indx, hash := range headers {
		if err := b.db.WriteReceipts(hash, receipts[indx]); err != nil {
			return err
		}
	}
	return nil
}

// CommitChain writes all the other data related to the chain (body and receipts).
// TODO: I think this function is not used anymore.
func (b *Blockchain) CommitChain(blocks []*types.Block, receipts [][]*types.Receipt) error {
	if len(blocks) != len(receipts) {
		return fmt.Errorf("length dont match. %d and %d", len(blocks), len(receipts))
	}

	for i := 1; i < len(blocks); i++ {
		if blocks[i].Number()-1 != blocks[i-1].Number() {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, blocks[i].Number(), blocks[i-1].Number())
		}
		if blocks[i].ParentHash() != blocks[i-1].Hash() {
			return fmt.Errorf("parent hash not correct")
		}
	}

	for indx, block := range blocks {
		r := receipts[indx]

		hash := block.Hash()

		body := &types.Body{
			Transactions: block.Transactions,
			Uncles:       block.Uncles,
		}
		if err := b.db.WriteBody(hash, body); err != nil {
			return err
		}
		if err := b.db.WriteReceipts(hash, r); err != nil {
			return err
		}
	}

	return nil
}

// GetReceiptsByHash returns the receipts by their hash
func (b *Blockchain) GetReceiptsByHash(hash types.Hash) []*types.Receipt {
	r, _ := b.db.ReadReceipts(hash)
	return r
}

// GetBodyByHash returns the body by their hash
func (b *Blockchain) GetBodyByHash(hash types.Hash) (*types.Body, bool) {
	return b.readBody(hash)
}

// GetHeaderByHash returns the header by his hash
func (b *Blockchain) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return b.readHeader(hash)
}

func (b *Blockchain) readHeader(hash types.Hash) (*types.Header, bool) {
	h, ok := b.headersCache.Get(hash)
	if ok {
		return h.(*types.Header), true
	}
	hh, ok := b.db.ReadHeader(hash)
	if !ok {
		return nil, false
	}
	b.headersCache.Add(hash, hh)
	return hh, true
}

func (b *Blockchain) readBody(hash types.Hash) (*types.Body, bool) {
	body, ok := b.bodiesCache.Get(hash)
	if ok {
		return body.(*types.Body), true
	}
	bb, ok := b.db.ReadBody(hash)
	if !ok {
		return nil, false
	}
	b.bodiesCache.Add(hash, bb)
	return bb, true
}

func (b *Blockchain) readDiff(hash types.Hash) (*big.Int, bool) {
	d, ok := b.difficultyCache.Get(hash)
	if ok {
		return d.(*big.Int), true
	}
	dd, ok := b.db.ReadDiff(hash)
	if !ok {
		return nil, false
	}
	b.difficultyCache.Add(hash, dd)
	return dd, true
}

// GetHeaderByNumber returns the header by his number
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

func (b *Blockchain) WriteHeaders(headers []*types.Header) error {
	return b.WriteHeadersWithBodies(headers)
}

// WriteHeadersWithBodies writes a batch of headers
func (b *Blockchain) WriteHeadersWithBodies(headers []*types.Header) error {
	// validate chain
	for i := 1; i < len(headers); i++ {
		if headers[i].Number-1 != headers[i-1].Number {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, headers[i].Number, headers[i-1].Number)
		}
		if headers[i].ParentHash != headers[i-1].Hash {
			return fmt.Errorf("parent hash not correct")
		}
	}

	for _, h := range headers {
		if err := b.WriteHeader(h); err != nil {
			return err
		}
	}
	return nil
}

// WriteBlocks writes a batch of blocks
func (b *Blockchain) WriteBlocks(blocks []*types.Block) error {
	if len(blocks) == 0 {
		return fmt.Errorf("no headers found to insert")
	}

	parent, ok := b.readHeader(blocks[0].ParentHash())
	if !ok {
		return fmt.Errorf("parent of %s (%d) not found: %s", blocks[0].Hash().String(), blocks[0].Number(), blocks[0].ParentHash())
	}

	// validate chain
	for i := 0; i < len(blocks); i++ {
		block := blocks[i]

		if blocks[i].Number()-1 != parent.Number {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, blocks[i].Number(), parent.Number)
		}
		if blocks[i].ParentHash() != parent.Hash {
			return fmt.Errorf("parent hash not correct")
		}
		if err := b.consensus.VerifyHeader(nil, parent, blocks[i].Header, false, true); err != nil {
			return fmt.Errorf("failed to verify the header: %v", err)
		}

		// This is not necessary.

		// verify body data
		if hash := buildroot.CalculateUncleRoot(block.Uncles); hash != blocks[i].Header.Sha3Uncles {
			return fmt.Errorf("uncle root hash mismatch: have %s, want %s", hash, blocks[i].Header.Sha3Uncles)
		}
		// TODO, the wrapper around transactions
		if hash := buildroot.CalculateTransactionsRoot(block.Transactions); hash != blocks[i].Header.TxRoot {
			return fmt.Errorf("transaction root hash mismatch: have %s, want %s", hash, blocks[i].Header.TxRoot)
		}
		parent = blocks[i].Header
	}

	// Write chain
	for indx, block := range blocks {
		header := block.Header

		body := block.Body()
		if err := b.db.WriteBody(block.Header.Hash, block.Body()); err != nil {
			return err
		}
		b.bodiesCache.Add(block.Header.Hash, body)

		// Verify uncles. It requires to have the bodies on memory
		if err := b.VerifyUncles(block); err != nil {
			return err
		}
		// Process and validate the block
		if err := b.processBlock(blocks[indx]); err != nil {
			return err
		}
		// Write the header to the chain
		if err := b.WriteHeader(header); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) processBlock(block *types.Block) error {
	header := block.Header

	// process the block
	parent, ok := b.readHeader(header.ParentHash)
	if !ok {
		return fmt.Errorf("unknown ancestor 1")
	}
	transition, root, err := b.executor.ProcessBlock(parent.StateRoot, block)
	if err != nil {
		return err
	}

	// validate the fields
	if root != header.StateRoot {
		return fmt.Errorf("invalid merkle root")
	}
	if transition.TotalGas() != header.GasUsed {
		return fmt.Errorf("gas used is different")
	}
	receiptSha := buildroot.CalculateReceiptsRoot(transition.Receipts())
	if receiptSha != header.ReceiptsRoot {
		return fmt.Errorf("invalid receipts root")
	}
	rbloom := types.CreateBloom(transition.Receipts())
	if rbloom != header.LogsBloom {
		return fmt.Errorf("invalid receipts bloom")
	}
	return nil
}

var emptyFrom = types.Address{}

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

func (b *Blockchain) GetHashByNumber(i uint64) types.Hash {
	block, ok := b.GetBlockByNumber(i, false)
	if !ok {
		return types.Hash{}
	}
	return block.Hash()
}

func (b *Blockchain) VerifyUncles(block *types.Block) error {
	if len(block.Uncles) == 0 {
		return nil
	}
	if len(block.Uncles) > 2 {
		return fmt.Errorf("too many uncles")
	}

	// Gather the set of past uncles and ancestors
	uncles, ancestors := mapset.NewSet(), make(map[types.Hash]*types.Header)

	number, parent := block.Number()-1, block.ParentHash()
	for i := 0; i < 7; i++ {
		ancestor, ok := b.GetBlockByHash(parent, true)
		if !ok {
			break
		}
		ancestors[ancestor.Hash()] = ancestor.Header
		for _, uncle := range ancestor.Uncles {
			uncles.Add(uncle.Hash)
		}
		parent, number = ancestor.ParentHash(), number-1
	}
	ancestors[block.Hash()] = block.Header
	uncles.Add(block.Hash())

	// Verify each of the uncles that it's recent, but not an ancestor
	for _, uncle := range block.Uncles {
		// Make sure every uncle is rewarded only once
		hash := uncle.Hash
		if uncles.Contains(hash) {
			return errDuplicateUncle
		}
		uncles.Add(hash)

		// Make sure the uncle has a valid ancestry
		if ancestors[hash] != nil {
			return errUncleIsAncestor
		}
		if ancestors[uncle.ParentHash] == nil || uncle.ParentHash == block.ParentHash() {
			return errDanglingUncle
		}

		if err := b.consensus.VerifyHeader(nil, ancestors[uncle.ParentHash], uncle, true, false); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) addHeader(header *types.Header) error {
	b.headersCache.Add(header.Hash, header)

	if err := b.db.WriteHeader(header); err != nil {
		return err
	}
	if err := b.db.WriteCanonicalHash(header.Number, header.Hash); err != nil {
		return err
	}
	return nil
}

// WriteBlock writes a block of data
func (b *Blockchain) WriteBlock(block *types.Block) error {
	return b.WriteHeader(block.Header)
}

// WriteHeader writes a block and the data, assumes the genesis is already set
func (b *Blockchain) WriteHeader(header *types.Header) error {
	head, ok := b.Header()
	if !ok {
		return fmt.Errorf("header not found")
	}

	// Write the data
	if header.ParentHash == head.Hash {
		// Fast path to save the new canonical header
		return b.writeCanonicalHeader(header)
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
		return fmt.Errorf("parent of %s (%d) not found", header.Hash.String(), header.Number)
	}
	if err := b.db.WriteDiff(header.Hash, big.NewInt(1).Add(parentDiff, new(big.Int).SetUint64(header.Difficulty))); err != nil {
		return err
	}
	b.headersCache.Add(header.Hash, header)

	incomingDiff := big.NewInt(1).Add(parentDiff, new(big.Int).SetUint64(header.Difficulty))
	if incomingDiff.Cmp(headerDiff) > 0 {
		// new block has higher difficulty than us, reorg the chain
		if err := b.handleReorg(head, header); err != nil {
			return err
		}
	} else {
		// new block has lower difficulty than us, create a new fork
		if err := b.writeFork(header); err != nil {
			return err
		}
	}

	return nil
}

// SideChainCh returns the channel of headers
func (b *Blockchain) SideChainCh() chan *types.Header {
	return b.sidechainCh
}

func (b *Blockchain) writeFork(header *types.Header) error {
	forks := b.db.ReadForks()

	select {
	case b.sidechainCh <- header:
	default:
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

func (b *Blockchain) handleReorg(oldHeader *types.Header, newHeader *types.Header) error {
	newChainHead := newHeader
	oldChainHead := oldHeader

	oldChain := []*types.Header{}
	newChain := []*types.Header{}

	var ok bool

	for oldHeader.Number > newHeader.Number {
		oldHeader, ok = b.readHeader(oldHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", oldHeader.ParentHash.String())
		}
		oldChain = append(oldChain, oldHeader)
	}

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

	if err := b.writeFork(oldChainHead); err != nil {
		return fmt.Errorf("failed to write the old header as fork: %v", err)
	}

	// Update canonical chain numbers
	for _, h := range newChain {
		if err := b.db.WriteCanonicalHash(h.Number, h.Hash); err != nil {
			return err
		}
	}

	// oldChain headers can become now uncles
	go func() {
		for _, i := range oldChain {
			select {
			case b.sidechainCh <- i:
			default:
			}
		}
	}()

	return b.advanceHead(newChainHead)
}

// GetForks returns the forks
func (b *Blockchain) GetForks() []types.Hash {
	return b.db.ReadForks()
}

// GetBlockByHash returns the block by their hash
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
	body, ok := b.readBody(hash)
	if !ok {
		return block, true
	}

	block.Transactions = body.Transactions
	block.Uncles = body.Uncles
	return block, true

	// return block.WithBody(body.Transactions, body.Uncles), true
}

// GetBlockByNumber returns the block by their number
func (b *Blockchain) GetBlockByNumber(n uint64, full bool) (*types.Block, bool) {
	hash, ok := b.db.ReadCanonicalHash(n)
	if !ok {
		return nil, false
	}
	return b.GetBlockByHash(hash, full)
}

func (b *Blockchain) Close() error {
	return b.db.Close()
}
