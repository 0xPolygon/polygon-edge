package ethash

import (
	"encoding/binary"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/minimal/helper/hex"
)

func TestHashimotoLight(t *testing.T) {
	cases := []struct {
		epoch  int
		digest string
		result string
	}{
		{
			epoch:  0,
			digest: "0xa2b2199089a71759688bbac4ac27d289d6fb08095b177631a6a74b4fb4b933f3",
			result: "0xd60e5e7cda364597214232e28b5673c59d93eb2cc0885df097269fa726cc0a35",
		},
		{
			epoch:  1,
			digest: "0x1f741fbcbd6ada281642dc589cc1b51d8a8648562df26d2cf58777ba43819dd5",
			result: "0x25d1d046befe65c2a6dd5574dfb56155fd3ed31c156a67a9a12882a70bd016ed",
		},
	}

	header := make([]byte, 32)

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			cache := newCache(c.epoch)
			digest, result := cache.hashimoto(header, 100)

			assert.Equal(t, c.digest, hex.EncodeToHex(digest))
			assert.Equal(t, c.result, hex.EncodeToHex(result))
		})
	}
}

func TestCache(t *testing.T) {
	cases := []struct {
		epoch int
		hash  string
	}{
		{
			epoch: 0,
			hash:  "0xbdae398aa93d3e0593f55180d4d9e14a",
		},
		{
			epoch: 1,
			hash:  "0xfd9ef335c5dc2f3831abe21fa248747b",
		},
		{
			epoch: 100,
			hash:  "0x19bea946d49edfcfc01e34a58495921b",
		},
		{
			epoch: 1000,
			hash:  "0xe42941588426211fb7d56eaba630a687",
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			cache := newCache(c.epoch)
			hash := fnv.New128()
			b := make([]byte, 4)
			for _, i := range cache.cache {
				binary.BigEndian.PutUint32(b, i)
				hash.Write(b)
			}
			assert.Equal(t, c.hash, hex.EncodeToHex(hash.Sum(nil)))
		})
	}
}
