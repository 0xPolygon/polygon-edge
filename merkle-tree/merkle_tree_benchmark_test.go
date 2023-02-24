package merkle

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/stretchr/testify/require"
)

func Benchmark_MerkleTreeCreation(b *testing.B) {
	const numOfLeaves = 10_000

	data := make([][]byte, numOfLeaves)
	for i := uint64(0); i < numOfLeaves; i++ {
		data[i] = common.EncodeUint64ToBytes(i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		NewMerkleTree(data)
	}
}

func Benchmark_GenerateProof(b *testing.B) {
	const numOfLeaves = 10_000

	data := make([][]byte, numOfLeaves)
	for i := uint64(0); i < numOfLeaves; i++ {
		data[i] = common.EncodeUint64ToBytes(i)
	}

	tree, err := NewMerkleTree(data)
	require.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree.GenerateProof(data[0])
	}
}

func Benchmark_VerifyProof(b *testing.B) {
	const (
		numOfLeaves = 10_000
		leafIndex   = 0
	)

	data := make([][]byte, numOfLeaves)
	for i := uint64(0); i < numOfLeaves; i++ {
		data[i] = common.EncodeUint64ToBytes(i)
	}

	tree, err := NewMerkleTree(data)
	require.NoError(b, err)

	proof, err := tree.GenerateProof(data[leafIndex])
	require.NoError(b, err)

	rootHash := tree.Hash()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		VerifyProof(leafIndex, data[leafIndex], proof, rootHash)
	}
}
