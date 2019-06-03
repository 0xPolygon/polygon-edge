package ethash

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/minimal/helper/hex"
)

func TestSizes(t *testing.T) {
	t.Run("cache", func(t *testing.T) {
		for epoch, val := range cacheSizes {
			assert.Equal(t, getCacheSizeByEpoch(epoch), val)
		}
	})
	t.Run("dataset", func(t *testing.T) {
		for epoch, val := range datasetSizes {
			assert.Equal(t, getDatasetSizeByEpoch(epoch), val)
		}
	})
}

func TestBlockSeed(t *testing.T) {
	cases := []struct {
		block int
		seed  string
	}{
		{
			block: 0,
			seed:  "0x0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			block: 30000,
			seed:  "0x290decd9548b62a8d60345a988386fc84ba6bc95484008f6362f93160ef3e563",
		},
		{
			block: 60000,
			seed:  "0x510e4e770828ddbf7f7b00ab00a9f6adaf81c0dc9cc85f1f8249c256942d61d9",
		},
		{
			block: 80000,
			seed:  "0x510e4e770828ddbf7f7b00ab00a9f6adaf81c0dc9cc85f1f8249c256942d61d9",
		},
		{
			block: 90000,
			seed:  "0x356e5a2cc1eba076e650ac7473fccc37952b46bc2e419a200cec0c451dce2336",
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, c.seed, hex.EncodeToHex(getSeedHash(c.block)))
			assert.Equal(t, c.seed, hex.EncodeToHex(getSeedHashByEpoch(c.block/epochLength)))
		})
	}
}
