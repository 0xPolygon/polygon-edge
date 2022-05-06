package blockchain

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/types/buildroot"
)

var (
	// defaultBlockGasTarget is the default value for the block gas target for new blocks
	defaultBlockGasTarget uint64 = 8000000
)

// NewTestHeaderChainWithSeed creates a new chain with a seed factor
func NewTestHeaderChainWithSeed(genesis *types.Header, n int, seed uint64) []*types.Header {
	head := func(i int64) *types.Header {
		return &types.Header{
			Number:       uint64(i),
			GasLimit:     seed,
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
func NewTestHeaderFromChainWithSeed(headers []*types.Header, n int, seed uint64) []*types.Header {
	// We do +1 because the first header will be the genesis we supplied
	newHeaders := NewTestHeaderChainWithSeed(headers[len(headers)-1], n+1, seed)

	preHeaders := make([]*types.Header, len(headers))
	copy(preHeaders, headers)

	return append(preHeaders, newHeaders[1:]...) //nolint:makezero
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
	genesis.Header.ComputeHash()

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
			Value:    big.NewInt(0),
			Gas:      0,
			GasPrice: big.NewInt(0),
			Input:    header.Hash.Bytes(),
			V:        big.NewInt(27),
		}
		t0.ComputeHash()

		txs := []*types.Transaction{t0}

		// -- receipts --
		r0 := &types.Receipt{
			GasUsed: uint64(i),
			//TxHash:            t0.Hash,
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

// NewTestBlockchain creates a new dummy blockchain for testing
func NewTestBlockchain(t *testing.T, headers []*types.Header) *Blockchain {
	t.Helper()

	genesis := &chain.Genesis{
		Number:   0,
		GasLimit: 0,
	}
	config := &chain.Chain{
		Genesis: genesis,
		Params: &chain.Params{
			Forks: &chain.Forks{
				EIP155:    chain.NewFork(0),
				Homestead: chain.NewFork(0),
			},
			BlockGasTarget: defaultBlockGasTarget,
		},
	}

	st := itrie.NewState(itrie.NewMemoryStorage())
	b, err := newBlockChain(config, state.NewExecutor(config.Params, st, hclog.NewNullLogger()))

	if err != nil {
		t.Fatal(err)
	}

	if headers != nil {
		if _, err := b.advanceHead(headers[0]); err != nil {
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

type MockVerifier struct {
}

func (m *MockVerifier) VerifyHeader(parent, header *types.Header) error {
	return nil
}

func (m *MockVerifier) ProcessHeaders(headers []*types.Header) error {
	return nil
}

func (m *MockVerifier) GetBlockCreator(header *types.Header) (types.Address, error) {
	return header.Miner, nil
}

func (m *MockVerifier) PreStateCommit(header *types.Header, txn *state.Transition) error {
	return nil
}

type mockExecutor struct {
}

func (m *mockExecutor) ProcessBlock(
	parentRoot types.Hash,
	block *types.Block,
	blockCreator types.Address,
) (*state.Transition, error) {
	return nil, nil
}

func TestBlockchain(t *testing.T, genesis *chain.Genesis) *Blockchain {
	if genesis == nil {
		genesis = &chain.Genesis{}
	}

	config := &chain.Chain{
		Genesis: genesis,
		Params: &chain.Params{
			BlockGasTarget: defaultBlockGasTarget,
		},
	}

	b, err := newBlockChain(config, nil)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func newBlockChain(config *chain.Chain, executor Executor) (*Blockchain, error) {
	if executor == nil {
		executor = &mockExecutor{}
	}

	b, err := NewBlockchain(hclog.NewNullLogger(), "", config, &MockVerifier{}, executor)
	if err != nil {
		return nil, err
	}
	// if we are using mock consensus we can compute right away the genesis since
	// this consensus does not change the header hash
	if err = b.ComputeGenesis(); err != nil {
		return nil, fmt.Errorf("compute genisis: %w", err)
	}

	return b, nil
}
