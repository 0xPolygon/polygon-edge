package badgerdb

import (
	"fmt"
	"log"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/umbracle/minimal/blockchain/storage"
)

// Factory creates a leveldb storage
func Factory(config map[string]string, logger *log.Logger) (storage.Storage, error) {
	path, ok := config["path"]
	if !ok {
		return nil, fmt.Errorf("path not found")
	}
	return NewBadgerDBStorage(path, logger)
}

// NewBadgerDBStorage creates the new storage reference with badgerDB
func NewBadgerDBStorage(path string, logger *log.Logger) (storage.Storage, error) {
	opts := badger.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	kv := &badgerDBKV{db}
	return storage.NewKeyValueStorage(logger, kv), nil
}

// badgerDBKV is the leveldb implementation of the kv storage
type badgerDBKV struct {
	db *badger.DB
}

func (b *badgerDBKV) Set(p []byte, v []byte) error {
	return b.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(p, v)
		return err
	})
}

func (b *badgerDBKV) Get(p []byte) ([]byte, bool, error) {
	var val []byte
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(p)
		if err != nil {
			return err
		}
		val, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return nil
	})
	return val, true, err
}
