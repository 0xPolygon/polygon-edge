package blockchain

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain/storage/memory"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state"
	trie "github.com/umbracle/minimal/state/immutable-trie"
)

type fakeConsensus struct {
}

func (f *fakeConsensus) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

func (f *fakeConsensus) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (f *fakeConsensus) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return nil, nil
}

func (f *fakeConsensus) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (f *fakeConsensus) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}

func (f *fakeConsensus) Close() error {
	return nil
}

// NewTestHeaderChainWithSeed creates a new chain with a seed factor
func NewTestHeaderChainWithSeed(genesis *types.Header, n int, seed int) []*types.Header {
	head := func(i int64) *types.Header {
		return &types.Header{
			Number:      big.NewInt(i),
			GasLimit:    uint64(seed),
			TxHash:      types.EmptyRootHash,
			UncleHash:   types.EmptyUncleHash,
			ReceiptHash: types.EmptyRootHash,
			Difficulty:  big.NewInt(int64(i)),
		}
	}

	if genesis == nil {
		genesis = head(0)
	}
	headers := []*types.Header{genesis}

	count := genesis.Number.Int64() + 1
	for i := 1; i < n; i++ {
		header := head(count)
		header.ParentHash = headers[i-1].Hash()
		headers = append(headers, header)
		count++
	}

	return headers
}

// NewTestHeaderChain creates a chain of valid headers
func NewTestHeaderChain(n int) []*types.Header {
	return NewTestHeaderChainWithSeed(nil, n, 0)
}

// NewTestHeaderFromChain creates n new headers from an already existing chain
func NewTestHeaderFromChain(headers []*types.Header, n int) []*types.Header {
	return NewTestHeaderFromChainWithSeed(headers, n, 0)
}

// NewTestHeaderFromChainWithSeed creates n new headers from an already existing chain
func NewTestHeaderFromChainWithSeed(headers []*types.Header, n int, seed int) []*types.Header {
	// We do +1 because the first header will be the genesis we supplied
	newHeaders := NewTestHeaderChainWithSeed(headers[len(headers)-1], n+1, seed)

	preHeaders := make([]*types.Header, len(headers))
	copy(preHeaders, headers)

	return append(preHeaders, newHeaders[1:]...)
}

func HeadersToBlocks(headers []*types.Header) []*types.Block {
	blocks := make([]*types.Block, len(headers))
	for indx, i := range headers {
		blocks[indx] = types.NewBlockWithHeader(i)
	}
	return blocks
}

// NewTestBodyChain creates a test blockchain with headers, body and receipts
func NewTestBodyChain(n int) ([]*types.Header, []*types.Block, [][]*types.Receipt) {
	genesis := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(0), GasLimit: uint64(0)})

	blocks := []*types.Block{genesis}
	receipts := [][]*types.Receipt{types.Receipts{}} // genesis does not have tx

	for i := 1; i < n; i++ {
		header := &types.Header{
			ParentHash: blocks[i-1].Hash(),
			Number:     big.NewInt(int64(i)),
			Difficulty: big.NewInt(int64(i)),
			Extra:      []byte{},
		}

		// -- txs ---
		t0 := types.NewTransaction(uint64(i), common.HexToAddress("00"), big.NewInt(0), 0, big.NewInt(0), header.Hash().Bytes())
		txs := []*types.Transaction{t0}

		// -- receipts --
		r0 := types.NewReceipt([]byte{1}, false, uint64(i))
		r0.TxHash = t0.Hash()

		localReceipts := types.Receipts{r0}

		block := types.NewBlock(header, txs, nil, localReceipts)

		blocks = append(blocks, block)
		receipts = append(receipts, localReceipts)
	}

	headers := []*types.Header{}
	for _, block := range blocks {
		headers = append(headers, block.Header())
	}

	return headers, blocks, receipts
}

// NewTestBlockchainWithBlocks creates a dummy blockchain with headers, bodies and receipts
func NewTestBlockchainWithBlocks(t *testing.T, blocks []*types.Block, receipts [][]*types.Receipt) *Blockchain {
	headers := []*types.Header{}
	for _, block := range blocks {
		headers = append(headers, block.Header())
	}

	b := NewTestBlockchain(t, headers)
	if err := b.CommitChain(blocks, receipts); err != nil {
		t.Fatal(err)
	}

	return b
}

// NewTestBlockchain creates a new dummy blockchain for testing
func NewTestBlockchain(t *testing.T, headers []*types.Header) *Blockchain {
	s, err := memory.NewMemoryStorage(nil)
	if err != nil {
		t.Fatal(err)
	}

	config := &chain.Params{
		Forks: &chain.Forks{
			EIP155:    chain.NewFork(0),
			Homestead: chain.NewFork(0),
		},
	}

	state := trie.NewState(trie.NewMemoryStorage())

	b := NewBlockchain(s, state, &fakeConsensus{}, config)
	if headers != nil {
		if err := b.WriteHeaderGenesis(headers[0]); err != nil {
			t.Fatal(err)
		}
		if err := b.WriteHeaders(headers[1:]); err != nil {
			t.Fatal(err)
		}
	}

	// TODO, find a way to add the snapshot, this will fail until that is fixed.
	// snap, _ := state.NewSnapshot(common.Hash{})
	return b
}

func createGenesis(header *types.Header) *chain.Genesis {
	genesis := &chain.Genesis{
		Nonce:      header.Nonce.Uint64(),
		ExtraData:  header.Extra,
		GasLimit:   header.GasLimit,
		Difficulty: header.Difficulty,
		Mixhash:    header.MixDigest,
		Coinbase:   header.Coinbase,
		ParentHash: header.ParentHash,
		Number:     header.Number.Uint64(),
	}
	if header.Time != nil {
		genesis.Timestamp = header.Time.Uint64()
	}
	return genesis
}
