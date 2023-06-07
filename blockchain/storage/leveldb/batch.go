package leveldb

import (
	"github.com/syndtr/goleveldb/leveldb"
)

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

func (b *batchLevelDB) Write() error {
	return b.db.Write(b.b, nil)
}

func (b *batchLevelDB) Put(k []byte, data []byte) {
	b.b.Put(k, data)
}
