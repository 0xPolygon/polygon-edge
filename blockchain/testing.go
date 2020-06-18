package blockchain

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"

	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"
)

type fakeConsensus struct {
}

func (f *fakeConsensus) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

func (f *fakeConsensus) Author(header *types.Header) (types.Address, error) {
	return types.Address{}, nil
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
			Number:       uint64(i),
			GasLimit:     uint64(seed),
			TxRoot:       types.EmptyRootHash,
			Sha3Uncles:   types.EmptyUncleHash,
			ReceiptsRoot: types.EmptyRootHash,
			Difficulty:   uint64(i),
		}
	}

	if genesis == nil {
		genesis = head(0)
		genesis.ComputeHash()
	}
	headers := []*types.Header{genesis}

	count := int64(genesis.Number) + 1
	for i := 1; i < n; i++ {
		header := head(count)
		header.ParentHash = headers[i-1].Hash
		header.ComputeHash()
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
		blocks[indx] = &types.Block{Header: i}
	}
	return blocks
}

// NewTestBodyChain creates a test blockchain with headers, body and receipts
func NewTestBodyChain(n int) ([]*types.Header, []*types.Block, [][]*types.Receipt) {
	genesis := &types.Block{
		Header: &types.Header{
			Number:   0,
			GasLimit: uint64(0),
		},
	}

	blocks := []*types.Block{genesis}
	receipts := [][]*types.Receipt{nil} // genesis does not have tx

	for i := 1; i < n; i++ {
		header := &types.Header{
			ParentHash: blocks[i-1].Hash(),
			Number:     uint64(i),
			Difficulty: uint64(i),
			ExtraData:  []byte{},
		}
		header.ComputeHash()

		// -- txs ---

		addr0 := types.StringToAddress("00")
		t0 := &types.Transaction{
			Nonce:    uint64(i),
			To:       &addr0,
			Value:    big.NewInt(0).Bytes(),
			Gas:      0,
			GasPrice: big.NewInt(0).Bytes(),
			Input:    header.Hash.Bytes(),
			V:        0x27,
		}
		t0.ComputeHash()

		txs := []*types.Transaction{t0}

		// -- receipts --
		r0 := &types.Receipt{
			GasUsed:           uint64(i),
			TxHash:            t0.Hash,
			CumulativeGasUsed: uint64(i), // this value changes the rlpHash
		}
		localReceipts := []*types.Receipt{r0}

		header.TxRoot = buildroot.CalculateTransactionsRoot(txs)
		header.ReceiptsRoot = buildroot.CalculateReceiptsRoot(localReceipts)
		header.LogsBloom = types.CreateBloom(localReceipts)
		header.Sha3Uncles = types.EmptyUncleHash

		block := &types.Block{
			Header:       header,
			Transactions: txs,
		}

		blocks = append(blocks, block)
		receipts = append(receipts, localReceipts)
	}

	headers := []*types.Header{}
	for _, block := range blocks {
		headers = append(headers, block.Header)
	}

	return headers, blocks, receipts
}

// NewTestBlockchainWithBlocks creates a dummy blockchain with headers, bodies and receipts
func NewTestBlockchainWithBlocks(t *testing.T, blocks []*types.Block, receipts [][]*types.Receipt) *Blockchain {
	headers := []*types.Header{}
	for _, block := range blocks {
		headers = append(headers, block.Header)
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

	st := itrie.NewState(itrie.NewMemoryStorage())
	b := NewBlockchain(s, &fakeConsensus{}, state.NewExecutor(config, st))
	if headers != nil {
		if err := b.WriteHeaderGenesis(headers[0]); err != nil {
			t.Fatal(err)
		}
		if err := b.WriteHeaders(headers[1:]); err != nil {
			t.Fatal(err)
		}
	}

	// TODO, find a way to add the snapshot, this will fail until that is fixed.
	// snap, _ := state.NewSnapshot(types.Hash{})
	return b
}

func createGenesis(header *types.Header) *chain.Genesis {
	genesis := &chain.Genesis{
		Nonce:      header.Nonce,
		ExtraData:  header.ExtraData,
		GasLimit:   header.GasLimit,
		Difficulty: header.Difficulty,
		Mixhash:    header.MixHash,
		Coinbase:   header.Miner,
		ParentHash: header.ParentHash,
		Number:     header.Number,
		Timestamp:  header.Timestamp,
	}
	return genesis
}
