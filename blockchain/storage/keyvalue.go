//nolint:stylecheck
package storage

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"
)

// Prefixes for the key-value store
var (
	// DIFFICULTY is the difficulty prefix
	DIFFICULTY = []byte("d")

	// HEADER is the header prefix
	HEADER = []byte("h")

	// HEAD is the chain head prefix
	HEAD = []byte("o")

	// FORK is the entry to store forks
	FORK = []byte("f")

	// CANONICAL is the prefix for the canonical chain numbers
	CANONICAL = []byte("c")

	// BODY is the prefix for bodies
	BODY = []byte("b")

	// RECEIPTS is the prefix for receipts
	RECEIPTS = []byte("r")

	// SNAPSHOTS is the prefix for snapshots
	SNAPSHOTS = []byte("s")

	// TX_LOOKUP_PREFIX is the prefix for transaction lookups
	TX_LOOKUP_PREFIX = []byte("l")
)

// Sub-prefixes
var (
	HASH   = []byte("hash")
	NUMBER = []byte("number")
	EMPTY  = []byte("empty")
)

// KV is a key value storage interface.
//
// KV = Key-Value
type KV interface {
	Close() error
	Get(p []byte) ([]byte, bool, error)
	NewBatch() Batch
}

// KeyValueStorage is a generic storage for kv databases
type KeyValueStorage struct {
	logger hclog.Logger
	db     KV
	Db     KV
}

func NewKeyValueStorage(logger hclog.Logger, db KV) Storage {
	return &KeyValueStorage{logger: logger, db: db}
}

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *KeyValueStorage) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	data, ok := s.get(CANONICAL, common.EncodeUint64ToBytes(n))
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// HEAD //

// ReadHeadHash returns the hash of the head
func (s *KeyValueStorage) ReadHeadHash() (types.Hash, bool) {
	data, ok := s.get(HEAD, HASH)
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// ReadHeadNumber returns the number of the head
func (s *KeyValueStorage) ReadHeadNumber() (uint64, bool) {
	data, ok := s.get(HEAD, NUMBER)
	if !ok {
		return 0, false
	}

	if len(data) != 8 {
		return 0, false
	}

	return common.EncodeBytesToUint64(data), true
}

// FORK //

// ReadForks read the current forks
func (s *KeyValueStorage) ReadForks() ([]types.Hash, error) {
	forks := &Forks{}
	err := s.readRLP(FORK, EMPTY, forks)

	return *forks, err
}

// DIFFICULTY //

// ReadTotalDifficulty reads the difficulty
func (s *KeyValueStorage) ReadTotalDifficulty(hash types.Hash) (*big.Int, bool) {
	v, ok := s.get(DIFFICULTY, hash.Bytes())
	if !ok {
		return nil, false
	}

	return big.NewInt(0).SetBytes(v), true
}

// HEADER //

// ReadHeader reads the header
func (s *KeyValueStorage) ReadHeader(hash types.Hash) (*types.Header, error) {
	header := &types.Header{}
	err := s.readRLP(HEADER, hash.Bytes(), header)

	return header, err
}

// BODY //

// ReadBody reads the body
func (s *KeyValueStorage) ReadBody(hash types.Hash) (*types.Body, error) {
	body := &types.Body{}
	if err := s.readRLP(BODY, hash.Bytes(), body); err != nil {
		return nil, err
	}

	// must read header because block number is needed in order to calculate each tx hash
	header := &types.Header{}
	if err := s.readRLP(HEADER, hash.Bytes(), header); err != nil {
		return nil, err
	}

	for _, tx := range body.Transactions {
		tx.ComputeHash(header.Number)
	}

	return body, nil
}

// RECEIPTS //

// ReadReceipts reads the receipts
func (s *KeyValueStorage) ReadReceipts(hash types.Hash) ([]*types.Receipt, error) {
	receipts := &types.Receipts{}
	err := s.readRLP(RECEIPTS, hash.Bytes(), receipts)

	return *receipts, err
}

// TX LOOKUP //

// ReadTxLookup reads the block hash using the transaction hash
func (s *KeyValueStorage) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	parser := &fastrlp.Parser{}

	v := s.read2(TX_LOOKUP_PREFIX, hash.Bytes(), parser)
	if v == nil {
		return types.Hash{}, false
	}

	blockHash := []byte{}
	blockHash, err := v.GetBytes(blockHash[:0], 32)

	if err != nil {
		return types.Hash{}, false
	}

	return types.BytesToHash(blockHash), true
}

var ErrNotFound = fmt.Errorf("not found")

func (s *KeyValueStorage) readRLP(p, k []byte, raw types.RLPUnmarshaler) error {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)

	if err != nil {
		return err
	}

	if !ok {
		return ErrNotFound
	}

	if obj, ok := raw.(types.RLPStoreUnmarshaler); ok {
		// decode in the store format
		if err := obj.UnmarshalStoreRLP(data); err != nil {
			return err
		}
	} else {
		// normal rlp decoding
		if err := raw.UnmarshalRLP(data); err != nil {
			return err
		}
	}

	return nil
}

func (s *KeyValueStorage) read2(p, k []byte, parser *fastrlp.Parser) *fastrlp.Value {
	data, ok := s.get(p, k)
	if !ok {
		return nil
	}

	v, err := parser.Parse(data)
	if err != nil {
		return nil
	}

	return v
}

func (s *KeyValueStorage) get(p []byte, k []byte) ([]byte, bool) {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)

	if err != nil {
		return nil, false
	}

	return data, ok
}

// Close closes the connection with the db
func (s *KeyValueStorage) Close() error {
	return s.db.Close()
}

// NewBatch creates batch used for write/update/delete operations
func (s *KeyValueStorage) NewBatch() Batch {
	return s.db.NewBatch()
}
