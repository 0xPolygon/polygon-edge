package polybft

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

// MerkleTree is the structure for the Merkle tree.
type MerkleTree struct {
	// hasher is a pointer to the hashing struct (e.g., Keccak256)
	hasher hash.Hash
	// data is the data from which the Merkle tree is created
	data [][]byte
	// nodes are the leaf and branch nodes of the Merkle tree
	nodes [][]byte
}

// NewMerkleTree creates a new Merkle tree from the provided data and using the default hashing (Keccak256).
func NewMerkleTree(data [][]byte) (*MerkleTree, error) {
	return NewMerkleTreeWithHashing(data, crypto.NewKeccakState())
}

// NewMerkleTreeWithHashing creates a new Merkle tree from the provided data and hash type
func NewMerkleTreeWithHashing(data [][]byte, hash hash.Hash) (*MerkleTree, error) {
	if len(data) == 0 {
		return nil, errors.New("tree must contain at least one leaf")
	}

	branchesLen := int(math.Exp2(math.Ceil(math.Log2(float64(len(data))))))

	nodes := make([][]byte, branchesLen+len(data)+(branchesLen-len(data)))
	// create leaves
	for i := range data {
		hash.Reset()
		hash.Write(data[i])
		h := hash.Sum(nil)
		nodes[i+branchesLen] = h
	}

	for i := len(data) + branchesLen; i < len(nodes); i++ {
		nodes[i] = make([]byte, types.HashLength)
	}

	// create branches
	for i := branchesLen - 1; i > 0; i-- {
		hash.Reset()
		hash.Write(nodes[i*2])
		hash.Write(nodes[i*2+1])
		nodes[i] = hash.Sum(nil)
	}

	tree := &MerkleTree{
		hasher: hash,
		nodes:  nodes,
		data:   data,
	}

	return tree, nil
}

// Hash is the Merkle Tree root hash
func (t *MerkleTree) Hash() types.Hash {
	return types.BytesToHash(t.nodes[1])
}

// String implements the stringer interface
func (t *MerkleTree) String() string {
	return hex.EncodeToString(t.Hash().Bytes())
}

// GenerateProof generates the proof of membership for a piece of data in the Merkle tree.
// If the data is not present in the tree this will return an error.
func (t *MerkleTree) GenerateProof(index uint64, height int) []types.Hash {
	proofLen := int(math.Ceil(math.Log2(float64(len(t.data))))) - height
	proofHashes := make([]types.Hash, proofLen)

	hashIndex := 0
	minI := uint64(math.Pow(2, float64(height+1))) - 1

	for i := index + uint64(len(t.nodes)/2); i > minI; i /= 2 {
		proofHashes[hashIndex] = types.BytesToHash(t.nodes[i^1])
		hashIndex++
	}

	return proofHashes
}

// VerifyProof verifies a Merkle tree proof of membership for provided data using the default hash type (Keccak256)
func VerifyProof(index uint64, leaf []byte, proof []types.Hash, root types.Hash) error {
	return VerifyProofUsing(index, leaf, proof, root, crypto.NewKeccakState())
}

// VerifyProofUsing verifies a Merkle tree proof of membership for provided data using the provided hash type
func VerifyProofUsing(index uint64, leaf []byte, proof []types.Hash, root types.Hash, hash hash.Hash) error {
	proofHash := getProofHash(index, leaf, proof, hash)
	if !bytes.Equal(root.Bytes(), proofHash) {
		return fmt.Errorf("leaf with index %v, not a member of merkle tree. Merkle root hash: %v", index, root)
	}

	return nil
}

func getProofHash(index uint64, leaf []byte, proof []types.Hash, hash hash.Hash) []byte {
	hash.Write(leaf)
	computedHash := hash.Sum(nil)

	for i := 0; i < len(proof); i++ {
		hash.Reset()

		if index%2 == 0 {
			hash.Write(computedHash)
			hash.Write(proof[i].Bytes())
		} else {
			hash.Write(proof[i].Bytes())
			hash.Write(computedHash)
		}

		computedHash = hash.Sum(nil)
		index /= 2
	}

	return computedHash
}
