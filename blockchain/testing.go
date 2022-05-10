package blockchain

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/types"
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

// Verifier delegators

type verifyHeaderDelegate func(*types.Header) error
type processHeadersDelegate func([]*types.Header) error
type getBlockCreatorDelegate func(*types.Header) (types.Address, error)
type preStateCommitDelegate func(*types.Header, *state.Transition) error

type MockVerifier struct {
	verifyHeaderFn    verifyHeaderDelegate
	processHeadersFn  processHeadersDelegate
	getBlockCreatorFn getBlockCreatorDelegate
	preStateCommitFn  preStateCommitDelegate
}

func (m *MockVerifier) VerifyHeader(header *types.Header) error {
	if m.verifyHeaderFn != nil {
		return m.verifyHeaderFn(header)
	}

	return nil
}

func (m *MockVerifier) HookVerifyHeader(fn verifyHeaderDelegate) {
	m.verifyHeaderFn = fn
}

func (m *MockVerifier) ProcessHeaders(headers []*types.Header) error {
	if m.processHeadersFn != nil {
		return m.processHeadersFn(headers)
	}

	return nil
}

func (m *MockVerifier) HookProcessHeaders(fn processHeadersDelegate) {
	m.processHeadersFn = fn
}

func (m *MockVerifier) GetBlockCreator(header *types.Header) (types.Address, error) {
	if m.getBlockCreatorFn != nil {
		return m.getBlockCreatorFn(header)
	}

	return header.Miner, nil
}

func (m *MockVerifier) HookGetBlockCreator(fn getBlockCreatorDelegate) {
	m.getBlockCreatorFn = fn
}

func (m *MockVerifier) PreStateCommit(header *types.Header, txn *state.Transition) error {
	if m.preStateCommitFn != nil {
		return m.preStateCommitFn(header, txn)
	}

	return nil
}

func (m *MockVerifier) HookPreStateCommit(fn preStateCommitDelegate) {
	m.preStateCommitFn = fn
}

// Executor delegators

type processBlockDelegate func(types.Hash, *types.Block, types.Address) (*state.Transition, error)

type mockExecutor struct {
	processBlockFn processBlockDelegate
}

func (m *mockExecutor) ProcessBlock(
	parentRoot types.Hash,
	block *types.Block,
	blockCreator types.Address,
) (*state.Transition, error) {
	if m.processBlockFn != nil {
		return m.processBlockFn(parentRoot, block, blockCreator)
	}

	return nil, nil
}

func (m *mockExecutor) HookProcessBlock(fn processBlockDelegate) {
	m.processBlockFn = fn
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
