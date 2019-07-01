package itrie

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/minimal/helper/hex"
	"github.com/umbracle/minimal/rlpv2"
	"github.com/umbracle/minimal/types"
)

var parserPool rlpv2.ParserPool

var (
	// codePrefix is the code prefix for leveldb
	codePrefix = []byte("code")
)

type Batch interface {
	Put(k, v []byte)
	Write()
}

// Storage stores the trie
type Storage interface {
	Put(k, v []byte)
	Get(k []byte) ([]byte, bool)
	Batch() Batch
	SetCode(hash types.Hash, code []byte)
	GetCode(hash types.Hash) ([]byte, bool)
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

func (b *KVBatch) Write() {
	b.db.Write(b.batch, nil)
}

func (kv *KVStorage) SetCode(hash types.Hash, code []byte) {
	kv.Put(append(codePrefix, hash.Bytes()...), code)
}

func (kv *KVStorage) GetCode(hash types.Hash) ([]byte, bool) {
	return kv.Get(append(codePrefix, hash.Bytes()...))
}

func (kv *KVStorage) Batch() Batch {
	return &KVBatch{db: kv.db, batch: &leveldb.Batch{}}
}

func (kv *KVStorage) Put(k, v []byte) {
	kv.db.Put(k, v, nil)
}

func (kv *KVStorage) Get(k []byte) ([]byte, bool) {
	data, err := kv.db.Get(k, nil)
	if err != nil {
		if err.Error() == "leveldb: not found" {
			return nil, false
		} else {
			panic(err)
		}
	}
	return data, true
}

func NewLevelDBStorage(path string, logger hclog.Logger) (Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &KVStorage{db}, nil
}

type memStorage struct {
	db   map[string][]byte
	code map[string][]byte
}

type memBatch struct {
	db *map[string][]byte
}

// NewMemoryStorage creates an inmemory trie storage
func NewMemoryStorage() Storage {
	return &memStorage{db: map[string][]byte{}, code: map[string][]byte{}}
}

func (m *memStorage) Put(p []byte, v []byte) {
	buf := make([]byte, len(v))
	copy(buf[:], v[:])
	m.db[hex.EncodeToHex(p)] = buf
}

func (m *memStorage) Get(p []byte) ([]byte, bool) {
	v, ok := m.db[hex.EncodeToHex(p)]
	if !ok {
		return []byte{}, false
	}
	return v, true
}

func (m *memStorage) SetCode(hash types.Hash, code []byte) {
	m.code[hash.String()] = code
}

func (m *memStorage) GetCode(hash types.Hash) ([]byte, bool) {
	code, ok := m.code[hash.String()]
	return code, ok
}

func (m *memStorage) Batch() Batch {
	return &memBatch{db: &m.db}
}

func (m *memBatch) Put(p, v []byte) {
	buf := make([]byte, len(v))
	copy(buf[:], v[:])
	(*m.db)[hex.EncodeToHex(p)] = buf
}

func (m *memBatch) Write() {
}

// GetNode retrieves a node from storage
func GetNode(root []byte, storage Storage) (Node, bool, error) {
	data, ok := storage.Get(root)
	if !ok {
		return nil, false, nil
	}

	// NOTE. We dont need to make copies of the bytes because the nodes
	// take the reference from data itself which is a safe copy.
	p := parserPool.Get()
	defer parserPool.Put(p)

	v, err := p.Parse(data)
	if err != nil {
		return nil, false, err
	}

	if v.Type() != rlpv2.TypeArray {
		return nil, false, fmt.Errorf("storage item should be an array")
	}

	n, err := decodeNode(v, storage)
	return n, err == nil, err
}

func decodeNode(v *rlpv2.Value, s Storage) (Node, error) {
	if v.Type() == rlpv2.TypeBytes {
		return &ValueNode{
			buf:  v.Bytes(),
			hash: true,
		}, nil
	}

	var err error

	ll := v.Elems()
	if ll == 2 {
		key := v.Get(0)
		if key.Type() != rlpv2.TypeBytes {
			return nil, fmt.Errorf("short key expected to be bytes")
		}

		// this can be either an array (extension node)
		// or bytes (leaf node)
		nc := &ShortNode{}
		nc.key = compactToHex(key.Bytes())
		if hasTerm(nc.key) {
			// value node
			if v.Get(1).Type() != rlpv2.TypeBytes {
				return nil, fmt.Errorf("short leaf value expected to be bytes")
			}
			vv := &ValueNode{
				buf: v.Get(1).Bytes(),
			}
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
			if v.Get(i).Type() == rlpv2.TypeBytes && len(v.Get(i).Bytes()) == 0 {
				// empty
				continue
			}
			nc.children[i], err = decodeNode(v.Get(i), s)
			if err != nil {
				return nil, err
			}
		}

		if v.Get(16).Type() != rlpv2.TypeBytes {
			return nil, fmt.Errorf("full node value expected to be bytes")
		}
		if len(v.Get(16).Bytes()) != 0 {
			vv := &ValueNode{
				buf: v.Get(16).Bytes(),
			}
			nc.value = vv
		}
		return nc, nil
	}
	return nil, fmt.Errorf("node has incorrect number of leafs")
}
