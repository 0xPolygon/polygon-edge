package storage

import (
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// prefix

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
)

// sub-prefix

var (
	HASH   = []byte("hash")
	NUMBER = []byte("number")
	EMPTY  = []byte("empty")
)

// KV is a key value storage interface
type KV interface {
	Set(p []byte, v []byte) error
	Get(p []byte) ([]byte, bool, error)
}

// KeyValueStorage is a generic storage for kv databases
type KeyValueStorage struct {
	logger *log.Logger
	db     KV
	Db     KV
}

func NewKeyValueStorage(logger *log.Logger, db KV) Storage {
	return &KeyValueStorage{logger: logger, db: db}
}

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *KeyValueStorage) ReadCanonicalHash(n *big.Int) (common.Hash, bool) {
	data, ok := s.get(CANONICAL, n.Bytes())
	if !ok {
		return common.Hash{}, false
	}
	return common.BytesToHash(data), true
}

// WriteCanonicalHash writes a hash for a number block in the canonical chain
func (s *KeyValueStorage) WriteCanonicalHash(n *big.Int, hash common.Hash) error {
	return s.set(CANONICAL, n.Bytes(), hash.Bytes())
}

// -- head --

// ReadHeadHash returns the hash of the head
func (s *KeyValueStorage) ReadHeadHash() (common.Hash, bool) {
	data, ok := s.get(HEAD, HASH)
	if !ok {
		return common.Hash{}, false
	}
	return common.BytesToHash(data), true
}

// ReadHeadNumber returns the number of the head
func (s *KeyValueStorage) ReadHeadNumber() (*big.Int, bool) {
	data, ok := s.get(HEAD, HASH)
	if !ok {
		return nil, false
	}
	return big.NewInt(1).SetBytes(data), true
}

// WriteHeadHash writes the hash of the head
func (s *KeyValueStorage) WriteHeadHash(h common.Hash) error {
	return s.set(HEAD, HASH, h.Bytes())
}

// WriteHeadNumber writes the number of the head
func (s *KeyValueStorage) WriteHeadNumber(n *big.Int) error {
	return s.set(HEAD, NUMBER, n.Bytes())
}

// -- fork --

// WriteForks writes the current forks
func (s *KeyValueStorage) WriteForks(forks []common.Hash) error {
	return s.write(FORK, EMPTY, forks)
}

// ReadForks read the current forks
func (s *KeyValueStorage) ReadForks() []common.Hash {
	var forks []common.Hash
	s.read(FORK, EMPTY, &forks)
	return forks
}

// -- difficulty --

// WriteDiff writes the difficulty
func (s *KeyValueStorage) WriteDiff(hash common.Hash, diff *big.Int) error {
	return s.set(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

// ReadDiff reads the difficulty
func (s *KeyValueStorage) ReadDiff(hash common.Hash) (*big.Int, bool) {
	v, ok := s.get(DIFFICULTY, hash.Bytes())
	if !ok {
		return nil, false
	}
	return big.NewInt(0).SetBytes(v), true
}

// -- header --

// WriteHeader writes the header
func (s *KeyValueStorage) WriteHeader(h *types.Header) error {
	return s.write(HEADER, h.Hash().Bytes(), h)
}

// ReadHeader reads the header
func (s *KeyValueStorage) ReadHeader(hash common.Hash) (*types.Header, bool) {
	var header *types.Header
	ok := s.read(HEADER, hash.Bytes(), &header)
	return header, ok
}

// -- body --

// WriteBody writes the body
func (s *KeyValueStorage) WriteBody(hash common.Hash, body *types.Body) error {
	return s.write(BODY, hash.Bytes(), body)
}

// ReadBody reads the body
func (s *KeyValueStorage) ReadBody(hash common.Hash) (*types.Body, bool) {
	var body *types.Body
	ok := s.read(BODY, hash.Bytes(), &body)
	return body, ok
}

// -- receipts --

// WriteReceipts writes the receipts
func (s *KeyValueStorage) WriteReceipts(hash common.Hash, receipts []*types.Receipt) error {
	storageReceipts := make([]*types.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*types.ReceiptForStorage)(receipt)
	}
	return s.write(RECEIPTS, hash.Bytes(), storageReceipts)
}

// ReadReceipts reads the receipts
func (s *KeyValueStorage) ReadReceipts(hash common.Hash) []*types.Receipt {
	var storage []*types.ReceiptForStorage
	s.read(RECEIPTS, hash.Bytes(), &storage)

	receipts := make([]*types.Receipt, len(storage))
	for i, receipt := range storage {
		receipts[i] = (*types.Receipt)(receipt)
	}

	return receipts
}

// -- write ops --

func (s *KeyValueStorage) write(p []byte, k []byte, obj interface{}) error {
	data, err := rlp.EncodeToBytes(obj)
	if err != nil {
		return fmt.Errorf("failed to encode rlp: %v", err)
	}
	return s.set(p, k, data)
}

func (s *KeyValueStorage) read(p []byte, k []byte, obj interface{}) bool {
	data, ok := s.get(p, k)
	if !ok {
		return false
	}
	if err := rlp.DecodeBytes(data, obj); err != nil {
		s.logger.Printf("failed to decode rlp: %v", err)
		return false
	}
	return true
}

func (s *KeyValueStorage) set(p []byte, k []byte, v []byte) error {
	p = append(p, k...)
	return s.db.Set(p, v)
}

func (s *KeyValueStorage) get(p []byte, k []byte) ([]byte, bool) {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)
	if err != nil {
		s.logger.Printf("failed to read: %v", err)
		return nil, false
	}
	return data, ok
}
