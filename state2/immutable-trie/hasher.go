package itrie

import (
	"errors"
	"fmt"
	"hash"
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
)

var arenaPool fastrlp.ArenaPool

var (
	emptyRoot = types.StringToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421").Bytes()
)

var hasherPool = sync.Pool{
	New: func() interface{} {
		impl, ok := sha3.NewLegacyKeccak256().(hashImpl)
		if !ok {
			return nil
		}

		return &hasher{
			hash: impl,
		}
	},
}

type hashImpl interface {
	hash.Hash
	Read([]byte) (int, error)
}

type hasher struct {
	arena []*fastrlp.Arena
	buf   []byte
	hash  hashImpl
	tmp   [32]byte
}

func (h *hasher) Reset() {
	h.buf = h.buf[:0]
	h.hash.Reset()
	h.tmp = [32]byte{}
}

func (h *hasher) ReleaseArenas(idx int) {
	for i := idx; i < len(h.arena); i++ {
		arenaPool.Put(h.arena[i])
	}

	h.arena = h.arena[:idx]
}

func (h *hasher) ReleaseArena(a *fastrlp.Arena) {
	a.Reset()
	arenaPool.Put(a)
}

func (h *hasher) AcquireArena() (*fastrlp.Arena, int) {
	v := arenaPool.Get()
	idx := len(h.arena)
	h.arena = append(h.arena, v)

	return v, idx
}

func (h *hasher) Hash(data []byte) []byte {
	h.hash.Reset()
	h.hash.Write(data)
	n, err := h.hash.Read(h.tmp[:])

	if err != nil {
		panic(err)
	}

	if n != 32 {
		panic("incorrect length")
	}

	return h.tmp[:]
}

func (t *Txn) Hash() ([]byte, error) {
	if t.root == nil {
		return emptyRoot, nil
	}

	h, ok := hasherPool.Get().(*hasher)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	var root []byte

	arena, _ := h.AcquireArena()
	val := t.hash(t.root, h, arena, 0)

	// REDO
	if val.Type() == fastrlp.TypeBytes {
		if val.Len() != 32 {
			h.hash.Reset()
			h.hash.Write(val.Raw())

			root = h.hash.Sum(nil)

			if t.batch != nil {
				t.batch.Put(root, val.Raw())
			}
		} else {
			root = make([]byte, 32)
			copy(root, val.Raw())
		}
	} else {
		tmp := val.MarshalTo(nil)
		h.hash.Reset()
		h.hash.Write(tmp)

		root = h.hash.Sum(nil)

		if t.batch != nil {
			t.batch.Put(root, tmp)
		}
	}

	h.ReleaseArenas(0)
	hasherPool.Put(h)

	return root, nil
}

func (t *Txn) hash(node Node, h *hasher, a *fastrlp.Arena, d int) *fastrlp.Value {
	var val *fastrlp.Value

	var aa *fastrlp.Arena

	var idx int

	if h, ok := node.Hash(); ok {
		return a.NewCopyBytes(h)
	}

	switch n := node.(type) {
	case *ValueNode:
		return a.NewCopyBytes(n.buf)

	case *ShortNode:
		child := t.hash(n.child, h, a, d+1)

		val = a.NewArray()
		val.Set(a.NewBytes(encodeCompact(n.key)))
		val.Set(child)

	case *FullNode:
		val = a.NewArray()

		aa, idx = h.AcquireArena()

		for _, i := range n.children {
			if i == nil {
				val.Set(a.NewNull())
			} else {
				val.Set(t.hash(i, h, aa, d+1))
			}
		}

		// Add the value
		if n.value == nil {
			val.Set(a.NewNull())
		} else {
			val.Set(t.hash(n.value, h, a, d+1))
		}

	default:
		panic(fmt.Sprintf("unknown node type %v", n))
	}

	if val.Len() < 32 {
		return val
	}

	// marshal RLP value
	h.buf = val.MarshalTo(h.buf[:0])

	if aa != nil {
		h.ReleaseArenas(idx)
	}

	tmp := h.Hash(h.buf)
	hh := node.SetHash(tmp)

	// Write data
	if t.batch != nil {
		t.batch.Put(tmp, h.buf)
	}

	return a.NewCopyBytes(hh)
}
