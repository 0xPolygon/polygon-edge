package ethash

import (
	"encoding/binary"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/minimal/helper/hex"
)

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
				for _, j := range i {
					binary.BigEndian.PutUint32(b, j)
					hash.Write(b)
				}
			}
			assert.Equal(t, c.hash, hex.EncodeToHex(hash.Sum(nil)))
		})
	}
}
