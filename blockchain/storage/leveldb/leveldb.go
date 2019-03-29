package leveldb

import (
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/umbracle/minimal/blockchain/storage"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
)

// TODO, move memory database out of here

// Factory creates a leveldb storage
func Factory(config map[string]string, logger *log.Logger) (storage.Storage, error) {
	path, ok := config["path"]
	if !ok {
		return nil, fmt.Errorf("path not found")
	}
	return NewLevelDBStorage(path, logger)
}

var ErrNotFound = fmt.Errorf("not found")

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
	set(p []byte, v []byte) error
	get(p []byte) ([]byte, error)
}

// levelDBKV is the leveldb implementation of the kv storage
type levelDBKV struct {
	db *leveldb.DB
}

func (l *levelDBKV) set(p []byte, v []byte) error {
	return l.db.Put(p, v, nil)
}

func (l *levelDBKV) get(p []byte) ([]byte, error) {
	data, err := l.db.Get(p, nil)
	if err != nil {
		if err.Error() == "leveldb: not found" {
			return nil, ErrNotFound
		}
	}
	return data, err
}

// memoryKV is an in memory implementation of the kv storage
type memoryKV struct {
	db map[string][]byte
}

func (m *memoryKV) set(p []byte, v []byte) error {
	m.db[hexutil.Encode(p)] = v
	return nil
}

func (m *memoryKV) get(p []byte) ([]byte, error) {
	v, ok := m.db[hexutil.Encode(p)]
	if !ok {
		return []byte{}, ErrNotFound
	}
	return v, nil
}

// Storage is the blockchain storage using boltdb
type Storage struct {
	logger *log.Logger
	// db     *leveldb.DB
	db KV
}

// NewLevelDBStorage creates the new storage reference with leveldb
func NewLevelDBStorage(path string, logger *log.Logger) (*Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	kv := &levelDBKV{db}
	return &Storage{logger, kv}, nil
}

// NewMemoryStorage creates the new storage reference with inmemory
func NewMemoryStorage(logger *log.Logger) (*Storage, error) {
	db := &memoryKV{map[string][]byte{}}
	return &Storage{logger, db}, nil
}

// Close closes the storage connection
func (s *Storage) Close() error {
	return s.Close()
}

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *Storage) ReadCanonicalHash(n *big.Int) common.Hash {
	data := s.get(CANONICAL, n.Bytes())
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash writes a hash for a number block in the canonical chain
func (s *Storage) WriteCanonicalHash(n *big.Int, hash common.Hash) {
	s.set(CANONICAL, n.Bytes(), hash.Bytes())
}

// -- head --

// ReadHeadHash returns the hash of the head
func (s *Storage) ReadHeadHash() *common.Hash {
	data := s.get(HEAD, HASH)
	if len(data) == 0 {
		return nil
	}
	hash := common.BytesToHash(data)
	return &hash
}

// ReadHeadNumber returns the number of the head
func (s *Storage) ReadHeadNumber() *big.Int {
	data := s.get(HEAD, HASH)
	if len(data) == 0 {
		return nil
	}
	return big.NewInt(1).SetBytes(data)
}

// WriteHeadHash writes the hash of the head
func (s *Storage) WriteHeadHash(h common.Hash) {
	s.set(HEAD, HASH, h.Bytes())
}

// WriteHeadNumber writes the number of the head
func (s *Storage) WriteHeadNumber(n *big.Int) {
	s.set(HEAD, NUMBER, n.Bytes())
}

// -- fork --

// WriteForks writes the current forks
func (s *Storage) WriteForks(forks []common.Hash) {
	s.write(FORK, EMPTY, forks)
}

// ReadForks read the current forks
func (s *Storage) ReadForks() []common.Hash {
	var forks []common.Hash
	s.read(FORK, EMPTY, &forks)
	return forks
}

// -- difficulty --

// WriteDiff writes the difficulty
func (s *Storage) WriteDiff(hash common.Hash, diff *big.Int) {
	s.set(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

// ReadDiff reads the difficulty
func (s *Storage) ReadDiff(hash common.Hash) *big.Int {
	v := s.get(DIFFICULTY, hash.Bytes())
	if len(v) == 0 {
		return nil
	}
	return big.NewInt(0).SetBytes(v)
}

// -- header --

// WriteHeader writes the header
func (s *Storage) WriteHeader(h *types.Header) {
	s.write(HEADER, h.Hash().Bytes(), h)
}

// ReadHeader reads the header
func (s *Storage) ReadHeader(hash common.Hash) *types.Header {
	var header *types.Header
	s.read(HEADER, hash.Bytes(), &header)
	return header
}

// -- body --

// WriteBody writes the body
func (s *Storage) WriteBody(hash common.Hash, body *types.Body) {
	s.write(BODY, hash.Bytes(), body)
}

// ReadBody reads the body
func (s *Storage) ReadBody(hash common.Hash) *types.Body {
	var body *types.Body
	s.read(BODY, hash.Bytes(), &body)
	return body
}

// -- receipts --

// WriteReceipts writes the receipts
func (s *Storage) WriteReceipts(hash common.Hash, receipts []*types.Receipt) {
	storageReceipts := make([]*types.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*types.ReceiptForStorage)(receipt)
	}

	s.write(RECEIPTS, hash.Bytes(), storageReceipts)
}

// ReadReceipts reads the receipts
func (s *Storage) ReadReceipts(hash common.Hash) []*types.Receipt {
	var storage []*types.ReceiptForStorage
	s.read(RECEIPTS, hash.Bytes(), &storage)

	receipts := make([]*types.Receipt, len(storage))
	for i, receipt := range storage {
		receipts[i] = (*types.Receipt)(receipt)
	}

	return receipts
}

// -- write ops --

func (s *Storage) write(p []byte, k []byte, obj interface{}) {
	data, err := rlp.EncodeToBytes(obj)
	if err != nil {
		s.logger.Printf("failed to encode rlp: %v", err)
	} else {
		s.set(p, k, data)
	}
}

func (s *Storage) read(p []byte, k []byte, obj interface{}) {
	data := s.get(p, k)
	if len(data) != 0 {
		if err := rlp.DecodeBytes(data, obj); err != nil {
			s.logger.Printf("failed to decode rlp: %v", err)
		}
	}
}

func (s *Storage) set(p []byte, k []byte, v []byte) {
	p = append(p, k...)
	if err := s.db.set(p, v); err != nil {
		s.logger.Printf("failed to write: %v", err)
	}
}

func (s *Storage) get(p []byte, k []byte) []byte {
	p = append(p, k...)
	data, err := s.db.get(p)
	if err != nil {
		if err != ErrNotFound {
			s.logger.Printf("failed to read: %v", err)
		}
	}
	return data
}
