package leveldb

import (
	"github.com/0xPolygon/polygon-edge/blockchain/storageV2"
	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// levelDB is the leveldb implementation of the kv storage
type levelDB struct {
	db *leveldb.DB
}

// NewLevelDBStorage creates the new storage reference with leveldb default options
func NewLevelDBStorage(path string, logger hclog.Logger) (*storageV2.Storage, error) {
	var ldbs [2]storageV2.Database

	// Open LevelDB storage
	// Set default options
	options := &opt.Options{
		BlockCacheCapacity: 64 * opt.MiB,
		WriteBuffer:        128 * opt.MiB, // Two of these are used internally
	}
	db, err := openLevelDBStorage(path, options)
	if err != nil {
		return nil, err
	}

	// Open GidLid
	// Set default options
	options = &opt.Options{
		BlockCacheCapacity: 64 * opt.MiB,
		WriteBuffer:        opt.DefaultWriteBuffer,
	}
	path = path + "/gidlid"
	gidlid, err := openLevelDBStorage(path, options)
	if err != nil {
		return nil, err
	}

	ldbs[0] = &levelDB{db}
	ldbs[1] = &levelDB{gidlid}
	return storageV2.Open(logger.Named("leveldb"), ldbs)
}

func openLevelDBStorage(path string, options *opt.Options) (*leveldb.DB, error) {
	db, err := leveldb.OpenFile(path, options)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Get retrieves the key-value pair in leveldb storage
func (l *levelDB) Get(k []byte) ([]byte, error) {
	data, err := l.db.Get(k, nil)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Close closes the leveldb storage instance
func (l *levelDB) Close() error {
	return l.db.Close()
}

// NewBatch creates batch for database write operations
func (l *levelDB) NewBatch() storageV2.Batch {
	return newBatchLevelDB(l.db)
}
