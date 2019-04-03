package storage

import (
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Storage is a generic blockchain storage
type Storage interface {
	ReadCanonicalHash(n *big.Int) (common.Hash, bool)
	WriteCanonicalHash(n *big.Int, hash common.Hash) error

	ReadHeadHash() (common.Hash, bool)
	ReadHeadNumber() (*big.Int, bool)
	WriteHeadHash(h common.Hash) error
	WriteHeadNumber(n *big.Int) error

	WriteForks(forks []common.Hash) error
	ReadForks() []common.Hash

	WriteDiff(hash common.Hash, diff *big.Int) error
	ReadDiff(hash common.Hash) (*big.Int, bool)

	WriteHeader(h *types.Header) error
	ReadHeader(hash common.Hash) (*types.Header, bool)

	WriteBody(hash common.Hash, body *types.Body) error
	ReadBody(hash common.Hash) (*types.Body, bool)

	WriteReceipts(hash common.Hash, receipts []*types.Receipt) error
	ReadReceipts(hash common.Hash) []*types.Receipt
}

// Factory is a factory method to create a blockchain storage
type Factory func(config map[string]interface{}, logger *log.Logger) (Storage, error)
