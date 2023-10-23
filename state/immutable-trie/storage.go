package itrie

import (
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/fastrlp"
)

var parserPool fastrlp.ParserPool

var (
	// codePrefix is the code prefix for leveldb
	codePrefix = []byte("code")

	// leveldb not found error message
	levelDBNotFoundMsg = "leveldb: not found"
)

// Batch is batch write interface
type Batch interface {
	// Put puts key and value into batch. It can not return error because actual writing is done with Write method
	Put(k, v []byte)
	// Write writes all the key values pair previosly putted with Put method to the database
	Write() error
}

// Storage stores the trie
type Storage interface {
	Put(k, v []byte) error
	Get(k []byte) ([]byte, bool, error)
	Batch() Batch
	SetCode(hash types.Hash, code []byte) error
	GetCode(hash types.Hash) ([]byte, bool)

	Close() error
}

// KVStorage is a k/v storage on memory using leveldb
type KVStorage struct {
	db *leveldb.DB
}

// KVBatch is a batch write for leveldb
type KVBatch struct {
	db    *leveldb.DB
	batch *leveldb.Batch
}

func (b *KVBatch) Put(k, v []byte) {
	b.batch.Put(k, v)
}

func (b *KVBatch) Write() error {
	return b.db.Write(b.batch, nil)
}

func (kv *KVStorage) SetCode(hash types.Hash, code []byte) error {
	return kv.Put(GetCodeKey(hash), code)
}

func (kv *KVStorage) GetCode(hash types.Hash) ([]byte, bool) {
	res, ok, err := kv.Get(GetCodeKey(hash))
	if err != nil {
		return nil, false
	}

	return res, ok
}

func (kv *KVStorage) Batch() Batch {
	return &KVBatch{db: kv.db, batch: &leveldb.Batch{}}
}

func (kv *KVStorage) Put(k, v []byte) error {
	return kv.db.Put(k, v, nil)
}

func (kv *KVStorage) Get(k []byte) ([]byte, bool, error) {
	data, err := kv.db.Get(k, nil)
	if err != nil {
		if err.Error() == levelDBNotFoundMsg {
			return nil, false, nil
		}

		return nil, false, err
	}

	return data, true, nil
}

func (kv *KVStorage) Close() error {
	return kv.db.Close()
}

func NewLevelDBStorage(path string, logger hclog.Logger) (Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}

	return &KVStorage{db}, nil
}

type memStorage struct {
	l    *sync.Mutex
	db   map[string][]byte
	code map[string][]byte
}

type memBatch struct {
	l  *sync.Mutex
	db *map[string][]byte
}

// NewMemoryStorage creates an inmemory trie storage
func NewMemoryStorage() Storage {
	return &memStorage{db: map[string][]byte{}, code: map[string][]byte{}, l: new(sync.Mutex)}
}

func (m *memStorage) Put(p []byte, v []byte) error {
	m.l.Lock()
	defer m.l.Unlock()

	buf := make([]byte, len(v))
	copy(buf[:], v[:])
	m.db[hex.EncodeToHex(p)] = buf

	return nil
}

func (m *memStorage) Get(p []byte) ([]byte, bool, error) {
	m.l.Lock()
	defer m.l.Unlock()

	v, ok := m.db[hex.EncodeToHex(p)]
	if !ok {
		return []byte{}, false, nil
	}

	return v, true, nil
}

func (m *memStorage) SetCode(hash types.Hash, code []byte) error {
	return m.Put(append(codePrefix, hash.Bytes()...), code)
}

func (m *memStorage) GetCode(hash types.Hash) ([]byte, bool) {
	res, ok, err := m.Get(GetCodeKey(hash))
	if err != nil {
		return nil, false
	}

	return res, ok
}

func (m *memStorage) Batch() Batch {
	return &memBatch{db: &m.db, l: new(sync.Mutex)}
}

func (m *memStorage) Close() error {
	return nil
}

func (m *memBatch) Put(p, v []byte) {
	m.l.Lock()
	defer m.l.Unlock()

	buf := make([]byte, len(v))
	copy(buf[:], v[:])
	(*m.db)[hex.EncodeToHex(p)] = buf
}

func (m *memBatch) Write() error {
	return nil
}

// GetNode retrieves a node from storage
func GetNode(root []byte, storage Storage) (Node, bool, error) {
	data, ok, err := storage.Get(root)
	if err != nil || !ok || len(data) == 0 {
		return nil, false, err
	}

	// NOTE. We dont need to make copies of the bytes because the nodes
	// take the reference from data itself which is a safe copy.
	p := parserPool.Get()
	defer parserPool.Put(p)

	v, err := p.Parse(data)
	if err != nil {
		return nil, false, err
	}

	if v.Type() != fastrlp.TypeArray {
		return nil, false, fmt.Errorf("storage item should be an array")
	}

	n, err := decodeNode(v, storage)

	return n, err == nil, err
}

func decodeNode(v *fastrlp.Value, s Storage) (Node, error) {
	if v.Type() == fastrlp.TypeBytes {
		vv := &ValueNode{
			hash: true,
		}
		vv.buf = append(vv.buf[:0], v.Raw()...)

		return vv, nil
	}

	var err error

	ll := v.Elems()
	if ll == 2 {
		key := v.Get(0)
		if key.Type() != fastrlp.TypeBytes {
			return nil, fmt.Errorf("short key expected to be bytes")
		}

		// this can be either an array (extension node)
		// or bytes (leaf node)
		nc := &ShortNode{}
		nc.key = decodeCompact(key.Raw())

		if hasTerminator(nc.key) {
			// value node
			if v.Get(1).Type() != fastrlp.TypeBytes {
				return nil, fmt.Errorf("short leaf value expected to be bytes")
			}

			vv := &ValueNode{}
			vv.buf = append(vv.buf, v.Get(1).Raw()...)
			nc.child = vv
		} else {
			nc.child, err = decodeNode(v.Get(1), s)
			if err != nil {
				return nil, err
			}
		}

		return nc, nil
	} else if ll == 17 {
		// full node
		nc := &FullNode{}
		for i := 0; i < 16; i++ {
			if v.Get(i).Type() == fastrlp.TypeBytes && len(v.Get(i).Raw()) == 0 {
				// empty
				continue
			}
			nc.children[i], err = decodeNode(v.Get(i), s)
			if err != nil {
				return nil, err
			}
		}

		if v.Get(16).Type() != fastrlp.TypeBytes {
			return nil, fmt.Errorf("full node value expected to be bytes")
		}
		if len(v.Get(16).Raw()) != 0 {
			vv := &ValueNode{}
			vv.buf = append(vv.buf[:0], v.Get(16).Raw()...)
			nc.value = vv
		}

		return nc, nil
	}

	return nil, fmt.Errorf("node has incorrect number of leafs")
}

func GetCodeKey(hash types.Hash) []byte {
	return append(codePrefix, hash.Bytes()...)
}
