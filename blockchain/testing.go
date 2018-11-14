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

// NewTestChainWithSeed creates a new chain with a seed factor
func NewTestChainWithSeed(n int, seed int) []*types.Header {
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

// NewTestChain creates a chain of valid headers
func NewTestChain(n int) []*types.Header {
	return NewTestChainWithSeed(n, 0)
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
