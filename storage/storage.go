package storage

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
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
)

// sub-prefix

var (
	HASH   = []byte("hash")
	NUMBER = []byte("number")
	EMPTY  = []byte("")
)

// Storage is the blockchain storage using boltdb
type Storage struct {
	db *leveldb.DB
}

// NewStorage creates the new storage reference
func NewStorage(path string) (*Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &Storage{db}, nil
}

// Close closes the storage connection
func (s *Storage) Close() error {
	return s.Close()
}

// -- head --

// ReadHeadHash returns the hash of the head
func (s *Storage) ReadHeadHash() (*common.Hash, error) {
	data, err := s.get(HEAD, HASH)
	if err != nil {
		return nil, err
	}
	hash := common.BytesToHash(data)
	return &hash, nil
}

// ReadHeadNumber returns the number of the head
func (s *Storage) ReadHeadNumber() (*big.Int, error) {
	data, err := s.get(HEAD, HASH)
	if err != nil {
		return nil, err
	}
	n := big.NewInt(1).SetBytes(data)
	return n, nil
}

// WriteHeadHash writes the hash of the head
func (s *Storage) WriteHeadHash(h common.Hash) error {
	return s.set(HEAD, HASH, h.Bytes())
}

// WriteHeadNumber writes the number of the head
func (s *Storage) WriteHeadNumber(n *big.Int) error {
	return s.set(HEAD, NUMBER, n.Bytes())
}

// -- fork --

// WriteForks writes the current forks
func (s *Storage) WriteForks(forks []common.Hash) error {
	data, err := rlp.EncodeToBytes(forks)
	if err != nil {
		return err
	}
	return s.set(FORK, EMPTY, data)
}

// ReadForks read the current forks
func (s *Storage) ReadForks() ([]common.Hash, error) {
	data, err := s.get(FORK, EMPTY)
	if err != nil {
		return nil, err
	}
	var forks []common.Hash
	err = rlp.DecodeBytes(data, &forks)
	return forks, err
}

// -- difficulty --

// WriteDiff writes the difficulty
func (s *Storage) WriteDiff(hash common.Hash, diff *big.Int) error {
	return s.set(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

// ReadDiff reads the difficulty
func (s *Storage) ReadDiff(hash common.Hash) (*big.Int, error) {
	v, err := s.get(DIFFICULTY, hash.Bytes())
	if err != nil {
		return nil, err
	}
	return big.NewInt(0).SetBytes(v), nil
}

// -- header --

// WriteHeader writes the header
func (s *Storage) WriteHeader(h *types.Header) error {
	data, err := rlp.EncodeToBytes(h)
	if err != nil {
		return err
	}
	return s.set(HEADER, h.Hash().Bytes(), data)
}

// ReadHeader reads the header
func (s *Storage) ReadHeader(hash common.Hash) (*types.Header, error) {
	data, err := s.get(HEADER, hash.Bytes())
	if err != nil {
		return nil, err
	}
	var header *types.Header
	err = rlp.DecodeBytes(data, &header)
	return header, err
}

// -- write ops --

func (s *Storage) set(p []byte, k []byte, v []byte) error {
	return s.db.Put(k, v, nil)
}

func (s *Storage) get(p []byte, k []byte) ([]byte, error) {
	return s.db.Get(k, nil)
}
