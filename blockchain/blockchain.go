package blockchain

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"

	"github.com/umbracle/minimal/consensus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/state/evm"
	"github.com/umbracle/minimal/state/evm/precompiled"

	mapset "github.com/deckarep/golang-set"
)

var (
	errLargeBlockTime    = errors.New("timestamp too big")
	errZeroBlockTime     = errors.New("timestamp equals parent's")
	errTooManyUncles     = errors.New("too many uncles")
	errDuplicateUncle    = errors.New("duplicate uncle")
	errUncleIsAncestor   = errors.New("uncle is ancestor")
	errDanglingUncle     = errors.New("uncle's parent is not ancestor")
	errInvalidDifficulty = errors.New("non-positive difficulty")
	errInvalidMixDigest  = errors.New("invalid mix digest")
	errInvalidPoW        = errors.New("invalid proof-of-work")
)

// Blockchain is a blockchain reference
type Blockchain struct {
	db          *storage.Storage
	consensus   consensus.Consensus
	genesis     *types.Header
	state       map[string]*state.State
	stateRoot   common.Hash
	params      *chain.Params
	precompiled map[common.Address]*precompiled.Precompiled
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(db *storage.Storage, consensus consensus.Consensus, params *chain.Params) *Blockchain {
	return &Blockchain{
		db:        db,
		consensus: consensus,
		genesis:   nil,
		state:     map[string]*state.State{},
		params:    params,
	}
}

func (b *Blockchain) SetPrecompiled(precompiled map[common.Address]*precompiled.Precompiled) {
	b.precompiled = precompiled
}

// GetParent return the parent
func (b *Blockchain) GetParent(header *types.Header) *types.Header {
	return b.db.ReadHeader(header.ParentHash)
}

// Genesis returns the genesis block
func (b *Blockchain) Genesis() *types.Header {
	return b.genesis
}

// WriteGenesis writes the genesis block if not present
func (b *Blockchain) WriteGenesis(genesis *chain.Genesis) error {
	// The chain is not empty
	if !b.Empty() {
		return nil
	}

	s := state.NewState()

	txn := s.Txn()
	for addr, account := range genesis.Alloc {
		if account.Balance != nil {
			txn.AddBalance(addr, account.Balance)
		}
		if account.Nonce != 0 {
			txn.SetNonce(addr, account.Nonce)
		}
		if len(account.Code) != 0 {
			txn.SetCode(addr, account.Code)
		}
		for key, value := range account.Storage {
			txn.SetState(addr, key, value)
		}
	}

	ss, root := txn.Commit(false)

	header := genesis.ToBlock()
	header.Root = common.BytesToHash(root)

	b.genesis = header
	b.state[hexutil.Encode(root)] = ss
	b.stateRoot = common.BytesToHash(root)

	// add genesis block
	if err := b.addHeader(header); err != nil {
		return err
	}
	if err := b.advanceHead(header); err != nil {
		return err
	}

	return nil
}

func (b *Blockchain) getStateRoot(root common.Hash) (*state.State, bool) {
	s, ok := b.state[root.String()]
	return s, ok
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

	b.state[types.EmptyRootHash.String()] = state.NewState()
	return nil
}

// Empty checks if the blockchain is empty
func (b *Blockchain) Empty() bool {
	hash := b.db.ReadHeadHash()
	if hash == nil {
		return true
	}
	return false
}

func (b *Blockchain) advanceHead(h *types.Header) error {
	b.db.WriteHeadHash(h.Hash())
	b.db.WriteHeadNumber(h.Number)
	return nil
}

// Header returns the header of the blockchain
func (b *Blockchain) Header() *types.Header {
	hash := b.db.ReadHeadHash()
	if hash == nil {
		return nil
	}
	header := b.db.ReadHeader(*hash)
	return header
}

// CommitBodies writes the bodies
func (b *Blockchain) CommitBodies(headers []common.Hash, bodies []*types.Body) error {
	if len(headers) != len(bodies) {
		return fmt.Errorf("lengths dont match %d and %d", len(headers), len(bodies))
	}

	for indx, hash := range headers {
		b.db.WriteBody(hash, bodies[indx])
	}
	return nil
}

// CommitReceipts writes the receipts
func (b *Blockchain) CommitReceipts(headers []common.Hash, receipts []types.Receipts) error {
	if len(headers) != len(receipts) {
		return fmt.Errorf("lengths dont match %d and %d", len(headers), len(receipts))
	}
	for indx, hash := range headers {
		b.db.WriteReceipts(hash, receipts[indx])
	}
	return nil
}

// CommitChain writes all the other data related to the chain (body and receipts)
func (b *Blockchain) CommitChain(blocks []*types.Block, receipts [][]*types.Receipt) error {
	if len(blocks) != len(receipts) {
		return fmt.Errorf("length dont match. %d and %d", len(blocks), len(receipts))
	}

	for i := 1; i < len(blocks); i++ {
		if blocks[i].Number().Uint64()-1 != blocks[i-1].Number().Uint64() {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, blocks[i].Number().Uint64(), blocks[i-1].Number().Uint64())
		}
		if blocks[i].ParentHash() != blocks[i-1].Hash() {
			return fmt.Errorf("parent hash not correct")
		}
		// TODO, validate bodies
	}

	for indx, block := range blocks {
		r := receipts[indx]

		hash := block.Hash()
		b.db.WriteBody(hash, block.Body())
		b.db.WriteReceipts(hash, r)
	}

	return nil
}

