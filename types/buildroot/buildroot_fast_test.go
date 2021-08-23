package buildroot

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/helper/keccak"
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
		real := deriveSlow(int(num), res)

		if !bytes.Equal(found, real) {
			t.Fatal("bad")
		}

		f.reset()
	}
}

func randomInt(min, max uint64) uint64 {
	rand.Seed(time.Now().UnixNano())
	return min + uint64(rand.Intn(int(max-min)))
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
		rand.Read(b)
		res = append(res, b)
	}
	return func(i int) []byte {
		return res[i]
	}
}
