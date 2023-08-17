package jsonrpc

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func FuzzBlockNumberOrHashUnmarshalJSON(f *testing.F) {
	var blockHash types.Hash
	err := blockHash.UnmarshalText([]byte("0xe0ee62fd4a39a6988e24df0b406b90af71932e1b01d5561400a8eab943a33d68"))
	assert.NoError(f, err)

	seeds := []string{

		`{"blockHash": "", "blockNumber": ""}`,

		`{"blockHash": "0xe0ee62fd4a39a6988e24df0b406b90af71932e1b01d5561400a8eab943a33d68", "blockNumber": "0x0"}`,

		`{"blockNumber": "abc"}`,

		`{"blockNumber": ""}`,

		`"latest"`,

		`{"blockNumber": "0x0"}`,

		`"0x0"`,

		`{"blockHash": "0xe0ee62fd4a39a6988e24df0b406b90af71932e1b01d5561400a8eab943a33d68"}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		t.Parallel()

		bnh := BlockNumberOrHash{}
		_ = bnh.UnmarshalJSON([]byte(input))
	})
}
