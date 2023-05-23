package merkle

import (
	"math"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestMerkleTree_VerifyProofs(t *testing.T) {
	t.Parallel()

	const dataLen = 515

	verifyProof := func(numOfItems uint64) {
		data := make([][]byte, numOfItems)
		for i := uint64(0); i < numOfItems; i++ {
			data[i] = common.EncodeUint64ToBytes(i)
		}

		tree, err := NewMerkleTree(data)
		require.NoError(t, err)

		merkleRootHash := tree.Hash()
		treeDepth := tree.Depth()

		for i := uint64(0); i < numOfItems; i++ {
			leafIndex, err := tree.LeafIndex(data[i])
			require.NoError(t, err)

			proof, err := tree.GenerateProof(data[i])
			require.NoError(t, err)

			// valid proof
			require.NoError(t, VerifyProof(leafIndex, data[i], proof, merkleRootHash))
			// proof length should always equal to the depth of the tree
			require.Equal(t, treeDepth, len(proof))

			// invalid leaf
			require.ErrorContains(t, VerifyProof(leafIndex, []byte{}, proof, merkleRootHash), "empty leaf") // invalid leaf
			// invalid index
			require.ErrorContains(t, VerifyProof(uint64(math.Pow(2, float64(len(proof))+1)),
				data[i], proof, merkleRootHash), "invalid leaf index")
			// invalid proof - not a member of merkle tree
			proof[0][0]++
			require.ErrorContains(t, VerifyProof(leafIndex, data[i], proof, merkleRootHash), "not a member of merkle tree")

			// invalid leaf data on generating proof
			dataCopy := make([]byte, len(data[i]))
			copy(dataCopy, data[i])
			dataCopy[0] = dataCopy[0] + 1
			_, err = tree.GenerateProof(dataCopy)
			require.ErrorContains(t, err, "data not in merkle tree")
		}
	}

	// verify proofs for trees of different sizes
	for i := uint64(2); i <= dataLen; i++ {
		verifyProof(i)
	}
}

func TestMerkleTree_VerifyProof_TreeWithOneNode(t *testing.T) {
	t.Parallel()

	leafData := []byte{1}
	treeData := [][]byte{leafData}

	tree, err := NewMerkleTree(treeData)
	require.NoError(t, err)

	proof, err := tree.GenerateProof(leafData)
	require.NoError(t, err)
	require.Empty(t, proof) // since tree contains one node, there is no proof, it's proof is rootHash == hashOfLeaf

	index, err := tree.LeafIndex(leafData)
	require.NoError(t, err)
	require.Equal(t, uint64(0), index) // should be 0 since tree only has one node
	require.NoError(t, VerifyProof(index, leafData, proof, tree.Hash()))

	// invalid proof
	invalidProof := []types.Hash{types.BytesToHash([]byte{0, 1, 2, 3, 4})}
	require.ErrorContains(t, VerifyProof(index, leafData, invalidProof, tree.Hash()), "not a member of merkle tree")

	// invalid index
	require.ErrorContains(t, VerifyProof(11, leafData, proof, tree.Hash()), "invalid leaf index")

	// empty leaf
	require.ErrorContains(t, VerifyProof(11, []byte{}, proof, tree.Hash()), "empty leaf")
}