// GetReceiptsByHash returns the receipts by their hash
func (b *Blockchain) GetReceiptsByHash(hash common.Hash) types.Receipts {
	r := b.db.ReadReceipts(hash)
	return r
}

// GetBodyByHash returns the body by their hash
func (b *Blockchain) GetBodyByHash(hash common.Hash) *types.Body {
	return b.db.ReadBody(hash)
}

// GetHeaderByHash returns the header by his hash
func (b *Blockchain) GetHeaderByHash(hash common.Hash) *types.Header {
	return b.db.ReadHeader(hash)
}

// GetHeaderByNumber returns the header by his number
func (b *Blockchain) GetHeaderByNumber(n *big.Int) *types.Header {
	hash := b.db.ReadCanonicalHash(n)
	h := b.db.ReadHeader(hash)
	return h
}

func (b *Blockchain) WriteHeaders(headers []*types.Header) error {
	panic("TODO")
}

// WriteHeadersWithBodies writes a batch of headers
func (b *Blockchain) WriteHeadersWithBodies(headers []*types.Header) error {
	// validate chain
	for i := 1; i < len(headers); i++ {
		if headers[i].Number.Uint64()-1 != headers[i-1].Number.Uint64() {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, headers[i].Number.Uint64(), headers[i-1].Number.Uint64())
		}
		if headers[i].ParentHash != headers[i-1].Hash() {
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

	headers := make([]*types.Header, len(blocks)-1)
	for _, block := range blocks {
		headers = append(headers, block.Header())
	}

	parent := b.db.ReadHeader(headers[0].ParentHash)
	if parent == nil {
		return fmt.Errorf("parent of %s (%d) not found", headers[0].Hash().String(), headers[0].Number.Uint64())
	}

	// validate chain
	for i := 0; i < len(headers); i++ {
		block := blocks[i]

		if headers[i].Number.Uint64()-1 != parent.Number.Uint64() {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, headers[i].Number.Uint64(), parent.Number.Uint64())
		}
		if headers[i].ParentHash != parent.Hash() {
			return fmt.Errorf("parent hash not correct")
		}
		if err := b.consensus.VerifyHeader(parent, headers[i], false, true); err != nil {
			return fmt.Errorf("failed to verify the header: %v", err)
		}

		// verify uncles
		if err := b.VerifyUncles(block); err != nil {
			return err
		}

		// verify body data
		if hash := types.CalcUncleHash(block.Uncles()); hash != headers[i].UncleHash {
			return fmt.Errorf("uncle root hash mismatch: have %x, want %x", hash, headers[i].UncleHash)
		}
		if hash := types.DeriveSha(block.Transactions()); hash != headers[i].TxHash {
			return fmt.Errorf("transaction root hash mismatch: have %x, want %x", hash, headers[i].TxHash)
		}

		parent = headers[i]
	}

	// NOTE: This is only done for the tests for now, write all the blocks to memory
	for _, block := range blocks {
		b.db.WriteBody(block.Header().Hash(), block.Body())
	}

	for indx, h := range headers {

		// Try to write first the state transition
		parent := b.db.ReadHeader(headers[indx].ParentHash)
		if parent == nil {
			return fmt.Errorf("unknown ancestor")
		}

		st, ok := b.getStateRoot(parent.Root)
		if !ok {
			return fmt.Errorf("unknown ancestor")
		}

		block := blocks[indx]
		state, root, receipts, totalGas, err := b.Process(st, block)
		if err != nil {
			return err
		}

		// Validate the result

		if hexutil.Encode(root) != block.Root().String() {
			return fmt.Errorf("invalid merkle root")
		}
		if totalGas != block.GasUsed() {
			return fmt.Errorf("gas used is different")
		}

		receiptSha := types.DeriveSha(receipts)
		if receiptSha != block.ReceiptHash() {
			return fmt.Errorf("invalid receipt root hash (remote: %x local: %x)", block.ReceiptHash(), receiptSha)
		}
		rbloom := types.CreateBloom(receipts)
		if rbloom != block.Bloom() {
			return fmt.Errorf("invalid bloom (remote: %x  local: %x)", block.Bloom(), rbloom)
		}

		if err := b.WriteHeader(h); err != nil {
			return err
		}

		// Add state if everything worked
		b.state[hexutil.Encode(root)] = state

		// b.state = state
		b.stateRoot = common.BytesToHash(root)
	}

	// fmt.Printf("Done: last header written was %s at %s\n", headers[len(headers)-1].Hash().String(), headers[len(headers)-1].Number.String())
	return nil
}

