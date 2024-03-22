//nolint:stylecheck
package storagev2

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
)

// Database interface.
type Database interface {
	Close() error
	Get(t uint8, k []byte) ([]byte, bool, error)
	NewBatch() Batch
}

// Database transaction/batch interface
type Batch interface {
	Write() error
	Put(t uint8, k []byte, v []byte)
}

type Storage struct {
	logger hclog.Logger
	db     [2]Database
}

type Writer struct {
	batch [2]Batch
}

// Tables
const (
	BODY       = uint8(0)
	CANONICAL  = uint8(2)
	DIFFICULTY = uint8(4)
	HEADER     = uint8(6)
	RECEIPTS   = uint8(8)
)

// Lookup tables
const (
	FORK         = uint8(0) | LOOKUP_INDEX
	HEAD_HASH    = uint8(2) | LOOKUP_INDEX
	HEAD_NUMBER  = uint8(4) | LOOKUP_INDEX
	BLOCK_LOOKUP = uint8(6) | LOOKUP_INDEX
	TX_LOOKUP    = uint8(8) | LOOKUP_INDEX
)

const MAX_TABLES = uint8(20)

// Database indexes
const (
	MAINDB_INDEX = uint8(0)
	LOOKUP_INDEX = uint8(1)
)

var (
	FORK_KEY        = []byte("0000000f")
	HEAD_HASH_KEY   = []byte("0000000h")
	HEAD_NUMBER_KEY = []byte("0000000n")
)

var ErrNotFound = fmt.Errorf("not found")
var ErrInvalidData = fmt.Errorf("invalid data")

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

func getIndex(t uint8) uint8 {
	if t&LOOKUP_INDEX != 0 {
		return LOOKUP_INDEX
	}

	return MAINDB_INDEX
}
