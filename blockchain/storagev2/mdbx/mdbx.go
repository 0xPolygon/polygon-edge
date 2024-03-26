package mdbx

import (
	"os"

	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	"github.com/erigontech/mdbx-go/mdbx"
	"github.com/hashicorp/go-hclog"
)

type MdbxOpts struct {
	path  string
	flags uint
}

// MdbxDB is the mdbx implementation of the kv storage
type MdbxDB struct {
	env *mdbx.Env
	dbi [storagev2.MAX_TABLES]mdbx.DBI
}

const (
	B  uint64 = 1
	KB        = B << 10
	MB        = KB << 10
	GB        = MB << 10
	TB        = GB << 10
)

var (
	mapSize    uint64 = 2 * TB
	growthSize uint64 = 2 * GB
)

var tableMapper = map[uint8]string{
	storagev2.BODY:         "Body",
	storagev2.CANONICAL:    "Canonical",
	storagev2.DIFFICULTY:   "Difficulty",
	storagev2.HEADER:       "Header",
	storagev2.RECEIPTS:     "Receipts",
	storagev2.FORK:         "Fork",
	storagev2.HEAD_HASH:    "HeadHash",
	storagev2.HEAD_NUMBER:  "HeadNumber",
	storagev2.BLOCK_LOOKUP: "BlockLookup",
	storagev2.TX_LOOKUP:    "TxLookup",
}

// NewMdbxStorage creates the new storage reference for mdbx database
func NewMdbxStorage(path string, logger hclog.Logger) (*storagev2.Storage, error) {
	var dbs [2]storagev2.Database

	// Set default options
	opts := &MdbxOpts{
		path: path,
	}

	env, err := mdbx.NewEnv()
	if err != nil {
		return nil, err
	}

	if err = env.SetOption(mdbx.OptMaxDB, uint64(storagev2.MAX_TABLES)); err != nil {
		return nil, err
	}

	if err = env.SetGeometry(-1, -1, int(mapSize), int(growthSize), -1, int(defaultPageSize())); err != nil {
		return nil, err
	}

	err = env.Open(opts.path, opts.flags, 0664)
	if err != nil {
		return nil, err
	}

	db := &MdbxDB{
		env: env,
	}

	if err := db.openDBI(0); err != nil {
		return nil, err
	}

	dbs[0] = db
	dbs[1] = nil

	return storagev2.Open(logger.Named("mdbx"), dbs)
}

func defaultPageSize() uint64 {
	osPageSize := os.Getpagesize()
	if osPageSize < 4096 { // reduce further may lead to errors (because some data is just big)
		osPageSize = 4096
	} else if osPageSize > mdbx.MaxPageSize {
		osPageSize = mdbx.MaxPageSize
	}

	osPageSize = osPageSize / 4096 * 4096 // ensure it's rounded

	return uint64(osPageSize)
}

func (db *MdbxDB) view(f func(tx *mdbx.Txn) error) (err error) {
	// can't use db.env.View method - because it calls commit for read transactions - it conflicts with write transactions.
	tx, err := db.env.BeginTxn(nil, mdbx.Readonly)
	if err != nil {
		return err
	}

	return f(tx)
}

func (db *MdbxDB) update(f func(tx *mdbx.Txn) error) (err error) {
	tx, err := db.env.BeginTxn(nil, 0)
	if err != nil {
		return err
	}

	err = f(tx)
	if err != nil {
		return err
	}

	_, err = tx.Commit()

	return err
}

func (db *MdbxDB) openDBI(flags uint) error {
	if flags&mdbx.Accede != 0 {
		return db.view(func(tx *mdbx.Txn) error {
			for i, name := range tableMapper {
				dbi, err := tx.OpenDBISimple(name, mdbx.Accede)
				if err == nil {
					db.dbi[i] = dbi
				} else {
					return err
				}
			}

			return nil
		})
	}

	err := db.update(func(tx *mdbx.Txn) error {
		for i, name := range tableMapper {
			dbi, err := tx.OpenDBISimple(name, mdbx.Create)
			if err != nil {
				return err
			}

			db.dbi[i] = dbi
		}

		return nil
	})

	return err
}

// Get retrieves the key-value pair in mdbx storage
func (db *MdbxDB) Get(t uint8, k []byte) ([]byte, bool, error) {
	tx, err := db.env.BeginTxn(nil, mdbx.Readonly)
	defer tx.Abort()

	if err != nil {
		return nil, false, err
	}

	data, err := tx.Get(db.dbi[t], k)
	if err != nil {
		if err.Error() == "key not found" {
			return nil, false, nil
		}

		return nil, false, err
	}

	return data, true, nil
}

// Close closes the mdbx storage instance
func (db *MdbxDB) Close() error {
	db.env.Close()

	return nil
}

func (db *MdbxDB) NewBatch() storagev2.Batch {
	return newBatchMdbx(db)
}
