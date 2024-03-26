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

var tableMapper = map[uint8][]byte{
	// Main DB
	storagev2.BODY:       []byte("b"), // DB key = block number + block hash + mapper, value = block body
	storagev2.DIFFICULTY: []byte("d"), // DB key = block number + block hash + mapper, value = block total diffculty
	storagev2.HEADER:     []byte("h"), // DB key = block number + block hash + mapper, value = block header
	storagev2.RECEIPTS:   []byte("r"), // DB key = block number + block hash + mapper, value = block receipts
	storagev2.CANONICAL:  {},          // DB key = block number + mapper, value = block hash

	// Lookup DB
	storagev2.FORK:         {}, // DB key = FORK_KEY + mapper, value = fork hashes
	storagev2.HEAD_HASH:    {}, // DB key = HEAD_HASH_KEY + mapper, value = head hash
	storagev2.HEAD_NUMBER:  {}, // DB key = HEAD_NUMBER_KEY + mapper, value = head number
	storagev2.BLOCK_LOOKUP: {}, // DB key = block hash + mapper, value = block number
	storagev2.TX_LOOKUP:    {}, // DB key = tx hash + mapper, value = block number
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

	maindb, err := openLevelDBStorage(path, options)
	if err != nil {
		return nil, err
	}

	// Open Lookup
	// Set default options
	options = &opt.Options{
		BlockCacheCapacity: 64 * opt.MiB,
		WriteBuffer:        opt.DefaultWriteBuffer,
	}
	path += "/lookup"

	lookup, err := openLevelDBStorage(path, options)
	if err != nil {
		return nil, err
	}

	ldbs[0] = &levelDB{maindb}
	ldbs[1] = &levelDB{lookup}

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
func (l *levelDB) Get(t uint8, k []byte) ([]byte, bool, error) {
	mc := tableMapper[t]
	k = append(k, mc...)

	data, err := l.db.Get(k, nil)
	if err != nil {
		if err.Error() == "leveldb: not found" {
			return nil, false, nil
		}

		return nil, false, err
	}

	return data, true, nil
}

// Close closes the leveldb storage instance
func (l *levelDB) Close() error {
	return l.db.Close()
}

// NewBatch creates batch for database write operations
func (l *levelDB) NewBatch() storagev2.Batch {
	return newBatchLevelDB(l.db)
}
