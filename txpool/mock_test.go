package txpool

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

/* MOCK */

type defaultMockStore struct {
	DefaultHeader *types.Header
}

func NewDefaultMockStore(header *types.Header) defaultMockStore {
	var defaultHeader = &types.Header{
		GasLimit: 4712388,
	}

	if header != nil {
		defaultHeader = header
	}

	return defaultMockStore{
		defaultHeader,
	}
}

func (m defaultMockStore) Header() *types.Header {
	return m.DefaultHeader
}

func (m defaultMockStore) GetNonce(types.Hash, types.Address) uint64 {
	return 0
}

func (m defaultMockStore) GetBlockByHash(types.Hash, bool) (*types.Block, bool) {
	return nil, false
}

func (m defaultMockStore) GetBalance(types.Hash, types.Address) (*big.Int, error) {
	balance := big.NewInt(0).SetUint64(100000000000000)

	return balance, nil
}

type faultyMockStore struct {
}

func (fms faultyMockStore) Header() *types.Header {
	return &types.Header{}
}

func (fms faultyMockStore) GetNonce(root types.Hash, addr types.Address) uint64 {
	return 99999
}

func (fms faultyMockStore) GetBlockByHash(hash types.Hash, b bool) (*types.Block, bool) {
	return nil, false
}

func (fms faultyMockStore) GetBalance(root types.Hash, addr types.Address) (*big.Int, error) {
	return nil, fmt.Errorf("unable to fetch account state")
}

type mockSigner struct {
}

func (s *mockSigner) Sender(tx *types.Transaction) (types.Address, error) {
	return tx.From, nil
}