func (b *Blockchain) WriteAuxBlocks(block *types.Block) {
	b.db.WriteBody(block.Header().Hash(), block.Body())
}

func (b *Blockchain) GetState(header *types.Header) (*state.State, bool) {
	s, ok := b.state[header.Root.String()]
	return s, ok
}

func (b *Blockchain) BlockIterator(s *state.State, header *types.Header, getTx func(err error, gas uint64) (*types.Transaction, bool)) (*state.State, []byte, []*types.Transaction, error) {

	// add the rewards
	txn := s.Txn()

	// start the gasPool
	config := b.params.Forks.At(header.Number.Uint64())

	// gasPool
	gasPool := NewGasPool(header.GasLimit)

	totalGas := uint64(0)

	receipts := types.Receipts{}

	legacyConfig := &params.ChainConfig{
		ChainID:        big.NewInt(1), // TODO, Always 1 in tests
		EIP155Block:    nil,
		HomesteadBlock: nil,
	}
	if b.params.Forks.EIP155 != nil {
		legacyConfig.EIP155Block = b.params.Forks.EIP155.Int()
	}
	if b.params.Forks.Homestead != nil {
		legacyConfig.HomesteadBlock = b.params.Forks.Homestead.Int()
	}

	var txerr error

	count := 0

	txns := []*types.Transaction{}

	// apply the transactions
	for {
		tx, ok := getTx(txerr, gasPool.gas)
		if !ok {
			break
		}

		msg, err := tx.AsMessage(types.MakeSigner(legacyConfig, header.Number))
		if err != nil {
			panic(err)
		}

		gasTable := b.params.GasTable(header.Number)

		env := &evm.Env{
			Coinbase:   header.Coinbase,
			Timestamp:  header.Time,
			Number:     header.Number,
			Difficulty: header.Difficulty,
			GasLimit:   big.NewInt(int64(header.GasLimit)),
			GasPrice:   tx.GasPrice(),
		}

		gasUsed, failed, err := txn.Apply(&msg, env, gasTable, config, b.GetHashByNumber, gasPool, false, b.precompiled)
		if err != nil {
			continue
		}

		txerr = err
		totalGas += gasUsed

		logs := txn.Logs()

		ss, root := txn.Commit(config.EIP155)
		txn = ss.Txn()

		if config.Byzantium {
			root = []byte{}
		}

		// Create receipt

		receipt := types.NewReceipt(root, failed, totalGas)
		receipt.TxHash = tx.Hash()
		receipt.GasUsed = gasUsed

		// if the transaction created a contract, store the creation address in the receipt.
		if msg.To() == nil {
			receipt.ContractAddress = crypto.CreateAddress(msg.From(), tx.Nonce())
		}

		// Set the receipt logs and create a bloom for filtering
		receipt.Logs = buildLogs(logs, tx.Hash(), header.Hash(), uint(count))
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipts = append(receipts, receipt)

		count++

		txns = append(txns, tx)
	}

	// without uncles
	if err := b.consensus.Finalize(txn, types.NewBlock(header, nil, nil, nil)); err != nil {
		panic(err)
	}

	s2, root := txn.Commit(config.EIP155)

	fmt.Println(s2)
	fmt.Println(root)

	return s2, root, txns, nil
}

