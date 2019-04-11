package trie

import (
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// CODE is the code prefix
	CODE = []byte("code")
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
	SetCode(hash common.Hash, code []byte)
	GetCode(hash common.Hash) ([]byte, bool)
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

func (kv *KVStorage) SetCode(hash common.Hash, code []byte) {
	kv.Put(append(CODE, hash.Bytes()...), code)
}

func (kv *KVStorage) GetCode(hash common.Hash) ([]byte, bool) {
	return kv.Get(append(CODE, hash.Bytes()...))
}

func (b *KVBatch) Write() {
	b.db.Write(b.batch, nil)
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

func NewLevelDBStorage(path string, logger *log.Logger) (Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	return &KVStorage{db}, nil
}

type memStorage struct {
	db   map[string][]byte
	code map[common.Hash][]byte
}

type memBatch struct {
	db *map[string][]byte
}

func (m *memBatch) Put(p, v []byte) {
	(*m.db)[hexutil.Encode(p)] = v
}

func (m *memBatch) Write() {
}

// NewMemoryStorage creates an inmemory trie storage
func NewMemoryStorage() Storage {
	return &memStorage{db: map[string][]byte{}, code: map[common.Hash][]byte{}}
}

func (m *memStorage) SetCode(hash common.Hash, code []byte) {
	m.code[hash] = code
}

func (m *memStorage) GetCode(hash common.Hash) ([]byte, bool) {
	code, ok := m.code[hash]
	return code, ok
}

func (m *memStorage) Batch() Batch {
	return &memBatch{db: &m.db}
}

func (m *memStorage) Put(p []byte, v []byte) {
	m.db[hexutil.Encode(p)] = v
}

func (m *memStorage) Get(p []byte) ([]byte, bool) {
	v, ok := m.db[hexutil.Encode(p)]
	if !ok {
		return []byte{}, false
	}
	return v, true
}

// DecodeNode decodes bytes to his node representation
func DecodeNode(storage Storage, hash []byte, data []byte) (*Node, error) {
	// empty data, just return the node, not sure if this is a special case
	if len(data) == 0 {
		return &Node{}, nil
	}

	elems, _, err := rlp.SplitList(data)
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}

	count, _ := rlp.CountValues(elems)
	if count == 2 {
		return decodeShort(storage, hash, elems)
	} else if count == 17 {
		return decodeFull(storage, hash, elems)
	}

	return nil, fmt.Errorf("invalid number of list elements: %v", count)
}

func decodeShort(storage Storage, hash []byte, data []byte) (*Node, error) {
	kbuf, rest, err := rlp.SplitString(data)
	if err != nil {
		return nil, err
	}
	key := compactToHex(kbuf)

	if hasTerm(key) {
		// value node
		val, _, err := rlp.SplitString(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid value node: %v", err)
		}

		h := make([]byte, len(hash))
		copy(h[:], hash[:])

		n := &Node{
			prefix: key,
			leaf: &leafNode{
				key: append(h, key...),
				val: val,
			},
		}
		return n, nil
	}
	r, _, err := decodeRef(storage, append(hash, key...), rest)
	if err != nil {
		return nil, err
	}
	r.prefix = key
	return r, nil
}

func decodeFull(storage Storage, hash []byte, data []byte) (*Node, error) {
	edges := [17]*Node{}

	h := make([]byte, len(hash))
	copy(h[:], hash[:])

	for i := 0; i < 16; i++ {
		n, rest, err := decodeRef(storage, append(hash, []byte{byte(i)}...), data)
		if err != nil {
			return nil, err
		}
		if n != nil {
			n.prefix = append([]byte{byte(i)}, n.prefix...)
		}
		edges[i] = n
		data = rest
	}

	// value node
	val, _, err := rlp.SplitString(data)
	if err != nil {
		return nil, err
	}
	if len(val) > 0 {
		edges[16] = &Node{
			prefix: []byte{0x10},
			leaf: &leafNode{
				key: append(h, []byte{0x10}...),
				val: val,
			},
		}
	}

	n := &Node{
		edges: edges,
	}
	return n, nil
}

const hashLen = len(common.Hash{})

func decodeRef(storage Storage, hash []byte, buf []byte) (*Node, []byte, error) {
	kind, val, rest, err := rlp.Split(buf)
	if err != nil {
		return nil, buf, err
	}
	switch {
	case kind == rlp.List:
		// 'embedded' node
		if size := len(buf) - len(rest); size > hashLen {
			err := fmt.Errorf("oversized embedded node (size is %d bytes, want size < %d)", size, hashLen)
			return nil, buf, err
		}
		n, err := DecodeNode(storage, hash, buf)
		return n, rest, err
	case kind == rlp.String && len(val) == 0:
		// empty node
		return nil, rest, nil
	case kind == rlp.String && len(val) == 32:

		// NOTE, it fully expands all the internal nodes
		// only for testing now.

		realVal, ok := storage.Get(val)
		if !ok {
			return nil, nil, fmt.Errorf("Value could not be expanded")
		}

		n, err := DecodeNode(storage, hash, realVal)

		// fmt.Println("-- val --")
		// fmt.Println(val)

		n.hash = val
		return n, rest, err

		// return &Node{leaf: &leafNode{val: val}}, rest, nil
	default:
		return nil, nil, fmt.Errorf("invalid RLP string size %d (want 0 or 32)", len(val))
	}
}
