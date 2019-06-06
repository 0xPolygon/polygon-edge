package storage

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/types"
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
	logger hclog.Logger
	db     KV
	Db     KV
}

func NewKeyValueStorage(logger hclog.Logger, db KV) Storage {
	return &KeyValueStorage{logger: logger, db: db}
}

func (s *KeyValueStorage) encodeUint(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b[:], n)
	return b[:]
}

func (s *KeyValueStorage) decodeUint(b []byte) uint64 {
	return binary.BigEndian.Uint64(b[:])
}

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *KeyValueStorage) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	data, ok := s.get(CANONICAL, s.encodeUint(n))
	if !ok {
		return types.Hash{}, false
	}
	return types.BytesToHash(data), true
}

// WriteCanonicalHash writes a hash for a number block in the canonical chain
func (s *KeyValueStorage) WriteCanonicalHash(n uint64, hash types.Hash) error {
	return s.set(CANONICAL, s.encodeUint(n), hash.Bytes())
}

// -- head --

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
	data, ok := s.get(HEAD, HASH)
	if !ok {
		return 0, false
	}
	if len(data) != 8 {
		return 0, false
	}
	return s.decodeUint(data), true
}

// WriteHeadHash writes the hash of the head
func (s *KeyValueStorage) WriteHeadHash(h types.Hash) error {
	return s.set(HEAD, HASH, h.Bytes())
}

// WriteHeadNumber writes the number of the head
func (s *KeyValueStorage) WriteHeadNumber(n uint64) error {
	return s.set(HEAD, NUMBER, s.encodeUint(n))
}

// -- fork --

// WriteForks writes the current forks
func (s *KeyValueStorage) WriteForks(forks []types.Hash) error {
	return s.write(FORK, EMPTY, forks)
}

// ReadForks read the current forks
func (s *KeyValueStorage) ReadForks() []types.Hash {
	var forks []types.Hash
	s.read(FORK, EMPTY, &forks)
	return forks
}

// -- difficulty --

// WriteDiff writes the difficulty
func (s *KeyValueStorage) WriteDiff(hash types.Hash, diff *big.Int) error {
	return s.set(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

// ReadDiff reads the difficulty
func (s *KeyValueStorage) ReadDiff(hash types.Hash) (*big.Int, bool) {
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
func (s *KeyValueStorage) ReadHeader(hash types.Hash) (*types.Header, bool) {
	var header *types.Header
	ok := s.read(HEADER, hash.Bytes(), &header)
	return header, ok
}

// -- body --

// WriteBody writes the body
func (s *KeyValueStorage) WriteBody(hash types.Hash, body *types.Body) error {
	return s.write(BODY, hash.Bytes(), body)
}

// ReadBody reads the body
func (s *KeyValueStorage) ReadBody(hash types.Hash) (*types.Body, bool) {
	var body *types.Body
	ok := s.read(BODY, hash.Bytes(), &body)
	return body, ok
}

// -- receipts --

// WriteReceipts writes the receipts
func (s *KeyValueStorage) WriteReceipts(hash types.Hash, receipts []*types.Receipt) error {
	return s.write(RECEIPTS, hash.Bytes(), receipts)
}

// ReadReceipts reads the receipts
func (s *KeyValueStorage) ReadReceipts(hash types.Hash) []*types.Receipt {
	var receipts []*types.Receipt
	s.read(RECEIPTS, hash.Bytes(), &receipts)
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
		s.logger.Warn("failed to decode rlp: %v", err)
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
		s.logger.Warn("failed to read: %v", err)
		return nil, false
	}
	return data, ok
}
