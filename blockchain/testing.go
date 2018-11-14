package blockchain

import (
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/storage"
)

type fakeConsensus struct {
}

func (f *fakeConsensus) VerifyHeader(parent *types.Header, header *types.Header, seal bool) error {
	return nil
}

func (f *fakeConsensus) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (f *fakeConsensus) Seal(block *types.Block) error {
	return nil
}

func (f *fakeConsensus) Close() error {
	return nil
}

// NewTestHeaderChainWithSeed creates a new chain with a seed factor
func NewTestHeaderChainWithSeed(n int, seed int) []*types.Header {
	genesis := &types.Header{Number: big.NewInt(0), GasLimit: uint64(seed)}
	headers := []*types.Header{genesis}

	for i := 1; i < n; i++ {
		header := &types.Header{
			ParentHash: headers[i-1].Hash(),
			Number:     big.NewInt(int64(i)),
			Difficulty: big.NewInt(int64(i)),
			GasLimit:   uint64(seed), // enough to change the hash
		}

		headers = append(headers, header)
	}

	return headers
}

// NewTestHeaderChain creates a chain of valid headers
func NewTestHeaderChain(n int) []*types.Header {
	return NewTestHeaderChainWithSeed(n, 0)
}

// NewTestBodyChain creates a test blockchain with headers, body and receipts
func NewTestBodyChain(n int) ([]*types.Header, []*types.Block, []types.Receipts) {
	genesis := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(0), GasLimit: uint64(0)})

	blocks := []*types.Block{genesis}
	receipts := []types.Receipts{types.Receipts{}} // genesis does not have tx

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

		block := types.NewBlock(header, txs, nil, nil)

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
func NewTestBlockchainWithBlocks(t *testing.T, blocks []*types.Block, receipts []types.Receipts) (*Blockchain, func()) {
	headers := []*types.Header{}
	for _, block := range blocks {
		headers = append(headers, block.Header())
	}

	b, close := NewTestBlockchain(t, headers)
	if err := b.CommitChain(blocks, receipts); err != nil {
		t.Fatal(err)
	}

	return b, close
}

// NewTestBlockchain creates a new dummy blockchain for testing
func NewTestBlockchain(t *testing.T, headers []*types.Header) (*Blockchain, func()) {
	path, err := ioutil.TempDir("/tmp", "minimal_storage")
	if err != nil {
		t.Fatal(err)
	}
	s, err := storage.NewStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	close := func() {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
	b := NewBlockchain(s, &fakeConsensus{})

	if headers != nil {
		if err := b.WriteGenesis(headers[0]); err != nil {
			t.Fatal(err)
		}
		if err := b.WriteHeaders(headers[1:]); err != nil {
			t.Fatal(err)
		}
	}

	return b, close
}
