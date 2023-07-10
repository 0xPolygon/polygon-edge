package txpool

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
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
			name:    "sort txs by nonce with base fee",
			baseFee: 1000,
			unsorted: []*types.Transaction{
				// Highest tx fee
				{
					Type:      types.DynamicFeeTx,
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
					Nonce:     3,
				},
				// Lowest tx fee
				{
					Type:      types.DynamicFeeTx,
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
					Nonce:     1,
				},
				// Middle tx fee
				{
					Type:      types.DynamicFeeTx,
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
					Nonce:     2,
				},
			},
			sorted: []*types.Transaction{
				// Highest tx fee
				{
					Type:      types.DynamicFeeTx,
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
					Nonce:     1,
				},
				// Middle tx fee
				{
					Type:      types.DynamicFeeTx,
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
					Nonce:     2,
				},
				// Lowest tx fee
				{
					Type:      types.DynamicFeeTx,
					GasFeeCap: big.NewInt(2000),
					GasTipCap: big.NewInt(500),
					Nonce:     3,
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
		{
			name:     "empty",
			baseFee:  0,
			unsorted: nil,
			sorted:   []*types.Transaction{},
		},
	}

	for _, tt := range testTable {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queue := newPricesQueue(tt.baseFee, tt.unsorted)

			for _, tx := range tt.sorted {
				actual := queue.pop()
				assert.Equal(t, tx, actual)
			}
		})
	}
}

func Benchmark_pricedQueue(t *testing.B) {
	testTable := []struct {
		name        string
		unsortedTxs []*types.Transaction
	}{
		{
			name:        "1000 transactions",
			unsortedTxs: generateTxs(1000),
		},
		{
			name:        "10000 transactions",
			unsortedTxs: generateTxs(10000),
		},
		{
			name:        "100000 transactions",
			unsortedTxs: generateTxs(100000),
		},
	}

	for _, tt := range testTable {
		t.Run(tt.name, func(b *testing.B) {
			for i := 0; i < t.N; i++ {
				q := newPricesQueue(uint64(100), tt.unsortedTxs)

				for q.length() > 0 {
					_ = q.pop()
				}
			}
		})
	}
}

func generateTxs(num int) []*types.Transaction {
	txs := make([]*types.Transaction, num)

	for i := 0; i < num; i++ {
		txs[i] = generateTx(i + 1)
	}

	return txs
}

func generateTx(i int) *types.Transaction {
	s := rand.NewSource(int64(i))
	r := rand.New(s)

	txTypes := []types.TxType{
		types.LegacyTx,
		types.DynamicFeeTx,
	}

	tx := &types.Transaction{
		Type: txTypes[r.Intn(len(txTypes))],
	}

	switch tx.Type {
	case types.LegacyTx:
		minGasPrice := 1000 * i
		maxGasPrice := 100000 * i
		tx.GasPrice = new(big.Int).SetInt64(int64(rand.Intn(maxGasPrice-minGasPrice) + minGasPrice))
	case types.DynamicFeeTx:
		minGasFeeCap := 1000 * i
		maxGasFeeCap := 100000 * i
		tx.GasFeeCap = new(big.Int).SetInt64(int64(rand.Intn(maxGasFeeCap-minGasFeeCap) + minGasFeeCap))

		minGasTipCap := 100 * i
		maxGasTipCap := 10000 * i
		tx.GasTipCap = new(big.Int).SetInt64(int64(rand.Intn(maxGasTipCap-minGasTipCap) + minGasTipCap))
	}

	return tx
}
