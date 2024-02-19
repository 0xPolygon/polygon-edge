package storageV2

import (
	"bytes"

	"github.com/hashicorp/go-hclog"
)

// Database interface.
type Database interface {
	Close() error
	Get(p []byte) ([]byte, error)
	NewBatch() Batch
}

// Database transaction/batch interface
type Batch interface {
	Write() error
	Put(k []byte, v []byte)
}

type Storage struct {
	logger hclog.Logger
	db     [2]Database
}

type Writer struct {
	batch [2]Batch
}

// MCs for the key-value store
var (
	// DIFFICULTY is the difficulty prefix
	DIFFICULTY = []byte("d")

	// HEADER is the header prefix
	HEADER = []byte("h")

	// CANONICAL is the prefix for the canonical chain numbers
	CANONICAL = []byte("c")

	// BODY is the prefix for bodies
	BODY = []byte("b")

	// RECEIPTS is the prefix for receipts
	RECEIPTS = []byte("r")
)

// GidLid database MCs
var (
	// FORK is the entry to store forks
	FORK = []byte("0000000f")

	// HASH is the entry for head hash
	HASH = []byte("0000000h")

	// NUMBER is the entry for head number
	NUMBER = []byte("0000000n")

	// GIDLID is added to the model code as sufix
	GIDLID = []byte{}
)

const (
	MAINDB_INDEX = uint8(0)
	GIDLID_INDEX = uint8(1)
)

func Open(logger hclog.Logger, db [2]Database) (*Storage, error) {
	return &Storage{logger: logger, db: db}, nil
}

func (s *Storage) Close() error {
	for i, db := range s.db {
		if db != nil {
			err := db.Close()
			if err != nil {
				return err
			}

			s.db[i] = nil
		}
	}
	return nil
}

func (s *Storage) NewWriter() *Writer {
	var batch [2]Batch
	batch[0] = s.db[0].NewBatch()
	if s.db[1] != nil {
		batch[1] = s.db[1].NewBatch()
	}
	return &Writer{batch: batch}
}

func getIndex(mc []byte) uint8 {
	if bytes.Equal(mc, GIDLID) {
		return GIDLID_INDEX
	}

	return MAINDB_INDEX
}
