package trie

import (
	"bytes"
	"fmt"
	"hash"
	"reflect"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421").Bytes()
)

// WalkFn is used when walking the tree. Takes a
// key and value, returning if iteration should
// be terminated.
type WalkFn func(k []byte, v []byte) bool

type WalkFnNode func(v *Node) bool

type leafNode struct {
	key []byte
	val []byte
}

type edge struct {
	label byte
	node  *Node
}

type Node struct {
	leaf   *leafNode
	prefix []byte
	edges  [17]*Node
	hash   []byte
}

func (n *Node) Equal(nn *Node) bool {
	nVals := []*Node{}
	nnVals := []*Node{}

	n.WalkNode(func(n *Node) bool {
		n.hash = nil // dont compare the hashed cache
		nVals = append(nVals, n)
		return false
	})
	nn.WalkNode(func(n *Node) bool {
		n.hash = nil
		nnVals = append(nnVals, n)
		return false
	})

	if len(nVals) != len(nnVals) {
		return false
	}

	for indx, v := range nVals {
		v1 := nnVals[indx]
		if !reflect.DeepEqual(v, v1) {
			return false
		}
	}
	return true
}

func (n *Node) isLeaf() bool {
	return n.leaf != nil
}

func (n *Node) isShort() bool {
	return len(n.edges) == 0 && n.leaf != nil
}

func (n *Node) addEdge(e edge) {
	n.edges[e.label] = e.node
}

func (n *Node) replaceEdge(e edge) {
	if n.edges[e.label] == nil {
		panic("replacing missing edge")
	}
	n.edges[e.label] = e.node
}

func (n *Node) getEdge(label byte) (int, *Node) {
	if n.edges[label] == nil {
		return -1, nil
	}
	return int(label), n.edges[label]
}

// Walk is used to walk the tree
func (n *Node) Walk(fn WalkFn) {
	recursiveWalk(n, fn)
}

// recursiveWalk is used to do a pre-order walk of a node
// recursively. Returns true if the walk should be aborted
func recursiveWalk(n *Node, fn WalkFn) bool {
	// Visit the leaf values if any
	if n.leaf != nil && fn(n.leaf.key, n.leaf.val) {
		return true
	}

	// Recurse on the children
	for _, e := range n.edges {
		if e != nil {
			if recursiveWalk(e, fn) {
				return true
			}
		}
	}
	return false
}

func (n *Node) WalkNode(fn WalkFnNode) {
	recursiveWalkNode(n, fn)
}

func recursiveWalkNode(n *Node, fn WalkFnNode) bool {
	// Visit the leaf values if any
	if fn(n) {
		return true
	}

	// Recurse on the children
	for _, e := range n.edges {
		if e != nil {
			if recursiveWalkNode(e, fn) {
				return true
			}
		}
	}
	return false
}

func (n *Node) Get(k []byte) ([]byte, bool) {
	val, ok := n.get(k)
	return val, ok
}

func (n *Node) get(k []byte) ([]byte, bool) {
	search := k
	if n.prefix != nil {
		if bytes.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			return nil, false
		}
	}

	for {
		// Check for key exhaustion
		if len(search) == 0 {
			if n.isLeaf() {
				return n.leaf.val, true
			}
			break
		}

		// Look for an edge
		_, n = n.getEdge(search[0])
		if n == nil {
			break
		}
		// Consume the search prefix
		if bytes.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}
	return nil, false
}

// First returns the first element
func (n *Node) First() *Node {
	for _, i := range n.edges {
		if i != nil {
			return i
		}
	}
	return nil
}

// Len returns the number of edges in the node
func (n *Node) Len() int {
	l := 0
	for _, i := range n.edges {
		if i != nil {
			l++
		}
	}
	return l
}

type KVWriter interface {
	Put(k, v []byte)
}

func (n *Node) Hash(storage KVWriter) []byte {
	var x []byte
	var ok bool

	size := n.Len()

	hasher := newHasher()
	// hasher := hasherPool.Get().(*Hasher)

	if size == 0 {
		if storage != nil {
			storage.Put(emptyRoot, []byte{})
		}
		return emptyRoot
	} else if size == 1 { // only one short node
		// its a short node
		x, ok = hasher.Hash(storage, n.First(), 0, 0, false)
	} else {
		x, ok = hasher.Hash(storage, n, 0, 0, false)
	}

	if len(x) > 32 || !ok {
		return hasher.hashitAndStore(nil, storage, x)
	}

	// hasherPool.Put(hasher)
	return x
}

func depth(d int) string {
	if d == 0 {
		return ""
	}
	s := ""
	for i := 0; i < d; i++ {
		s += "\t"
	}
	return s
}

