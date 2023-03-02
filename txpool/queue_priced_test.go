package txpool

import (
	"math/big"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/polygon-edge/types"
)

func Test_maxPriceQueue(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name     string
		baseFee  uint64
		unsorted []*types.Transaction
		sorted   []*types.Transaction
	}{
		{
			name:    "sort txs by tips with base fee",
			baseFee: 1000,
			unsorted: []*types.Transaction{
				// Highest tx fee
				{
					Type:      types.DynamicFeeTx,
					GasPrice:  big.NewInt(0),
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
				},
				// Lowest tx fee
				{
					Type:     types.LegacyTx,
					GasPrice: big.NewInt(100),
				},
				// Middle tx fee
				{
					Type:      types.DynamicFeeTx,
					GasPrice:  big.NewInt(0),
					GasFeeCap: big.NewInt(1500),
					GasTipCap: big.NewInt(200),
				},
			},
			sorted: []*types.Transaction{
				// Highest tx fee
				{
					Type:      types.DynamicFeeTx,
					GasPrice:  big.NewInt(0),
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
				},
				// Middle tx fee
				{
					Type:      types.DynamicFeeTx,
					GasPrice:  big.NewInt(0),
					GasFeeCap: big.NewInt(1500),
					GasTipCap: big.NewInt(200),
				},
				// Lowest tx fee
				{
					Type:     types.LegacyTx,
					GasPrice: big.NewInt(100),
				},
			},
		},
		{
			name:    "sort txs without base fee by fee cap",
			baseFee: 0,
			unsorted: []*types.Transaction{
				// Highest tx fee
				{
					GasFeeCap: big.NewInt(3000),
					GasTipCap: big.NewInt(100),
				},
				// Lowest tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(100),
				},
				// Middle tx fee
				{
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(100),
				},
			},
			sorted: []*types.Transaction{
				// Highest tx fee
				{
					GasFeeCap: big.NewInt(3000),
					GasTipCap: big.NewInt(100),
				},
				// Middle tx fee
				{
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(100),
				},
				// Lowest tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(100),
				},
			},
		},
		{
			name:    "sort txs without base fee by tip cap",
			baseFee: 0,
			unsorted: []*types.Transaction{
				// Highest tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(300),
				},
				// Lowest tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(100),
				},
				// Middle tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(200),
				},
			},
			sorted: []*types.Transaction{
				// Highest tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(300),
				},
				// Middle tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(200),
				},
				// Lowest tx fee
				{
					GasFeeCap: big.NewInt(1000),
					GasTipCap: big.NewInt(100),
				},
			},
		},
		{
			name:    "sort txs without base fee by gas price",
			baseFee: 0,
			unsorted: []*types.Transaction{
				// Highest tx fee
				{
					GasPrice: big.NewInt(1000),
				},
				// Lowest tx fee
				{
					GasPrice: big.NewInt(100),
				},
				// Middle tx fee
				{
					GasPrice: big.NewInt(500),
				},
			},
			sorted: []*types.Transaction{
				// Highest tx fee
				{
					GasPrice: big.NewInt(1000),
				},
				// Middle tx fee
				{
					GasPrice: big.NewInt(500),
				},
				// Lowest tx fee
				{
					GasPrice: big.NewInt(100),
				},
			},
		},
	}

	for _, tt := range testTable {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queue := &maxPriceQueue{
				baseFee: tt.baseFee,
				txs:     tt.unsorted,
			}

			sort.Sort(queue)

			for _, tx := range tt.sorted {
				actual := queue.Pop()
				assert.Equal(t, tx, actual)
			}
		})
	}
}
