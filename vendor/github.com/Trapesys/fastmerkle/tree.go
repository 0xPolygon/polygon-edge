package fastmerkle

// Node represents a single node in the Merkle
type Node struct {
	hash []byte // The hash of the children nodes

	left   *Node // Reference to the left child
	right  *Node // Reference to the right child
	parent *Node // Reference to the parent node
}

// duplicate creates a copy of the node
// and its reference peers
func (n *Node) duplicate() *Node {
	return &Node{
		hash:   n.hash,
		left:   n.left,
		right:  n.right,
		parent: n.parent,
	}
}

// Hash returns the hash of the node's children
func (n *Node) Hash() []byte {
	return n.hash
}

// MerkleTree represents the perfect Merkle binary tree
type MerkleTree struct {
	root *Node // The root of the Merkle binary tree
}

// Root returns the root of the Merkle tree
func (m *MerkleTree) Root() *Node {
	return m.root
}

// RootHash returns the root hash fo the Merkle tree
func (m *MerkleTree) RootHash() []byte {
	return m.root.Hash()
}
