package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBlockNumberOrHash_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	var blockHash types.Hash
	err := blockHash.UnmarshalText([]byte("0xe0ee62fd4a39a6988e24df0b406b90af71932e1b01d5561400a8eab943a33d68"))
	assert.NoError(t, err)

	blockNumberZero := BlockNumber(0x0)
	blockNumberLatest := LatestBlockNumber

	tests := []struct {
		name        string
		rawRequest  string
		shouldFail  bool
		expectedBnh BlockNumberOrHash
	}{
		{
			"should return an error for non existing json fields",
			`{"blockHash": "", "blockNumber": ""}`,
			true,
			BlockNumberOrHash{},
		},
		{
			"should return an error for too many json fields",
			`{"blockHash": "0xe0ee62fd4a39a6988e24df0b406b90af71932e1b01d5561400a8eab943a33d68", "blockNumber": "0x0"}`,
			true,
			BlockNumberOrHash{
				BlockNumber: &blockNumberZero,
				BlockHash:   &blockHash,
			},
		},
		{
			"should return an error for invalid block number #1",
			`{"blockNumber": "abc"}`,
			true,
			BlockNumberOrHash{},
		},
		{
			"should return an error for invalid block number #2",
			`{"blockNumber": ""}`,
			true,
			BlockNumberOrHash{},
		},
		{
			"should unmarshal latest block number properly",
			`"latest"`,
			false,
			BlockNumberOrHash{
				BlockNumber: &blockNumberLatest,
			},
		},
		{
			"should unmarshal block number 0 properly #1",
			`{"blockNumber": "0x0"}`,
			false,
			BlockNumberOrHash{
				BlockNumber: &blockNumberZero,
			},
		},
		{
			"should unmarshal block number 0 properly #2",
			`"0x0"`,
			false,
			BlockNumberOrHash{
				BlockNumber: &blockNumberZero,
			},
		},
		{
			"should unmarshal block hash properly",
			`{"blockHash": "0xe0ee62fd4a39a6988e24df0b406b90af71932e1b01d5561400a8eab943a33d68"}`,
			false,
			BlockNumberOrHash{
				BlockHash: &blockHash,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bnh := BlockNumberOrHash{}
			err := bnh.UnmarshalJSON([]byte(tt.rawRequest))

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedBnh.BlockNumber != nil {
					assert.Equal(t, *bnh.BlockNumber, *tt.expectedBnh.BlockNumber)
				}
				if tt.expectedBnh.BlockHash != nil {
					assert.Equal(t, bnh.BlockHash.String(), tt.expectedBnh.BlockHash.String())
				}
			}
		})
	}
}
