package leveldb

import (
	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/syndtr/goleveldb/leveldb"
)

var _ storage.Batch = (*batchLevelDB)(nil)

type batchLevelDB struct {
	db *leveldb.DB
	b  *leveldb.Batch
}

func NewBatchLevelDB(db *leveldb.DB) *batchLevelDB {
	return &batchLevelDB{
		db: db,
		b:  new(leveldb.Batch),
	}
}

func (b *batchLevelDB) Delete(key []byte) {
	b.b.Delete(key)
}

func (b *batchLevelDB) Put(k []byte, v []byte) {
	b.b.Put(k, v)
}

func (b *batchLevelDB) Write() error {
	return b.db.Write(b.b, nil)
}
