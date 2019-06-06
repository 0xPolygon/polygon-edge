package rlp

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	legacyRlp "github.com/ethereum/go-ethereum/rlp"
)

func TestEncodeRandom(t *testing.T) {
	t.Parallel()

	rand.Seed(time.Now().UTC().UnixNano())

	for i := 0; i < 100000; i++ {
		x := generateRandom()

		legacy, err := legacyRlp.EncodeToBytes(x)
		if err != nil {
			t.Fatal(err)
		}

		enc := AcquireEncoder()
		if err := enc.Encode(x); err != nil {
			t.Fatal(err)
		}

		res := enc.Bytes()
		if !reflect.DeepEqual(res, legacy) {
			t.Fatal("bad")
		}
	}
}

func TestEncodeRandomTypes(t *testing.T) {
	t.Parallel()

	rand.Seed(time.Now().UTC().UnixNano())

	for i := 0; i < 100000; i++ {
		tt := pickRandomType(0)

		out := generateRandomType(tt)

		r1, err := legacyRlp.EncodeToBytes(out)
		if err != nil {
			t.Fatal(err)
		}

		enc := AcquireEncoder()
		if err := enc.Encode(out); err != nil {
			t.Fatal(err)
		}

		r2 := enc.Bytes()
		if !reflect.DeepEqual(r1, r2) {
			t.Fatal("bad")
		}
	}
}