func (n *Node) delEdge(label byte) {
	n.edges[label] = nil
}

func (n *Node) Show() {
	show(n, 0, 0, false)
}

func show(n *Node, d int, label byte, handlePrefix bool) {
	if n.leaf != nil {
		k, v := hexutil.Encode(n.leaf.key), hexutil.Encode(n.leaf.val)

		if n.hash != nil {
			fmt.Printf("%s(%d) HASH: %s\n", depth(d), label, hexutil.Encode(n.hash))
			fmt.Printf("%s(%d) LEAF: %s => %s\n", depth(d+1), label, k, v)
		} else {
			fmt.Printf("%s(%d) LEAF: %s => %s\n", depth(d), label, k, v)
		}
	} else {

		p := n.prefix
		if handlePrefix {
			p = n.prefix[1:]
		}

		if n.hash != nil {
			fmt.Printf("%s(%d) FULL HASH: %s\n", depth(d), label, hexutil.Encode(n.hash))
			return
		}

		if len(p) == 0 {
			fmt.Printf("%s(%d) EDGES: %d\n", depth(d), label, n.Len())
		} else {
			fmt.Printf("%s(%d) SHORT: %s\n", depth(d), label, hexutil.Encode(p))
		}

		for label, e := range n.edges {
			if e != nil {
				show(e, d+1, byte(label), true)
			}
		}
	}
}

type kk struct {
	Key, Val []byte
}

func encodeKeyValue(key, val []byte) []byte {
	dd, err := rlp.EncodeToBytes(&kk{Key: key, Val: val})
	if err != nil {
		panic(err)
	}
	return dd
}

type node []byte

const valueEdge = 16

type hashWithReader interface {
	hash.Hash
	Read([]byte) (int, error)
}

// Hasher hashes the trie
type Hasher struct {
	tmp  [32]byte // last hashed value
	hash hashWithReader
}

func newHasher() *Hasher {
	hash := sha3.NewLegacyKeccak256().(hashWithReader)
	return &Hasher{hash: hash}
}

var hasherPool = sync.Pool{
	New: func() interface{} {
		return &Hasher{
			hash: sha3.NewLegacyKeccak256().(hashWithReader),
		}
	},
}

func (h *Hasher) Hash(storage KVWriter, n *Node, d int, label byte, handlePrefix bool) ([]byte, bool) {
	if n.hash != nil {
		return n.hash, true
	}

	if n.leaf != nil {
		p := n.prefix
		if handlePrefix {
			p = n.prefix[1:]
		}

		v := n.leaf.val
		if label == valueEdge {
			// its a leaf, only encode with rlp and dont hash
			// false because it does not need to be encoded again as rlp
			return encodeItem(v), false
		}

		// Short node
		key := hexToCompact(p)
		val := encodeKeyValue(key, v)
		if len(val) >= 32 {
			hh := h.hashitAndStore(n, storage, val)
			return hh, true
		}

		return val, false
	}

	if len(n.edges) != 0 {
		vals := [17]node{}

		for label, e := range n.edges {
			if e != nil {
				r, ok := h.Hash(storage, e, d+1, byte(label), true)
				if ok {
					vals[label] = encodeItem(r)
				} else {
					vals[label] = r
				}
			}
		}

		val := encodeListOfBytes(vals)
		hashed := false

		if len(val) >= 32 {
			val = h.hashitAndStore(n, storage, val)
			hashed = true
		}

		p := n.prefix
		if handlePrefix {
			p = n.prefix[1:]
		}

		if len(p) == 0 {
			return val, hashed
		}

		// Encode the prefix
		val2 := encodeKeyValue(hexToCompact(p), val)

		if !hashed {
			val2 = encodeList(append(encodeItem(hexToCompact(p)), val...))
		}

		hashed = false

		if len(val2) >= 32 {
			hashed = true
			val2 = h.hashitAndStore(n, storage, val2)
		}

		return val2, hashed
	}

	panic("XX")
}

func (h *Hasher) hashitAndStore(n *Node, storage KVWriter, val []byte) []byte {
	// kk := hashit(val)

	h.hash.Reset()
	h.hash.Write(val)
	h.hash.Read(h.tmp[:])

	if n != nil {
		n.hash = make([]byte, 32)
		copy(n.hash[:], h.tmp[:])
	}

	if storage != nil {
		storage.Put(h.tmp[:], val)
	}
	return h.tmp[:]
}

func encodeListOfBytes(vals [17]node) []byte {
	res := []byte{}
	for _, i := range vals {
		if len(i) == 0 {
			res = append(res, []byte{128}...)
		} else {
			res = append(res, i...)
		}
	}
	return encodeList(res)
}