func (b *Blockchain) Process(s *state.State, block *types.Block) (*state.State, []byte, types.Receipts, uint64, error) {
	// add the rewards
	txn := s.Txn()

	// start the gasPool
	config := b.params.Forks.At(block.Number().Uint64())

	// gasPool
	gasPool := NewGasPool(block.GasLimit())

	totalGas := uint64(0)

	receipts := types.Receipts{}

	// apply the transactions
	for indx, tx := range block.Transactions() {
		legacyConfig := &params.ChainConfig{
			ChainID:        big.NewInt(1), // TODO, Always 1 in tests
			EIP155Block:    nil,
			HomesteadBlock: nil,
		}
		if b.params.Forks.EIP155 != nil {
			legacyConfig.EIP155Block = b.params.Forks.EIP155.Int()
		}
		if b.params.Forks.Homestead != nil {
			legacyConfig.HomesteadBlock = b.params.Forks.Homestead.Int()
		}

		msg, err := tx.AsMessage(types.MakeSigner(legacyConfig, block.Number()))
		if err != nil {
			panic(err)
		}

		gasTable := b.params.GasTable(block.Number())

		env := &evm.Env{
			Coinbase:   block.Coinbase(),
			Timestamp:  block.Time(),
			Number:     block.Number(),
			Difficulty: block.Difficulty(),
			GasLimit:   big.NewInt(int64(block.GasLimit())),
			GasPrice:   tx.GasPrice(),
		}

		gasUsed, failed, err := txn.Apply(&msg, env, gasTable, config, b.GetHashByNumber, gasPool, false, b.precompiled)
		if err != nil {
			return nil, nil, nil, 0, err
		}

		totalGas += gasUsed

		logs := txn.Logs()

		ss, root := txn.Commit(config.EIP155)
		txn = ss.Txn()

		if config.Byzantium {
			root = []byte{}
		}

		// Create receipt

		receipt := types.NewReceipt(root, failed, totalGas)
		receipt.TxHash = tx.Hash()
		receipt.GasUsed = gasUsed

		// if the transaction created a contract, store the creation address in the receipt.
		if msg.To() == nil {
			receipt.ContractAddress = crypto.CreateAddress(msg.From(), tx.Nonce())
		}

		// Set the receipt logs and create a bloom for filtering
		receipt.Logs = buildLogs(logs, tx.Hash(), block.Hash(), uint(indx))
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipts = append(receipts, receipt)
	}

	if err := b.consensus.Finalize(txn, block); err != nil {
		panic(err)
	}

	s2, root := txn.Commit(config.EIP155)

	return s2, root, receipts, totalGas, nil
}

func buildLogs(logs []*types.Log, txHash, blockHash common.Hash, txIndex uint) []*types.Log {
	newLogs := []*types.Log{}

	for indx, log := range logs {
		newLog := log

		newLog.TxHash = txHash
		newLog.BlockHash = blockHash
		newLog.TxIndex = txIndex
		newLog.Index = uint(indx)

		newLogs = append(newLogs, newLog)
	}

	return newLogs
}

func (b *Blockchain) GetHashByNumber(i uint64) common.Hash {
	block := b.GetBlockByNumber(big.NewInt(int64(i)), false)
	if block == nil {
		return common.Hash{}
	}
	return block.Hash()
}

