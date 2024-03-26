package leveldb

import (
	"github.com/syndtr/goleveldb/leveldb"
)

type batchLevelDB struct {
	db *leveldb.DB
	b  *leveldb.Batch
}

func newBatchLevelDB(db *leveldb.DB) *batchLevelDB {
	return &batchLevelDB{
		db: db,
		b:  new(leveldb.Batch),
	}
}

func (b *batchLevelDB) Put(t uint8, k []byte, v []byte) {
	mc := tableMapper[t]
	k = append(append(make([]byte, 0, len(k)+len(mc)), k...), mc...)
	b.b.Put(k, v)
}

func (b *batchLevelDB) Write() error {
	return b.db.Write(b.b, nil)
}
