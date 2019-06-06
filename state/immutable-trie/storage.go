// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/helper/hex"
	"github.com/umbracle/minimal/types"
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

func (kv *KVStorage) SetCode(hash types.Hash, code []byte) {
	kv.Put(append(CODE, hash.Bytes()...), code)
}

func (kv *KVStorage) GetCode(hash types.Hash) ([]byte, bool) {
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

func NewLevelDBStorage(path string, logger hclog.Logger) (Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &KVStorage{db}, nil
}

type memStorage struct {
	db   map[string][]byte
	code map[types.Hash][]byte
}

type memBatch struct {
	db *map[string][]byte
}

func (m *memBatch) Put(p, v []byte) {
	(*m.db)[hex.EncodeToHex(p)] = v
}

func (m *memBatch) Write() {
}

// NewMemoryStorage creates an inmemory trie storage
func NewMemoryStorage() Storage {
	return &memStorage{db: map[string][]byte{}, code: map[types.Hash][]byte{}}
}

func (m *memStorage) SetCode(hash types.Hash, code []byte) {
	m.code[hash] = code
}

func (m *memStorage) GetCode(hash types.Hash) ([]byte, bool) {
	code, ok := m.code[hash]
	return code, ok
}

func (m *memStorage) Batch() Batch {
	return &memBatch{db: &m.db}
}

func (m *memStorage) Put(p []byte, v []byte) {
	m.db[hex.EncodeToHex(p)] = v
}

func (m *memStorage) Get(p []byte) ([]byte, bool) {
	v, ok := m.db[hex.EncodeToHex(p)]
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

	i := rlp.NewIterator(data)
	if _, err := i.List(); err != nil {
		return nil, err
	}

	count, err := rlp.NewIterator(data).Count()
	if err != nil {
		return nil, err
	}

	elems := i.Raw()
	if count == 2 {
		return decodeShort(storage, hash, elems)
	} else if count == 17 {
		return decodeFull(storage, hash, elems)
	}

	return nil, fmt.Errorf("invalid number of list elements: %v", count)
}

func decodeShort(storage Storage, hash []byte, data []byte) (*Node, error) {
	i := rlp.NewIterator(data)

	kbuf, err := i.Bytes()
	if err != nil {
		panic(err)
	}
	key := compactToHex(kbuf)

	if hasTerm(key) {
		// value node
		val, err := i.Bytes()
		if err != nil {
			panic(err)
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

	rest := i.Raw()
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

	i := rlp.NewIterator(data)
	val, err := i.Bytes()
	if err != nil {
		panic(err)
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

const hashLen = len(types.Hash{})

func decodeRef(storage Storage, hash []byte, buf []byte) (*Node, []byte, error) {

	i := rlp.NewIterator(buf)
	isList, err := i.IsListNext()
	if err != nil {
		panic(err)
	}

	if isList {
		if !isList {
			panic("YY")
		}

		size, err := i.List()
		if err != nil {
			panic(err)
		}
		size = size + 1

		// 'embedded' node
		if int(size) > hashLen {
			err := fmt.Errorf("oversized embedded node (size is %d bytes, want size < %d)", size, hashLen)
			return nil, buf, err
		}
		n, err := DecodeNode(storage, hash, buf)

		rest := buf[size:]
		return n, rest, err
	}

	val, err := i.Bytes()
	if err != nil {
		panic(err)
	}

	if len(val) == 0 {
		return nil, i.Raw(), nil
	}
	if len(val) != 32 {
		panic("XX")
	}

	realVal, ok := storage.Get(val)
	if !ok {
		return nil, nil, fmt.Errorf("Value could not be expanded")
	}

	n, err := DecodeNode(storage, hash, realVal)
	n.hash = val

	return n, i.Raw(), err
}
