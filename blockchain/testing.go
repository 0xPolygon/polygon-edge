package blockchain

import (
	"io/ioutil"
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

// NewTestBlockchain creates a new dummy blockchain for testing
func NewTestBlockchain(t *testing.T) (*Blockchain, func()) {
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
	return NewBlockchain(s, &fakeConsensus{}), close
}
