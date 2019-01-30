package trie

import (
	"bytes"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421").Bytes()
)

// WalkFn is used when walking the tree. Takes a
// key and value, returning if iteration should
// be terminated.
type WalkFn func(k []byte, v interface{}) bool

type leafNode struct {
	key []byte
	val interface{}
}

type edge struct {
	label byte
	node  *Node
}

type edges []edge

func (e edges) Len() int {
	return len(e)
}

func (e edges) Less(i, j int) bool {
	return e[i].label < e[j].label
}

func (e edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e edges) Sort() {
	sort.Sort(e)
}

type Node struct {
	leaf   *leafNode
	prefix []byte
	edges  edges
}

func (n *Node) isLeaf() bool {
	return n.leaf != nil
}

func (n *Node) isShort() bool {
	return len(n.edges) == 0 && n.leaf != nil
}

func (n *Node) addEdge(e edge) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= e.label
	})
	n.edges = append(n.edges, e)
	if idx != num {
		copy(n.edges[idx+1:], n.edges[idx:num])
		n.edges[idx] = e
	}
}

func (n *Node) replaceEdge(e edge) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= e.label
	})
	if idx < num && n.edges[idx].label == e.label {
		n.edges[idx].node = e.node
		return
	}
	panic("replacing missing edge")
}

func (n *Node) getEdge(label byte) (int, *Node) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		return idx, n.edges[idx].node
	}
	return -1, nil
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
		if recursiveWalk(e.node, fn) {
			return true
		}
	}
	return false
}

func (n *Node) Get(k []byte) (interface{}, bool) {
	val, ok := n.get(k)
	return val, ok
}

func (n *Node) get(k []byte) (interface{}, bool) {
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

func (n *Node) Hash() []byte {
	var x []byte
	var ok bool

	if len(n.edges) == 0 && n.leaf == nil {
		return emptyRoot
	}

	if len(n.edges) == 1 {
		// its a short node
		x, ok = Hash(n.edges[0].node, 0, 0, false)
	} else {
		x, ok = Hash(n, 0, 0, false)
	}

	if len(x) > 32 || !ok {
		return hashit(x)
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
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		copy(n.edges[idx:], n.edges[idx+1:])
		n.edges[len(n.edges)-1] = edge{}
		n.edges = n.edges[:len(n.edges)-1]
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

func Hash(n *Node, d int, label byte, handlePrefix bool) ([]byte, bool) {
	if n.leaf != nil {
		if len(n.edges) != 0 {
			panic("Leaf cannot have edges")
		}

		p := n.prefix

		if handlePrefix {
			p = n.prefix[1:]
		}

		var v []byte
		var err error

		if b, ok := n.leaf.val.([]byte); ok {
			v, _ = rlp.EncodeToBytes(bytes.TrimLeft(b[:], "\x00"))
		} else {
			v, err = rlp.EncodeToBytes(n.leaf.val)
			if err != nil {
				panic(err)
			}
		}

		if label == valueEdge {
			// its a leaf, only encode with rlp and dont hash
			// false because it does not need to be encoded again as rlp
			return encodeItem(v), false
		}

		key := hexToCompact(p)
		val := encodeKeyValue(key, v)
		if len(val) >= 32 {
			return hashit(val), true
		}

		return val, false
	}

	if len(n.edges) != 0 {
		vals := [17]node{}

		for _, e := range n.edges {
			r, ok := Hash(e.node, d+1, e.label, true)
			if ok {
				vals[e.label] = encodeItem(r)
			} else {
				vals[e.label] = r
			}
		}

		val := encodeListOfBytes(vals)
		hashed := false

		if len(val) >= 32 {
			val = hashit(val)
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
			val2 = hashit(val2)
		}

		return val2, hashed
	}

	panic("XX")
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
