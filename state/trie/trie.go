package trie

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/sha3"
)

// Merkle-trie based on hashicorp go-immutable-radix

type Trie struct {
	root *Node
}

func NewTrie() *Trie {
	return &Trie{
		root: &Node{},
	}
}

func NewTrieAt(storage Storage, root common.Hash) (*Trie, error) {
	data, ok := storage.Get(root.Bytes())
	if !ok {
		return nil, fmt.Errorf("root not found")
	}

	// NOTE, this expands the whole trie
	node, err := DecodeNode(storage, []byte{}, data)
	if err != nil {
		return nil, err
	}

	t := &Trie{
		root: node,
	}
	return t, nil
}

func (t *Trie) Get(k []byte) ([]byte, bool) {
	return t.root.Get(KeybytesToHex(k))
}

func (t *Trie) Txn() *Txn {
	return &Txn{
		root: t.root,
	}
}

func (t *Trie) Root() *Node {
	return t.root
}

type Txn struct {
	root *Node
}

func (t *Txn) Copy() *Txn {
	tt := new(Txn)
	tt.root = t.root
	return tt
}

func (t *Txn) Commit() *Trie {
	return &Trie{t.root}
}

func (t *Txn) Insert(key []byte, v []byte) {
	k := KeybytesToHex(key)
	newRoot, _, _ := t.insert(t.root, k, k, v)
	if newRoot != nil {
		t.root = newRoot
	}
}

// Delete is used to delete a given key. Returns the old value if any,
// and a bool indicating if the key was set.
func (t *Txn) Delete(key []byte) bool {
	k := KeybytesToHex(key)
	newRoot, leaf := t.delete(nil, t.root, k)
	if newRoot != nil {
		t.root = newRoot
	}
	if leaf != nil {
		return true
	}
	return false
}

// Get returns a specific key
func (t *Txn) Get(key []byte) ([]byte, bool) {
	k := KeybytesToHex(key)
	return t.root.Get(k)
}

// insert does a recursive insertion
func (t *Txn) insert(n *Node, k, search []byte, v []byte) (*Node, []byte, bool) {
	// Handle key exhaustion
	if len(search) == 0 {
		var oldVal []byte
		didUpdate := false
		if n.isLeaf() {
			oldVal = n.leaf.val
			didUpdate = true
		}

		nc := t.writeNode(n, true)
		nc.leaf = &leafNode{
			key: k,
			val: v,
		}
		return nc, oldVal, didUpdate
	}

	// Look for the edge
	idx, child := n.getEdge(search[0])

	// No edge, create one
	if child == nil {
		e := edge{
			label: search[0],
			node: &Node{
				leaf: &leafNode{
					key: k,
					val: v,
				},
				prefix: search,
			},
		}
		nc := t.writeNode(n, false)
		nc.addEdge(e)
		return nc, nil, false
	}

	// Determine longest prefix of the search key on match
	commonPrefix := longestPrefix(search, child.prefix)
	if commonPrefix == len(child.prefix) {
		search = search[commonPrefix:]
		newChild, oldVal, didUpdate := t.insert(child, k, search, v)
		if newChild != nil {
			nc := t.writeNode(n, false)
			nc.edges[idx] = newChild
			return nc, oldVal, didUpdate
		}
		return nil, oldVal, didUpdate
	}

	// Split the node
	nc := t.writeNode(n, false)
	splitNode := &Node{
		prefix: search[:commonPrefix],
	}
	nc.replaceEdge(edge{
		label: search[0],
		node:  splitNode,
	})

	// Restore the existing child node
	modChild := t.writeNode(child, false)
	splitNode.addEdge(edge{
		label: modChild.prefix[commonPrefix],
		node:  modChild,
	})
	modChild.prefix = modChild.prefix[commonPrefix:]

	// Create a new leaf node
	leaf := &leafNode{
		key: k,
		val: v,
	}

	// If the new key is a subset, add to to this node
	search = search[commonPrefix:]
	if len(search) == 0 {
		splitNode.leaf = leaf
		return nc, nil, false
	}

	// Create a new edge for the node
	splitNode.addEdge(edge{
		label: search[0],
		node: &Node{
			leaf:   leaf,
			prefix: search,
		},
	})
	return nc, nil, false
}

// delete does a recursive deletion
func (t *Txn) delete(parent, n *Node, search []byte) (*Node, *leafNode) {
	// Check for key exhaustion
	if len(search) == 0 {
		if !n.isLeaf() {
			return nil, nil
		}
		// Copy the pointer in case we are in a transaction that already
		// modified this node since the node will be reused. Any changes
		// made to the node will not affect returning the original leaf
		// value.
		oldLeaf := n.leaf

		// Remove the leaf node
		nc := t.writeNode(n, true)
		nc.leaf = nil

		// Check if this node should be merged
		if n != t.root && nc.Len() == 1 {
			t.mergeChild(nc)
		}
		return nc, oldLeaf
	}

	// Look for an edge
	label := search[0]
	idx, child := n.getEdge(label)
	if child == nil || !bytes.HasPrefix(search, child.prefix) {
		return nil, nil
	}

	// Consume the search prefix
	search = search[len(child.prefix):]
	newChild, leaf := t.delete(n, child, search)
	if newChild == nil {
		return nil, nil
	}

	// Copy this node. WATCH OUT - it's safe to pass "false" here because we
	// will only ADD a leaf via nc.mergeChild() if there isn't one due to
	// the !nc.isLeaf() check in the logic just below. This is pretty subtle,
	// so be careful if you change any of the logic here.
	nc := t.writeNode(n, false)

	// Delete the edge if the node has no edges
	if newChild.leaf == nil && newChild.Len() == 0 {
		nc.delEdge(label)

		if n != t.root && nc.Len() == 1 && !nc.isLeaf() {
			t.mergeChild(nc)
		}
	} else {
		nc.edges[idx] = newChild
	}
	return nc, leaf
}

// mergeChild is called to collapse the given node with its child. This is only
// called when the given node is not a leaf and has a single edge.
func (t *Txn) mergeChild(n *Node) {
	e := n.First()
	child := e

	// Merge the nodes.
	n.prefix = concat(n.prefix, child.prefix)
	n.leaf = child.leaf

	n.edges = [17]*Node{}
	copy(n.edges[:], child.edges[:])
}

// concat two byte slices, returning a third new copy
func concat(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)
	return c
}

func hashit(b []byte) []byte {
	f := sha3.NewKeccak256()
	f.Write(b)
	res := f.Sum(nil)
	return res
}

func (t *Txn) Hash(storage Storage) []byte {
	return t.root.Hash(storage)
}

func (t *Txn) writeNode(n *Node, x bool) *Node {
	nc := &Node{
		leaf: n.leaf,
	}

	if n.prefix != nil {
		nc.prefix = make([]byte, len(n.prefix))
		copy(nc.prefix, n.prefix)
	}
	copy(nc.edges[:], n.edges[:])
	return nc
}

func longestPrefix(k1, k2 []byte) int {
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
