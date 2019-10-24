package blockchain

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/umbracle/minimal/helper/dao"
	"github.com/umbracle/minimal/helper/derivesha"
	"github.com/umbracle/minimal/helper/hex"

	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/crypto"

	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/state/runtime"
	"github.com/umbracle/minimal/state/runtime/evm"
	"github.com/umbracle/minimal/state/runtime/precompiled"

	itrie "github.com/umbracle/minimal/state/immutable-trie"
	"github.com/umbracle/minimal/types"

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
	db          storage.Storage
	consensus   consensus.Consensus
	genesis     *types.Header
	state       state.State
	triedb      itrie.Storage
	params      *chain.Params
	sidechainCh chan *types.Header

	// listener for advancedhead subscribers
	listeners []chan *types.Header

	headersCache    *lru.Cache
	bodiesCache     *lru.Cache
	difficultyCache *lru.Cache

	// executor of the transition
	executor *state.Executor

	daoBlock uint64
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(db storage.Storage, st state.State, consensus consensus.Consensus, params *chain.Params) *Blockchain {
	b := &Blockchain{
		db:          db,
		consensus:   consensus,
		genesis:     nil,
		state:       st,
		params:      params,
		sidechainCh: make(chan *types.Header, 10),
		listeners:   []chan *types.Header{},
		daoBlock:    dao.DAOForkBlock,
		executor:    state.NewExecutor(),
	}

	// setup the executor
	b.executor.SetRuntime(precompiled.NewPrecompiled())
	b.executor.SetRuntime(evm.NewEVM())

	b.headersCache, _ = lru.New(100)
	b.bodiesCache, _ = lru.New(100)
	b.difficultyCache, _ = lru.New(100)
	return b
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
func (b *Blockchain) Genesis() *types.Header {
	return b.genesis
}

// WriteGenesis writes the genesis block if not present
func (b *Blockchain) WriteGenesis(genesis *chain.Genesis) error {
	// Build precompiled contracts
	/*
		prec := map[types.Address]*precompiled.Precompiled{}
		for addr, i := range genesis.Alloc {
			if i.Builtin != nil {
				j, err := precompiled.CreatePrecompiled(i.Builtin)
				if err != nil {
					return err
				}
				prec[addr] = j
			}
		}
	*/

	// b.SetPrecompiled(prec)

	// The chain is not empty
	if !b.Empty() {
		// load genesis from memory
		genesisHash, ok := b.db.ReadCanonicalHash(0)
		if !ok {
			return fmt.Errorf("genesis hash not found")
		}
		b.genesis, ok = b.readHeader(genesisHash)
		if !ok {
			return fmt.Errorf("genesis header '%s' not found", genesisHash.String())
		}
		return nil
	}

	snap := b.state.NewSnapshot()

	txn := state.NewTxn(b.state, snap)
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

	_, root := txn.Commit(false)

	header := genesis.ToBlock()
	header.StateRoot = types.BytesToHash(root)
	header.ComputeHash()

	b.genesis = header

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

func (b *Blockchain) getStateRoot(root types.Hash) (state.Snapshot, bool) {
	ss, err := b.state.NewSnapshotAt(root)
	if err != nil {
		return nil, false
	}
	return ss, true
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

	b.genesis = header.Copy()
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

	/*
		// notify the listeners
		for _, ch := range b.listeners {
			select {
			case ch <- h:
			default:
			}
		}
	*/

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
		return fmt.Errorf("parent of %s (%d) not found", blocks[0].Hash().String(), blocks[0].Number())
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
		if err := b.consensus.VerifyHeader(parent, blocks[i].Header, false, true); err != nil {
			return fmt.Errorf("failed to verify the header: %v", err)
		}

		// This is not necessary.

		// verify body data
		if hash := derivesha.CalcUncleRoot(block.Uncles); hash != blocks[i].Header.Sha3Uncles {
			return fmt.Errorf("uncle root hash mismatch: have %s, want %s", hash, blocks[i].Header.Sha3Uncles)
		}
		// TODO, the wrapper around transactions
		if hash := derivesha.CalcTxsRoot(block.Transactions); hash != blocks[i].Header.TxRoot {
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

		// verify uncles. It requires to have the bodies in memory. TODO: Part of the consensus? Only required on POW.
		if err := b.VerifyUncles(block); err != nil {
			return err
		}

		// Try to write first the state transition
		parent, ok := b.readHeader(header.ParentHash)
		if !ok {
			return fmt.Errorf("unknown ancestor 1")
		}

		st, ok := b.getStateRoot(parent.StateRoot)
		if !ok {
			return fmt.Errorf("unknown state root: %s", parent.StateRoot.String())
		}

		if parent.StateRoot != (types.Hash{}) {
			// Done for testing. TODO, remove.

			block := blocks[indx]
			_, root, receipts, totalGas, err := b.Process(st, block)
			if err != nil {
				return err
			}

			// Validate the result

			if hex.EncodeToHex(root) != header.StateRoot.String() {
				return fmt.Errorf("invalid merkle root")
			}
			if totalGas != header.GasUsed {
				return fmt.Errorf("gas used is different")
			}

			receiptSha := derivesha.CalcReceiptRoot(receipts)
			if receiptSha != header.ReceiptsRoot {
				return fmt.Errorf("invalid receipt root hash (remote: %x local: %x)", header.ReceiptsRoot, receiptSha)
			}
			rbloom := types.CreateBloom(receipts)
			if rbloom != header.LogsBloom {
				return fmt.Errorf("invalid bloom (remote: %x  local: %x)", header.LogsBloom, rbloom)
			}
		}

		if err := b.WriteHeader(header); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) WriteAuxBlocks(block *types.Block) {
	b.db.WriteBody(block.Header.Hash, block.Body())
}

func (b *Blockchain) GetState(header *types.Header) (state.Snapshot, bool) {
	return b.getStateRoot(header.StateRoot)
}

func (b *Blockchain) SimpleIteration(s state.Snapshot, header *types.Header) ([]byte, error) {
	txn := state.NewTxn(b.state, s)

	// without uncles
	if err := b.consensus.Finalize(txn, &types.Block{Header: header}); err != nil {
		panic(err)
	}

	config := b.params.Forks.At(header.Number)

	_, root := txn.Commit(config.EIP155)
	return root, nil
}

/*
func (b *Blockchain) BlockIterator(s state.Snapshot, header *types.Header, getTx func(err error, gas uint64) (*types.Transaction, bool)) (state.Snapshot, []byte, []*types.Transaction, error) {

	// add the rewards
	// txn := s.Txn()
	txn := state.NewTxn(b.state, s)

	// start the gasPool
	config := b.params.Forks.At(header.Number)

	// gasPool
	gasPool := NewGasPool(header.GasLimit)

	totalGas := uint64(0)

	receipts := []*types.Receipt{}

	var txerr error

	count := 0

	txns := []*types.Transaction{}

	// apply the transactions
	for {
		tx, ok := getTx(txerr, gasPool.gas)
		if !ok {
			break
		}

		signer := crypto.NewSigner(config, uint64(b.params.ChainID))
		from, err := signer.Sender(tx)
		if err != nil {
			panic(err)
		}

		msg := tx.Copy()
		msg.SetFrom(from)

		gasTable := b.params.GasTable(header.Number)

		env := &runtime.Env{
			Coinbase:   header.Miner,
			Timestamp:  header.Timestamp,
			Number:     header.Number,
			Difficulty: new(big.Int).SetUint64(header.Difficulty),
			GasLimit:   big.NewInt(int64(header.GasLimit)),
			GasPrice:   new(big.Int).SetBytes(tx.GasPrice),
		}

		executor := state.NewExecutor(txn, env, config, gasTable, b.GetHashByNumber)

		gasUsed, failed, err := executor.Apply(txn, msg, env, gasTable, config, b.GetHashByNumber, gasPool, false, b.precompiled)

		txerr = err
		totalGas += gasUsed

		logs := txn.Logs()

		ss, root := txn.Commit(config.EIP155)
		txn = state.NewTxn(b.state, ss)

		if config.Byzantium {
			root = []byte{}
		}

		// Create receipt

		receipt := &types.Receipt{
			Root:              types.BytesToHash(root),
			CumulativeGasUsed: totalGas,
			TxHash:            tx.Hash(),
			GasUsed:           gasUsed,
		}
		if failed {
			receipt.Status = 0
		} else {
			receipt.Status = types.ReceiptSuccess
			// receipt.Root = types.ReceiptSuccessBytes
		}

		// if the transaction created a contract, store the creation address in the receipt.
		if msg.To == nil {
			receipt.ContractAddress = crypto.CreateAddress(msg.From(), tx.Nonce)
		}

		// Set the receipt logs and create a bloom for filtering
		receipt.Logs = buildLogs(logs, tx.Hash(), header.Hash(), uint(count))
		receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
		receipts = append(receipts, receipt)

		count++

		txns = append(txns, tx)

	}

	// without uncles
	if err := b.consensus.Finalize(txn, &types.Block{Header: header}); err != nil {
		panic(err)
	}

	s2, root := txn.Commit(config.EIP155)
	return s2, root, txns, nil
}
*/

// SetDAOBlock sets the dao block, only to be used during tests
func (b *Blockchain) SetDAOBlock(n uint64) {
	b.daoBlock = n
}

func (b *Blockchain) Process(s state.Snapshot, block *types.Block) (state.Snapshot, []byte, []*types.Receipt, uint64, error) {
	header := block.Header
	txn := state.NewTxn(b.state, s)

	// Mainnet
	if b.params.ChainID == 1 && block.Number() == b.daoBlock {
		// Apply the DAO hard fork. Move all the balances from 'drain accounts'
		// to a single refund contract.
		for _, i := range dao.DAODrainAccounts {
			addr := types.StringToAddress(i)
			txn.AddBalance(dao.DAORefundContract, txn.GetBalance(addr))
			txn.SetBalance(addr, big.NewInt(0))
		}
	}

	// start the gasPool
	config := b.params.Forks.At(block.Number())

	totalGas := uint64(0)

	receipts := []*types.Receipt{}

	hashByNumber := func(i uint64) (res types.Hash) {
		num, hash := block.Number()-1, block.ParentHash()

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

	env := runtime.TxContext{
		Coinbase:   header.Miner,
		Timestamp:  int64(header.Timestamp),
		Number:     int64(header.Number),
		Difficulty: types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		GasLimit:   int64(header.GasLimit),
	}

	transition := b.executor.NewTransition(txn, hashByNumber, env, config)

	// apply the transactions
	for indx, tx := range block.Transactions {
		transition.SetTxn(txn)

		signer := crypto.NewSigner(config, uint64(b.params.ChainID))
		from, err := signer.Sender(tx)
		if err != nil {
			panic(err)
		}

		msg := tx.Copy()
		msg.From = from

		gasUsed, failed, err := transition.Apply(msg)
		txn = transition.Txn()

		totalGas += gasUsed

		logs := txn.Logs()

		// Pre-Byzantium. Compute state root after each transaction. That root is included in the receipt
		// of the transaction.
		// Post-Byzantium. Compute one single state root after all the transactions.

		var root []byte

		receipt := &types.Receipt{
			CumulativeGasUsed: totalGas,
			TxHash:            tx.Hash,
			GasUsed:           gasUsed,
		}

		if config.Byzantium {
			// The suicided accounts are set as deleted for the next iteration
			txn.CleanDeleteObjects(true)

			if failed {
				receipt.SetStatus(types.ReceiptFailed)
			} else {
				receipt.SetStatus(types.ReceiptSuccess)
			}

		} else {
			ss, aux := txn.Commit(config.EIP155)
			txn = state.NewTxn(b.state, ss)
			root = aux
			receipt.Root = types.BytesToHash(root)
		}

		// Create receipt

		// if the transaction created a contract, store the creation address in the receipt.
		if msg.To == nil {
			receipt.ContractAddress = crypto.CreateAddress(msg.From, tx.Nonce)
		}

		// Set the receipt logs and create a bloom for filtering
		receipt.Logs = buildLogs(logs, tx.Hash, block.Hash(), uint(indx))
		receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
		receipts = append(receipts, receipt)
	}

	if err := b.consensus.Finalize(txn, block); err != nil {
		panic(err)
	}

	s2, root := txn.Commit(config.EIP155)
	return s2, root, receipts, totalGas, nil
}

func buildLogs(logs []*types.Log, txHash, blockHash types.Hash, txIndex uint) []*types.Log {
	newLogs := []*types.Log{}

	for indx, log := range logs {
		newLog := log

		newLog.TxHash = txHash
		newLog.BlockHash = blockHash
		newLog.TxIndex = txIndex
		newLog.LogIndex = uint(indx)

		newLogs = append(newLogs, newLog)
	}

	return newLogs
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

		if err := b.consensus.VerifyHeader(ancestors[uncle.ParentHash], uncle, true, false); err != nil {
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