func (b *Blockchain) VerifyUncles(block *types.Block) error {

	// Verify that there are at most 2 uncles included in this block
	if len(block.Uncles()) > 2 {
		return fmt.Errorf("too many uncles")
	}

	// Gather the set of past uncles and ancestors
	uncles, ancestors := mapset.NewSet(), make(map[common.Hash]*types.Header)

	number, parent := block.NumberU64()-1, block.ParentHash()
	for i := 0; i < 7; i++ {
		ancestor := b.GetBlockByHash(parent, true)
		if ancestor == nil {
			break
		}
		ancestors[ancestor.Hash()] = ancestor.Header()
		for _, uncle := range ancestor.Uncles() {
			uncles.Add(uncle.Hash())
		}
		parent, number = ancestor.ParentHash(), number-1
	}
	ancestors[block.Hash()] = block.Header()
	uncles.Add(block.Hash())

	// Verify each of the uncles that it's recent, but not an ancestor
	for _, uncle := range block.Uncles() {
		// Make sure every uncle is rewarded only once
		hash := uncle.Hash()
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

		if err := b.consensus.VerifyHeader(ancestors[uncle.ParentHash], uncle, true, false); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) addHeader(header *types.Header) error {
	b.db.WriteHeader(header)
	b.db.WriteCanonicalHash(header.Number, header.Hash())
	return nil
}

// WriteBlock writes a block of data
func (b *Blockchain) WriteBlock(block *types.Block) error {
	return b.WriteHeader(block.Header())
}

// WriteHeader writes a block and the data, assumes the genesis is already set
func (b *Blockchain) WriteHeader(header *types.Header) error {
	head := b.Header()

	parent := b.db.ReadHeader(header.ParentHash)
	if parent == nil {
		return fmt.Errorf("parent of %s (%d) not found", header.Hash().String(), header.Number.Uint64())
	}

	// local difficulty of the block
	localDiff := big.NewInt(1).Add(parent.Difficulty, header.Difficulty)

	// Write the data
	if err := b.addHeader(header); err != nil {
		return err
	}

	if header.ParentHash == head.Hash() {
		// advance the chain
		if err := b.advanceHead(header); err != nil {
			return err
		}
	} else if head.Difficulty.Cmp(localDiff) < 0 {
		// reorg
		if err := b.handleReorg(head, header); err != nil {
			return err
		}
	} else {
		// fork
		if err := b.writeFork(header); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) writeFork(header *types.Header) error {
	forks := b.db.ReadForks()

	newForks := []common.Hash{}
	for _, fork := range forks {
		if fork != header.ParentHash {
			newForks = append(newForks, fork)
		}
	}
	newForks = append(newForks, header.Hash())
	b.db.WriteForks(newForks)
	return nil
}

func (b *Blockchain) handleReorg(oldHeader *types.Header, newHeader *types.Header) error {
	newChainHead := newHeader
	oldChainHead := oldHeader

	for oldHeader.Number.Cmp(newHeader.Number) > 0 {
		oldHeader = b.db.ReadHeader(oldHeader.ParentHash)
	}

	for newHeader.Number.Cmp(oldHeader.Number) > 0 {
		newHeader = b.db.ReadHeader(newHeader.ParentHash)
	}

	for oldHeader.Hash() != newHeader.Hash() {
		oldHeader = b.db.ReadHeader(oldHeader.ParentHash)
		newHeader = b.db.ReadHeader(newHeader.ParentHash)
	}

	if err := b.writeFork(oldChainHead); err != nil {
		return fmt.Errorf("failed to write the old header as fork: %v", err)
	}

	// NOTE. this loops are used to know the oldblocks not belonging anymore
	// to the canonical chain and updating the tx and state

	return b.advanceHead(newChainHead)
}

// GetForks returns the forks
func (b *Blockchain) GetForks() []common.Hash {
	return b.db.ReadForks()
}

// GetBlockByHash returns the block by their hash
func (b *Blockchain) GetBlockByHash(hash common.Hash, full bool) *types.Block {
	header := b.db.ReadHeader(hash)
	if header == nil {
		return nil
	}
	block := types.NewBlockWithHeader(header)
	if !full {
		return block
	}
	body := b.db.ReadBody(hash)
	if body == nil {
		return block
	}
	return block.WithBody(body.Transactions, body.Uncles)
}

// GetBlockByNumber returns the block by their number
func (b *Blockchain) GetBlockByNumber(n *big.Int, full bool) *types.Block {
	return b.GetBlockByHash(b.db.ReadCanonicalHash(n), full)
}
