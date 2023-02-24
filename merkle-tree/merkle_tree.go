package merkle

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

// A Merkle tree example:
//
//              ROOT
//            /       \
//          h3         h7
//         /  \      /    \
//       h1   h2    h5    h6
//      / \   / \   / \   / \
//     L0 L1 L2 L3 L4 L5 L6 L7
//
// Each leaf node (L0 - L7) contains a data value, and each intermediate node
// (h1 - h7) contains the hash of its two child nodes. The root node (ROOT) contains
// the hash of the entire tree. To prove the inclusion of a leaf node in the tree,
// a Merkle proof would consist of the leaf node, along with the hash values of its
// sibling nodes on the path from the leaf to the root.
//
// So the proof for leaf node L0 will look like this:
// H(Hash(L1), Hash(H2), Hash(H7))

var (
	errLeafNotFound = errors.New("leaf not found")
)

// MerkleNode represents a single node in merkle tree
type MerkleNode struct {
	left   *MerkleNode
	right  *MerkleNode
	parent *MerkleNode
	data   []byte
	hash   types.Hash
}

// newMerkleNode creates a new merkle node
func newMerkleNode(left, right *MerkleNode, data []byte, hasher hash.Hash) *MerkleNode {
	node := &MerkleNode{left: left, right: right, data: data}

	var dataToHash []byte
	if left == nil && right == nil {
		// it's a leaf node
		dataToHash = data
	} else {
		// it's an inner node
		dataToHash = append(left.hash.Bytes(), right.hash.Bytes()...)
	}

	hasher.Reset()
	hasher.Write(dataToHash)
	node.hash = types.BytesToHash(hasher.Sum(nil))

	return node
}

// MerkleTree is the structure for the Merkle tree.
type MerkleTree struct {
	// hasher is a pointer to the hashing struct (e.g., Keccak256)
	hasher hash.Hash
	// rootNode is the root node of the tree
	rootNode *MerkleNode
	// leaf nodes is the list of leaf nodes of the tree (the lowest level of the tree)
	leafNodes []*MerkleNode
}

// NewMerkleTree creates a new Merkle tree from the provided data and using the default hashing (Keccak256).
func NewMerkleTree(data [][]byte) (*MerkleTree, error) {
	return NewMerkleTreeWithHashing(data, crypto.NewKeccakState())
}

// NewMerkleTreeWithHashing creates a new Merkle tree from the provided data and hash type
func NewMerkleTreeWithHashing(data [][]byte, hasher hash.Hash) (*MerkleTree, error) {
	if len(data) == 0 {
		return nil, errors.New("tree must contain at least one leaf")
	}

	leafNodes := make([]*MerkleNode, len(data))
	for i, d := range data {
		leafNodes[i] = newMerkleNode(nil, nil, d, hasher)
	}

	nodes := leafNodes
	for len(nodes) > 1 {
		var newLevel []*MerkleNode

		for i := 0; i < len(nodes); i += 2 {
			left := nodes[i]
			right := nodes[i] // if the tree has uneven number of nodes, this will duplicate the last one

			if i+1 < len(nodes) {
				right = nodes[i+1] // if it has even numbers, then this will point to the last node
			}

			parent := newMerkleNode(left, right, nil, hasher)
			left.parent = parent
			right.parent = parent

			newLevel = append(newLevel, parent)
		}

		nodes = newLevel
	}

	return &MerkleTree{
		hasher:    hasher,
		rootNode:  nodes[0],
		leafNodes: leafNodes,
	}, nil
}

// LeafIndex returns the index of given leaf if found in tree
func (t *MerkleTree) LeafIndex(leaf []byte) (uint64, error) {
	for i, leafNode := range t.leafNodes {
		if bytes.Equal(leafNode.data, leaf) {
			return uint64(i), nil
		}
	}

	return 0, errLeafNotFound
}

// Hash is the Merkle Tree root hash
func (t *MerkleTree) Hash() types.Hash {
	return t.rootNode.hash
}

// String implements the stringer interface
func (t *MerkleTree) String() string {
	return hex.EncodeToString(t.Hash().Bytes())
}

// Depth returns the depth of merkle tree
func (t *MerkleTree) Depth() int {
	return int(math.Ceil(math.Log2(float64(len(t.leafNodes)))))
}

// GenerateProof generates the proof of membership for a piece of data in the Merkle tree.
func (t *MerkleTree) GenerateProof(leaf []byte) ([]types.Hash, error) {
	proof := []types.Hash{}

	leafNode := t.findLeafNode(leaf)
	if leafNode == nil {
		return nil, fmt.Errorf("given data not in merkle tree")
	}

	node := leafNode
	for !bytes.Equal(node.hash.Bytes(), t.rootNode.hash.Bytes()) {
		if bytes.Equal(node.parent.left.hash.Bytes(), node.hash.Bytes()) {
			proof = append(proof, node.parent.right.hash)
		} else {
			proof = append(proof, node.parent.left.hash)
		}

		node = node.parent
	}

	return proof, nil
}

// findLeafNode finds the given leaf node that corresponds to given leaf data
func (t *MerkleTree) findLeafNode(leaf []byte) *MerkleNode {
	for _, leafNode := range t.leafNodes {
		if bytes.Equal(leafNode.data, leaf) {
			return leafNode
		}
	}

	return nil
}

// VerifyProof verifies a Merkle tree proof of membership for provided data using the default hash type (Keccak256)
func VerifyProof(index uint64, leaf []byte, proof []types.Hash, root types.Hash) error {
	return VerifyProofUsing(index, leaf, proof, root, crypto.NewKeccakState())
}

// VerifyProofUsing verifies a Merkle tree proof of membership for provided data using the provided hash type
func VerifyProofUsing(index uint64, leaf []byte, proof []types.Hash, root types.Hash, hasher hash.Hash) error {
	if len(leaf) == 0 {
		return fmt.Errorf("empty leaf")
	}

	if int(index) >= int(math.Pow(2, float64(len(proof)))) {
		return fmt.Errorf("invalid leaf index %v", index)
	}

	computedHash := getProofHash(index, leaf, proof, hasher)
	if !bytes.Equal(root.Bytes(), computedHash) {
		return fmt.Errorf("leaf with index %v, not a member of merkle tree. Merkle root hash: %v", index, root)
	}

	return nil
}

// getProofHash uses the leaf and proof to recalculate the root hash of a tree that contains given leaf
func getProofHash(index uint64, leaf []byte, proof []types.Hash, hasher hash.Hash) []byte {
	hasher.Write(leaf)
	computedHash := hasher.Sum(nil)

	for i := 0; i < len(proof); i++ {
		hasher.Reset()

		if index%2 == 0 {
			hasher.Write(computedHash)
			hasher.Write(proof[i].Bytes())
		} else {
			hasher.Write(proof[i].Bytes())
			hasher.Write(computedHash)
		}

		computedHash = hasher.Sum(nil)
		index /= 2
	}

	return computedHash
}
