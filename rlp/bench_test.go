package rlp

import (
	"io/ioutil"
	"testing"

	legacyTypes "github.com/ethereum/go-ethereum/core/types"
	legacyRlp "github.com/ethereum/go-ethereum/rlp"
)

var benchFile = "./testdata/headers_100"

func decodeHeadersFile(file string) []*legacyTypes.Header {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}

	var headers []*legacyTypes.Header
	if err := legacyRlp.DecodeBytes(data, &headers); err != nil {
		panic(err)
	}
	return headers
}

func BenchmarkDecodeLegacyRLPHeader(b *testing.B) {
	b.ReportAllocs()

	data, err := ioutil.ReadFile(benchFile)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))

	for n := 0; n < b.N; n++ {
		var headers []*legacyTypes.Header
		if err := legacyRlp.DecodeBytes(data, &headers); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeRLPHeader(b *testing.B) {
	b.ReportAllocs()

	data, err := ioutil.ReadFile(benchFile)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))

	for n := 0; n < b.N; n++ {
		var headers []*legacyTypes.Header
		if err := Decode(data, &headers); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeLegacyRLPHeader(b *testing.B) {
	b.ReportAllocs()

	headers := decodeHeadersFile(benchFile)

	for n := 0; n < b.N; n++ {
		if _, err := legacyRlp.EncodeToBytes(&headers); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRLPHeader(b *testing.B) {
	b.ReportAllocs()

	headers := decodeHeadersFile(benchFile)

	for n := 0; n < b.N; n++ {
		enc := AcquireEncoder()
		if err := enc.Encode(&headers); err != nil {
			b.Fatal(err)
		}

		enc.CopyBytes()
		ReleaseEncoder(enc)
	}
}
