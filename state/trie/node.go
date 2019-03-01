package trie

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/common/hexutil"

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
}

func (n *Node) Equal(nn *Node) bool {
	nVals := []*Node{}
	nnVals := []*Node{}

	n.WalkNode(func(n *Node) bool {
		nVals = append(nVals, n)
		return false
	})
	nn.WalkNode(func(n *Node) bool {
		nnVals = append(nnVals, n)
		return false
	})

	if len(nVals) != len(nnVals) {
		return false
	}

	for indx, v := range nVals {
		if !reflect.DeepEqual(v, nnVals[indx]) {
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

func (n *Node) Hash(storage Storage) []byte {
	var x []byte
	var ok bool

	size := n.Len()

	if size == 0 {
		return emptyRoot
	} else if size == 1 { // only one short node
		// its a short node
		x, ok = Hash(storage, n.First(), 0, 0, false)
	} else {
		x, ok = Hash(storage, n, 0, 0, false)
	}

	if len(x) > 32 || !ok {
		return hashitAndStore(storage, x)
	}
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

	/*
		size := n.Len()

		if size == 1 {
			fmt.Println("-- ONE --")
			show(n.First(), 0, 0, false)
		} else {
			fmt.Println("-- TWO --")
			show(n, 0, 0, false)
		}
	*/
}

func show(n *Node, d int, label byte, handlePrefix bool) {
	if n.leaf != nil {
		k, v := hexutil.Encode(n.leaf.key), hexutil.Encode(n.leaf.val)
		fmt.Printf("%s(%d) LEAF: %s => %s\n", depth(d), label, k, v)
	} else {

		p := n.prefix
		if handlePrefix {
			p = n.prefix[1:]
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

func Hash(storage Storage, n *Node, d int, label byte, handlePrefix bool) ([]byte, bool) {
	if n.leaf != nil {

		p := n.prefix

		if handlePrefix {
			p = n.prefix[1:]
		}

		// var v []byte
		// var err error

		v := n.leaf.val

		/*
			if b, ok := n.leaf.val.([]byte); ok {
				v, _ = rlp.EncodeToBytes(bytes.TrimLeft(b[:], "\x00"))
			} else {
				v, err = rlp.EncodeToBytes(n.leaf.val)
				if err != nil {
					panic(err)
				}
			}
		*/

		if label == valueEdge {
			// its a leaf, only encode with rlp and dont hash
			// false because it does not need to be encoded again as rlp
			return encodeItem(v), false
		}

		key := hexToCompact(p)
		val := encodeKeyValue(key, v)
		if len(val) >= 32 {
			return hashitAndStore(storage, val), true
		}

		return val, false
	}

	if len(n.edges) != 0 {
		vals := [17]node{}

		for label, e := range n.edges {
			if e != nil {
				r, ok := Hash(storage, e, d+1, byte(label), true)
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
			val = hashitAndStore(storage, val)
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
			val2 = hashitAndStore(storage, val2)
		}

		return val2, hashed
	}

	panic("XX")
}

func hashitAndStore(storage Storage, val []byte) []byte {
	kk := hashit(val)
	if storage != nil {
		storage.Put(kk, val)
	}
	return kk
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
