package rlpv2

import (
	"bytes"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/umbracle/minimal/helper/hex"
	"github.com/umbracle/minimal/rlp"
)

var bigIntTyp = reflect.TypeOf(new(big.Int))

func testEncode(t *testing.T, s interface{}) []byte {
	a := &Arena{}
	v := testEncodeImpl(a, t, reflect.ValueOf(s))
	return v.MarshalTo(nil)
}

func testEncodeImpl(a *Arena, t *testing.T, v reflect.Value) *Value {
	if v.Kind() == reflect.Array && isByte(v.Type().Elem()) {
		return a.NewBytes(v.Slice(0, v.Len()).Bytes())
	}
	if v.Kind() == reflect.Slice && isByte(v.Type().Elem()) {
		return a.NewBytes(v.Bytes())
	}

	switch v.Kind() {
	case reflect.Bool:
		return a.NewBool(v.Bool())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return a.NewUint(v.Uint())

	case reflect.Ptr:
		if v.Type() == bigIntTyp {
			return a.NewBigInt(v.Interface().(*big.Int))
		}
		return testEncodeImpl(a, t, v.Elem())

	case reflect.String:
		return a.NewString(v.String())

	case reflect.Slice:
		fallthrough
	case reflect.Array:
		vv := a.NewArray()
		for i := 0; i < v.Len(); i++ {
			vv.Set(testEncodeImpl(a, t, v.Index(i)))
		}
		return vv

	case reflect.Struct:
		vv := a.NewArray()
		for i := 0; i < v.NumField(); i++ {
			vv.Set(testEncodeImpl(a, t, v.Field(i)))
		}
		return vv

	default:
		t.Fatalf("failed to encode '%s'", v.Kind().String())
	}
	return nil
}

func TestEncode(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	for i := 0; i < 10000; i++ {
		// t.Run("", func(t *testing.T) {
		tt := pickRandomType(0)
		input := generateRandomType(tt)

		out1, err := rlp.EncodeToBytes(input)
		if err != nil {
			t.Fatal(err)
		}

		out2 := testEncode(t, input)
		if !bytes.Equal(out1, out2) {
			panic("X")
		}
		// })
	}
}

func BenchmarkRlpEncode(b *testing.B) {
	b.ReportAllocs()

	a := Arena{}

	b1 := hex.MustDecodeHex("0x1111111111111111")

	for i := 0; i < b.N; i++ {

		vv := a.NewArray()
		vv.Set(a.NewUint(10000000))
		vv.Set(a.NewBytes([]byte("1000000")))
		vv.Set(a.NewBytes(b1))

		a.Reset()
	}
}

func BenchmarkLegacyRlpEncode(b *testing.B) {
	b.ReportAllocs()

	data := []interface{}{
		uint(10000000),
		"1000000",
		hex.MustDecodeHex("0x1111111111111111"),
	}

	for i := 0; i < b.N; i++ {
		rlp.EncodeToBytes(data)
	}
}
