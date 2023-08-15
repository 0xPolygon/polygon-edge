package storage

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

// Storage is a generic blockchain storage
type Storage interface {
	ReadCanonicalHash(n uint64) (types.Hash, bool)

	ReadHeadHash() (types.Hash, bool)
	ReadHeadNumber() (uint64, bool)

	ReadForks() ([]types.Hash, error)

	ReadTotalDifficulty(hash types.Hash) (*big.Int, bool)

	ReadHeader(hash types.Hash) (*types.Header, error)

	ReadBody(hash types.Hash) (*types.Body, error)

	ReadReceipts(hash types.Hash) ([]*types.Receipt, error)

	ReadTxLookup(hash types.Hash) (types.Hash, bool)

	NewBatch() Batch

	Close() error
}

// Factory is a factory method to create a blockchain storage
type Factory func(config map[string]interface{}, logger hclog.Logger) (Storage, error)
