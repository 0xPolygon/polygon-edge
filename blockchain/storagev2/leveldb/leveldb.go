package leveldb

import (
	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// levelDB is the leveldb implementation of the kv storage
type levelDB struct {
	db *leveldb.DB
}

// DB key = k + mapper
var tableMapper = map[uint8][]byte{
	storagev2.BODY:         []byte("b"), // DB key = block number + mapper
	storagev2.CANONICAL:    []byte("c"), // DB key = block number + mapper
	storagev2.DIFFICULTY:   []byte("d"), // DB key = block number + mapper
	storagev2.HEADER:       []byte("h"), // DB key = block number + mapper
	storagev2.RECEIPTS:     []byte("r"), // DB key = block number + mapper
	storagev2.FORK:         {},          // DB key = FORK_KEY + mapper
	storagev2.HEAD_HASH:    {},          // DB key = HEAD_HASH_KEY + mapper
	storagev2.HEAD_NUMBER:  {},          // DB key = HEAD_NUMBER_KEY + mapper
	storagev2.BLOCK_LOOKUP: {},          // DB key = block hash + mapper, value = block number
	storagev2.TX_LOOKUP:    {},          // DB key = tx hash + mapper, value = block number
}

// NewLevelDBStorage creates the new storage reference with leveldb default options
func NewLevelDBStorage(path string, logger hclog.Logger) (*storagev2.Storage, error) {
	var ldbs [2]storagev2.Database

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

	return storagev2.Open(logger.Named("leveldb"), ldbs)
}

func openLevelDBStorage(path string, options *opt.Options) (*leveldb.DB, error) {
	db, err := leveldb.OpenFile(path, options)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Get retrieves the key-value pair in leveldb storage
func (l *levelDB) Get(t uint8, k []byte) ([]byte, error) {
	mc := tableMapper[t]
	k = append(k, mc...)

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
func (l *levelDB) NewBatch() storagev2.Batch {
	return newBatchLevelDB(l.db)
}
