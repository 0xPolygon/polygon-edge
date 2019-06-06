package rlp

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	legacyRlp "github.com/ethereum/go-ethereum/rlp"
)

func TestDecodeRandom(t *testing.T) {
	t.Parallel()

	rand.Seed(time.Now().UTC().UnixNano())

	for i := 0; i < 100000; i++ {
		x := generateRandom()

		out, err := legacyRlp.EncodeToBytes(x)
		if err != nil {
			t.Fatal(err)
		}

		var legacyRes interface{}
		legacyRlp.DecodeBytes(out, &legacyRes)

		var res interface{}
		if err := Decode(out, &res); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(legacyRes, res) {
			t.Fatal("bad")
		}
	}
}

func TestDecodeRandomTypes(t *testing.T) {
	t.Parallel()

	rand.Seed(time.Now().UTC().UnixNano())

	for i := 0; i < 100000; i++ {
		tt := pickRandomType(0)
		out := generateRandomType(tt)

		data, err := legacyRlp.EncodeToBytes(out)
		if err != nil {
			t.Fatal(err)
		}

		val1 := reflect.New(tt.Type()).Interface()
		if err := legacyRlp.DecodeBytes(data, &val1); err != nil {
			t.Fatal(err)
		}

		val2 := reflect.New(tt.Type()).Interface()
		if err := Decode(data, &val2); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(val1, val2) {
			t.Fatal("bad")
		}
	}
}
