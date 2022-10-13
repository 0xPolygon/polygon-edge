package polybft

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMerkleTree_VerifyProofs(t *testing.T) {
	const dataLen = 515

	verifyProof := func(numOfItems uint64) {
		data := make([][]byte, numOfItems)
		for i := uint64(0); i < numOfItems; i++ {
			data[i] = itob(i)
		}

		tree, err := NewMerkleTree(data)
		require.NoError(t, err)

		merkleRootHash := tree.Hash()

		for i := uint64(0); i < numOfItems; i++ {
			proof := tree.GenerateProof(i, 0)
			require.NoError(t, VerifyProof(i, data[i], proof, merkleRootHash))
		}
	}

	// verify proofs for trees of different sizes
	for i := uint64(1); i <= dataLen; i++ {
		verifyProof(i)
	}
}

func TestMerkleTree_VerifyProof_InvalidProof(t *testing.T) {
	const dataLen = 10
	data := make([][]byte, dataLen)

	for i := uint64(0); i < dataLen; i++ {
		data[i] = itob(i)
	}

	tree, err := NewMerkleTree(data)
	require.NoError(t, err)

	merkleRootHash := tree.Hash()

	proof := tree.GenerateProof(7, 0)
	proof[0][0] = proof[0][0] + 1 //change the proof

	require.Error(t, VerifyProof(7, data[7], proof, merkleRootHash))
}
