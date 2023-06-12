package itrie

import (
	"bytes"
	"fmt"

	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"

	commonHelpers "github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// Node represents a node reference
type Node interface {
	Hash() ([]byte, bool)
	SetHash(b []byte) []byte
}

// ValueNode is a leaf on the merkle-trie
type ValueNode struct {
	// hash marks if this value node represents a stored node
	hash bool
	buf  []byte
}

// Hash implements the node interface
func (v *ValueNode) Hash() ([]byte, bool) {
	return v.buf, v.hash
}

// SetHash implements the node interface
func (v *ValueNode) SetHash(b []byte) []byte {
	panic("We cannot set hash on value node") //nolint:gocritic
}

type common struct {
	hash []byte
}

// Hash implements the node interface
func (c *common) Hash() ([]byte, bool) {
	return c.hash, len(c.hash) != 0
}

// SetHash implements the node interface
func (c *common) SetHash(b []byte) []byte {
	c.hash = commonHelpers.ExtendByteSlice(c.hash, len(b))
	copy(c.hash, b)

	return c.hash
}

// ShortNode is an extension or short node
type ShortNode struct {
	common
	key   []byte
	child Node
}

// FullNode is a node with several children
type FullNode struct {
	common
	epoch    uint32
	value    Node
	children [16]Node
}

func (f *FullNode) copy() *FullNode {
	nc := &FullNode{}
	nc.value = f.value
	copy(nc.children[:], f.children[:])

	return nc
}

func (f *FullNode) setEdge(idx byte, e Node) {
	if idx == 16 {
		f.value = e
	} else {
		f.children[idx] = e
	}
}

func (f *FullNode) getEdge(idx byte) Node {
	if idx == 16 {
		return f.value
	} else {
		return f.children[idx]
	}
}

type Trie struct {
	root  Node
	epoch uint32
}

func NewTrie() *Trie {
	return &Trie{}
}

func (t *Trie) Get(k []byte, storage Storage) ([]byte, bool) {
	txn := t.Txn(storage)
	res := txn.Lookup(k)

	return res, res != nil
}

func hashit(k []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(k)

	return h.Sum(nil)
}

var (
	stateArenaPool fastrlp.ArenaPool
)

// Hash returns the root hash of the trie. It does not write to the
// database and can be used even if the trie doesn't have one.
func (t *Trie) Hash() types.Hash {
	if t.root == nil {
		return types.EmptyRootHash
	}

	hash := t.hashRoot()

	return types.BytesToHash(hash)
}

func (t *Trie) hashRoot() []byte {
	hash, _ := t.root.Hash()

	return hash
}

func (t *Trie) Txn(storage Storage) *Txn {
	return &Txn{root: t.root, epoch: t.epoch + 1, storage: storage}
}

type Putter interface {
	Put(k, v []byte)
}

type Txn struct {
	root    Node
	epoch   uint32
	storage Storage
	batch   Putter
}

func (t *Txn) Commit() *Trie {
	return &Trie{epoch: t.epoch, root: t.root}
}

func (t *Txn) Lookup(key []byte) []byte {
	_, res := t.lookup(t.root, bytesToHexNibbles(key))

	return res
}

func (t *Txn) lookup(node interface{}, key []byte) (Node, []byte) {
	switch n := node.(type) {
	case nil:
		return nil, nil

	case *ValueNode:
		if n.hash {
			nc, ok, err := GetNode(n.buf, t.storage)
			if err != nil {
				panic(err) //nolint:gocritic
			}

			if !ok {
				return nil, nil
			}

			_, res := t.lookup(nc, key)

			return nc, res
		}

		if len(key) == 0 {
			return nil, n.buf
		} else {
			return nil, nil
		}

	case *ShortNode:
		plen := len(n.key)
		if plen > len(key) || !bytes.Equal(key[:plen], n.key) {
			return nil, nil
		}

		child, res := t.lookup(n.child, key[plen:])

		if child != nil {
			n.child = child
		}

		return nil, res

	case *FullNode:
		if len(key) == 0 {
			return t.lookup(n.value, key)
		}

		child, res := t.lookup(n.getEdge(key[0]), key[1:])

		if child != nil {
			n.children[key[0]] = child
		}

		return nil, res

	default:
		panic(fmt.Sprintf("unknown node type %v", n)) //nolint:gocritic
	}
}

func (t *Txn) writeNode(n *FullNode) *FullNode {
	if t.epoch == n.epoch {
		return n
	}

	nc := &FullNode{
		epoch: t.epoch,
		value: n.value,
	}
	copy(nc.children[:], n.children[:])

	return nc
}

func (t *Txn) Insert(key, value []byte) {
	root := t.insert(t.root, bytesToHexNibbles(key), value)
	if root != nil {
		t.root = root
	}
}

func (t *Txn) insert(node Node, search, value []byte) Node {
	switch n := node.(type) {
	case nil:
		// NOTE, this only happens with the full node
		if len(search) == 0 {
			v := &ValueNode{}
			v.buf = make([]byte, len(value))
			copy(v.buf, value)

			return v
		} else {
			return &ShortNode{
				key:   search,
				child: t.insert(nil, nil, value),
			}
		}

	case *ValueNode:
		if n.hash {
			nc, ok, err := GetNode(n.buf, t.storage)
			if err != nil {
				panic(err) //nolint:gocritic
			}

			if !ok {
				return nil
			}

			node = nc

			return t.insert(node, search, value)
		}

		if len(search) == 0 {
			v := &ValueNode{}
			v.buf = make([]byte, len(value))
			copy(v.buf, value)

			return v
		} else {
			b := t.insert(&FullNode{epoch: t.epoch, value: n}, search, value)

			return b
		}

	case *ShortNode:
		plen := prefixLen(search, n.key)
		if plen == len(n.key) {
			// Keep this node as is and insert to child
			child := t.insert(n.child, search[plen:], value)

			return &ShortNode{key: n.key, child: child}
		} else {
			// Introduce a new branch
			b := FullNode{epoch: t.epoch}
			if len(n.key) > plen+1 {
				b.setEdge(n.key[plen], &ShortNode{key: n.key[plen+1:], child: n.child})
			} else {
				b.setEdge(n.key[plen], n.child)
			}

			child := t.insert(&b, search[plen:], value)

			if plen == 0 {
				return child
			} else {
				return &ShortNode{key: search[:plen], child: child}
			}
		}

	case *FullNode:
		b := t.writeNode(n)

		if len(search) == 0 {
			b.value = t.insert(b.value, nil, value)

			return b
		} else {
			k := search[0]
			child := n.getEdge(k)
			newChild := t.insert(child, search[1:], value)
			b.setEdge(k, newChild)

			return b
		}

	default:
		panic(fmt.Sprintf("unknown node type %v", n)) //nolint:gocritic
	}
}

func (t *Txn) Delete(key []byte) {
	root, ok := t.delete(t.root, bytesToHexNibbles(key))
	if ok {
		t.root = root
	}
}

func (t *Txn) delete(node Node, search []byte) (Node, bool) {
	switch n := node.(type) {
	case nil:
		return nil, false

	case *ShortNode:
		n.hash = n.hash[:0]

		plen := prefixLen(search, n.key)
		if plen == len(search) {
			return nil, true
		}

		if plen == 0 {
			return nil, false
		}

		child, ok := t.delete(n.child, search[plen:])
		if !ok {
			return nil, false
		}

		if child == nil {
			return nil, true
		}

		if short, ok := child.(*ShortNode); ok {
			// merge nodes
			return &ShortNode{key: concat(n.key, short.key), child: short.child}, true
		} else {
			// full node
			return &ShortNode{key: n.key, child: child}, true
		}

	case *ValueNode:
		if n.hash {
			nc, ok, err := GetNode(n.buf, t.storage)
			if err != nil {
				panic(err) //nolint:gocritic
			}

			if !ok {
				return nil, false
			}

			return t.delete(nc, search)
		}

		if len(search) != 0 {
			return nil, false
		}

		return nil, true

	case *FullNode:
		n = n.copy()
		n.hash = n.hash[:0]

		key := search[0]
		newChild, ok := t.delete(n.getEdge(key), search[1:])

		if !ok {
			return nil, false
		}

		n.setEdge(key, newChild)

		indx := -1

		var notEmpty bool

		for edge, i := range n.children {
			if i != nil {
				if indx != -1 {
					notEmpty = true

					break
				} else {
					indx = edge
				}
			}
		}

		if indx != -1 && n.value != nil {
			// We have one children and value, set notEmpty to true
			notEmpty = true
		}

		if notEmpty {
			// The full node still has some other values
			return n, true
		}

		if indx == -1 {
			// There are no children nodes
			if n.value == nil {
				// Everything is empty, return nil
				return nil, true
			}
			// The value is the only left, return a short node with it
			return &ShortNode{key: []byte{0x10}, child: n.value}, true
		}

		// Only one value left at indx
		nc := n.children[indx]

		if vv, ok := nc.(*ValueNode); ok && vv.hash {
			// If the value is a hash, we have to resolve it first.
			// This needs better testing
			aux, ok, err := GetNode(vv.buf, t.storage)
			if err != nil {
				panic(err) //nolint:gocritic
			}

			if !ok {
				return nil, false
			}

			nc = aux
		}

		obj, ok := nc.(*ShortNode)
		if !ok {
			obj := &ShortNode{}
			obj.key = []byte{byte(indx)}
			obj.child = nc

			return obj, true
		}

		ncc := &ShortNode{}
		ncc.key = concat([]byte{byte(indx)}, obj.key)
		ncc.child = obj.child

		return ncc, true
	}

	panic("it should not happen") //nolint:gocritic
}

func prefixLen(k1, k2 []byte) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}

	var i int

	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}

	return i
}

func concat(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)

	return c
}
