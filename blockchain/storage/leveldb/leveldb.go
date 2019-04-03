package leveldb

import (
	"fmt"
	"log"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/minimal/blockchain/storage"
)

// Factory creates a leveldb storage
func Factory(config map[string]interface{}, logger *log.Logger) (storage.Storage, error) {
	path, ok := config["path"]
	if !ok {
		return nil, fmt.Errorf("path not found")
	}
	pathStr, ok := path.(string)
	if !ok {
		return nil, fmt.Errorf("path is not a string")
	}
	return NewLevelDBStorage(pathStr, logger)
}

// NewLevelDBStorage creates the new storage reference with leveldb
func NewLevelDBStorage(path string, logger *log.Logger) (storage.Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	kv := &levelDBKV{db}
	return storage.NewKeyValueStorage(logger, kv), nil
}

// levelDBKV is the leveldb implementation of the kv storage
type levelDBKV struct {
	db *leveldb.DB
}

func (l *levelDBKV) Set(p []byte, v []byte) error {
	return l.db.Put(p, v, nil)
}

func (l *levelDBKV) Get(p []byte) ([]byte, bool, error) {
	data, err := l.db.Get(p, nil)
	if err != nil {
		if err.Error() == "leveldb: not found" {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}
