package buildroot

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

func BenchmarkFast(b *testing.B) {
	f := acquireFastHasher()

	res := buildInput(128, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		f.Hash(128, res)
		f.reset()
	}
}

func BenchmarkSlow(b *testing.B) {
	res := buildInput(128, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		deriveSlow(128, res)
	}
}

func TestFastHasher(t *testing.T) {
	f := FastHasher{
		k: keccak.NewKeccak256(),
	}

	for i := 0; i < 1000; i++ {
		num := randomInt(1, 128)
		res := buildRandomInput(int(num))

		found, _ := f.Hash(int(num), res)
		realRes := deriveSlow(int(num), res)

		if !bytes.Equal(found, realRes) {
			t.Fatal("bad")
		}

		f.reset()
	}
}

func randomInt(min, max uint64) uint64 {
	randNum, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))

	return min + randNum.Uint64()
}

func buildInput(n, m int) func(i int) []byte {
	res := [][]byte{}

	for i := 0; i < n; i++ {
		b := make([]byte, m)
		for indx := range b {
			b[indx] = byte(i)
		}

		res = append(res, b)
	}

	return func(i int) []byte {
		return res[i]
	}
}

func buildRandomInput(num int) func(i int) []byte {
	res := [][]byte{}

	for i := 0; i < num; i++ {
		b := make([]byte, randomInt(33, 200))
		_, _ = rand.Read(b)
		res = append(res, b)
	}

	return func(i int) []byte {
		return res[i]
	}
}

func TestAcquireFastHasher(t *testing.T) {
	// Test when fastHasherPool returns a value
	t.Run("FastHasherPoolReturnsValue", func(t *testing.T) {
		// Create a mock FastHasher object
		mockHasher := &FastHasher{k: keccak.NewKeccak256()}

		fastHasherPool.New = func() interface{} {
			return mockHasher
		}

		// Call the acquireFastHasher function
		hasher := acquireFastHasher()

		// Check if the returned hasher is the same as the mockHasher
		if hasher != mockHasher {
			t.Error("Expected acquireFastHasher to return the mockHasher")
		}
	})

	// Test when fastHasherPool returns nil
	t.Run("FastHasherPoolReturnsNil", func(t *testing.T) {
		// Mock the Get function of fastHasherPool to return nil
		fastHasherPool.New = func() interface{} {
			return nil
		}

		// Call the acquireFastHasher function
		hasher := acquireFastHasher()

		// Check if the returned hasher is not nil
		if hasher == nil {
			t.Error("Expected acquireFastHasher to return a non-nil hasher")
		}
	})
}
