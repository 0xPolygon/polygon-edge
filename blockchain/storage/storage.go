package storage

import (
	"math/big"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/types"
)

// Storage is a generic blockchain storage
type Storage interface {
	ReadCanonicalHash(n uint64) (types.Hash, bool)
	WriteCanonicalHash(n uint64, hash types.Hash) error

	ReadHeadHash() (types.Hash, bool)
	ReadHeadNumber() (uint64, bool)
	WriteHeadHash(h types.Hash) error
	WriteHeadNumber(uint64) error

	WriteForks(forks []types.Hash) error
	ReadForks() []types.Hash

	WriteDiff(hash types.Hash, diff *big.Int) error
	ReadDiff(hash types.Hash) (*big.Int, bool)

	WriteHeader(h *types.Header) error
	ReadHeader(hash types.Hash) (*types.Header, bool)

	WriteCanonicalHeader(h *types.Header, diff *big.Int) error

	WriteBody(hash types.Hash, body *types.Body) error
	ReadBody(hash types.Hash) (*types.Body, bool)

	WriteSnapshot(hash types.Hash, blob []byte) error
	ReadSnapshot(hash types.Hash) ([]byte, bool)

	WriteReceipts(hash types.Hash, receipts []*types.Receipt) error
	ReadReceipts(hash types.Hash) ([]*types.Receipt, bool)

	WriteTxLookup(hash types.Hash, blockHash types.Hash) error
	ReadTxLookup(hash types.Hash) (types.Hash, bool)

	// Chain indexer //
	ReadIndexSectionHead(section uint64) types.Hash
	WriteIndexSectionHead(section uint64, hash types.Hash) error
	RemoveSectionHead(section uint64) error
	WriteValidSectionsNum(sections uint64) error
	ReadValidSectionsNum() (uint64, error)
	WriteBloomBits(bitNum uint, currentSection uint64, bHead types.Hash, bits []byte)

	Close() error
}

// Factory is a factory method to create a blockchain storage
type Factory func(config map[string]interface{}, logger hclog.Logger) (Storage, error)
