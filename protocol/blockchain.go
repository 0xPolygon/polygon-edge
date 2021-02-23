package protocol

import (
	"math/big"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/types"
)

// Blockchain is the interface required by the syncer to connect to the blockchain
type blockchainShim interface {
	SubscribeEvents() blockchain.Subscription
	Header() *types.Header
	CurrentTD() *big.Int

	GetTD(hash types.Hash) (*big.Int, bool)
	GetReceiptsByHash(types.Hash) ([]*types.Receipt, error)
	GetBodyByHash(types.Hash) (*types.Body, bool)
	GetHeaderByHash(types.Hash) (*types.Header, bool)
	GetHeaderByNumber(n uint64) (*types.Header, bool)

	// advance chain methods
	WriteBlocks(blocks []*types.Block) error
}

type mockBlockchain struct {
}

func (b *mockBlockchain) SubscribeEvents() blockchain.Subscription {
	return nil
}

func (b *mockBlockchain) Header() *types.Header {
	return &types.Header{Number: 0}
}

func (b *mockBlockchain) GetTD(hash types.Hash) (*big.Int, bool) {
	return big.NewInt(0), true
}

func (b *mockBlockchain) GetReceiptsByHash(types.Hash) ([]*types.Receipt, error) {
	return nil, nil
}

func (b *mockBlockchain) GetBodyByHash(types.Hash) (*types.Body, bool) {
	return nil, false
}

func (b *mockBlockchain) GetHeaderByHash(types.Hash) (*types.Header, bool) {
	return nil, false
}

func (b *mockBlockchain) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	return nil, false
}

func (b *mockBlockchain) WriteBlocks(blocks []*types.Block) error {
	return nil
}

func (b *mockBlockchain) CurrentTD() *big.Int {
	return nil
}
