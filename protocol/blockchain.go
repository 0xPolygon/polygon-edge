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
