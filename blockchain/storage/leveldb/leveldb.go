package leveldb

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	DefaultCache   = int(256)
	DefaultHandles = int(256)
)

// Factory creates a leveldb storage
func Factory(config map[string]interface{}, logger hclog.Logger) (storage.Storage, error) {
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
func NewLevelDBStorage(path string, logger hclog.Logger) (storage.Storage, error) {
	options := opt.Options{}
	// Set default options
	options.OpenFilesCacheCapacity = DefaultHandles
	options.BlockCacheCapacity = DefaultCache / 2 * opt.MiB
	options.WriteBuffer = DefaultCache / 4 * opt.MiB // Two of these are used internally

	db, err := leveldb.OpenFile(path, &options)
	if err != nil {
		return nil, err
	}

	kv := &levelDBKV{db}

	return storage.NewKeyValueStorage(logger.Named("leveldb"), kv), nil
}

// levelDBKV is the leveldb implementation of the kv storage
type levelDBKV struct {
	db *leveldb.DB
}

// Set sets the key-value pair in leveldb storage
func (l *levelDBKV) Set(p []byte, v []byte) error {
	return l.db.Put(p, v, nil)
}

// Get retrieves the key-value pair in leveldb storage
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

// Close closes the leveldb storage instance
func (l *levelDBKV) Close() error {
	return l.db.Close()
}

func (l *levelDBKV) NewBatch() storage.Batch {
	return NewBatchLevelDB(l.db)
}
