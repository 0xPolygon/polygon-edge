package trie

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/hashicorp/go-hclog"
	"github.com/ledgerwatch/bolt"
)

// KVStorage is a k/v storage on memory using boltdb
type BoltKVStorage struct {
	db *bolt.DB
}

var bucket []byte = []byte{'b'}

// KVBatch is a batch write for leveldb
type BoltKVBatch struct {
	db *bolt.DB
	tx *bolt.Tx // Manually managed transaction
}

func (batch *BoltKVBatch) Put(k, v []byte) {
	if batch.tx == nil {
		tx, err := batch.db.Begin(true)
		if err != nil {
			panic(err)
		}
		batch.tx = tx
	}
	b, err := batch.tx.CreateBucketIfNotExists(bucket, false)
	if err != nil {
		panic(err)
	}
	err = b.Put(k, v)
	if err != nil {
		panic(err)
	}
}

func (kv *BoltKVStorage) SetCode(hash common.Hash, code []byte) {
	kv.Put(append(CODE, hash.Bytes()...), code)
}

func (kv *BoltKVStorage) GetCode(hash common.Hash) ([]byte, bool) {
	return kv.Get(append(CODE, hash.Bytes()...))
}

func (batch *BoltKVBatch) Write() {
	if batch.tx != nil {
		if err := batch.tx.Commit(); err != nil {
			panic(err)
		}
		batch.tx = nil
	}
}

func (kv *BoltKVStorage) Batch() Batch {
	return &BoltKVBatch{db: kv.db}
}

func (kv *BoltKVStorage) Put(k, v []byte) {
	err := kv.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket, false)
		if err != nil {
			return err
		}
		return b.Put(k, v)
	})
	if err != nil {
		panic(err)
	}
}

func (kv *BoltKVStorage) Get(k []byte) ([]byte, bool) {
	var data []byte
	var found bool
	err := kv.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b != nil {
			if v, _ := b.Get(k); v != nil { // second argument is "rank"
				// v is only valid for the lifetime of the tx, therefore copying
				data = make([]byte, len(v))
				copy(data, v)
				found = true
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return data, found
}

func NewBoltDBStorage(path string, logger hclog.Logger) (Storage, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{})
	if err != nil {
		return nil, err
	}
	return &BoltKVStorage{db}, nil
}
