package storage

import (
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Storage is a generic blockchain storage
type Storage interface {
	ReadCanonicalHash(n *big.Int) common.Hash
	WriteCanonicalHash(n *big.Int, hash common.Hash)

	ReadHeadHash() *common.Hash
	ReadHeadNumber() *big.Int
	WriteHeadHash(h common.Hash)
	WriteHeadNumber(n *big.Int)

	WriteForks(forks []common.Hash)
	ReadForks() []common.Hash

	WriteDiff(hash common.Hash, diff *big.Int)
	ReadDiff(hash common.Hash) *big.Int

	WriteHeader(h *types.Header)
	ReadHeader(hash common.Hash) *types.Header

	WriteBody(hash common.Hash, body *types.Body)
	ReadBody(hash common.Hash) *types.Body

	WriteReceipts(hash common.Hash, receipts []*types.Receipt)
	ReadReceipts(hash common.Hash) []*types.Receipt
}

// Factory is a factory method to create a blockchain storage
type Factory func(config map[string]string, logger *log.Logger) (Storage, error)
