package merkle

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/stretchr/testify/require"
)

func Benchmark_MerkleTreeCreation_10(b *testing.B) {
	benchmarkMerkleTreeCreation(b, 10)
}

func Benchmark_MerkleTreeCreation_100(b *testing.B) {
	benchmarkMerkleTreeCreation(b, 100)
}

func Benchmark_MerkleTreeCreation_1K(b *testing.B) {
	benchmarkMerkleTreeCreation(b, 1000)
}

func Benchmark_MerkleTreeCreation_10K(b *testing.B) {
	benchmarkMerkleTreeCreation(b, 10_000)
}

func Benchmark_MerkleTreeCreation_100K(b *testing.B) {
	benchmarkMerkleTreeCreation(b, 100_000)
}

func Benchmark_MerkleTreeCreation_1M(b *testing.B) {
	benchmarkMerkleTreeCreation(b, 1_000_000)
}

func Benchmark_GenerateProof_10(b *testing.B) {
	benchmarkGenerateProof(b, 10)
}

func Benchmark_GenerateProof_100(b *testing.B) {
	benchmarkGenerateProof(b, 100)
}

func Benchmark_GenerateProof_1K(b *testing.B) {
	benchmarkGenerateProof(b, 1000)
}

func Benchmark_GenerateProof_10K(b *testing.B) {
	benchmarkGenerateProof(b, 10_000)
}

func Benchmark_GenerateProof_100K(b *testing.B) {
	benchmarkGenerateProof(b, 100_000)
}

func Benchmark_GenerateProof_1M(b *testing.B) {
	benchmarkGenerateProof(b, 1_000_000)
}

func Benchmark_VerifyProof_10(b *testing.B) {
	benchmarkVerifyProof(b, 10)
}

func Benchmark_VerifyProof_100(b *testing.B) {
	benchmarkVerifyProof(b, 100)
}

func Benchmark_VerifyProof_1K(b *testing.B) {
	benchmarkVerifyProof(b, 1000)
}

func Benchmark_VerifyProof_10K(b *testing.B) {
	benchmarkVerifyProof(b, 10_000)
}

func Benchmark_VerifyProof_100K(b *testing.B) {
	benchmarkVerifyProof(b, 100_000)
}

func Benchmark_VerifyProof_1M(b *testing.B) {
	benchmarkVerifyProof(b, 1_000_000)
}

func benchmarkVerifyProof(b *testing.B, numOfLeaves uint64) {
	b.Helper()

	const (
		leafIndex = 0
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
		VerifyProof(leafIndex, data[leafIndex], proof, rootHash) //nolint:errcheck
	}
}

func benchmarkMerkleTreeCreation(b *testing.B, numOfLeaves uint64) {
	b.Helper()

	data := make([][]byte, numOfLeaves)
	for i := uint64(0); i < numOfLeaves; i++ {
		data[i] = common.EncodeUint64ToBytes(i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		NewMerkleTree(data) //nolint:errcheck
	}
}

func benchmarkGenerateProof(b *testing.B, numOfLeaves uint64) {
	b.Helper()

	data := make([][]byte, numOfLeaves)
	for i := uint64(0); i < numOfLeaves; i++ {
		data[i] = common.EncodeUint64ToBytes(i)
	}

	tree, err := NewMerkleTree(data)
	require.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree.GenerateProof(data[0]) //nolint:errcheck
	}
}
