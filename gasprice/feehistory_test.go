package gasprice

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGasHelper_FeeHistory(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Name                  string
		ExpectedOldestBlock   uint64
		ExpectedBaseFeePerGas []uint64
		ExpectedGasUsedRatio  []float64
		ExpectedRewards       [][]uint64
		BlockRange            uint64
		NewestBlock           uint64
		RewardPercentiles     []float64
		Error                 bool
		GetBackend            func() Blockchain
	}{
		{
			Name:              "Block does not exist",
			Error:             true,
			BlockRange:        10,
			NewestBlock:       30,
			RewardPercentiles: []float64{15, 20},
			GetBackend: func() Blockchain {
				header := &types.Header{
					Number: 1,
					Hash:   types.StringToHash("some header"),
				}
				backend := new(backendMock)
				backend.On("Header").Return(header)
				backend.On("GetBlockByNumber", mock.Anything, mock.Anything).Return(&types.Block{}, false)

				return backend
			},
		},
		{
			Name:              "Block Range < 1",
			Error:             true,
			BlockRange:        0,
			NewestBlock:       30,
			RewardPercentiles: []float64{10, 15},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 3, 200)

				return backend
			},
		},
		{
			Name:              "Invalid rewardPercentile",
			Error:             true,
			BlockRange:        10,
			NewestBlock:       30,
			RewardPercentiles: []float64{101, 0},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 50)
				createTestTxs(t, backend, 1, 200)

				return backend
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			backend := tc.GetBackend()
			gasHelper, err := NewGasHelper(DefaultGasHelperConfig, backend)
			require.NoError(t, err)
			oldestBlock, baseFeePerGas, gasUsedRatio, rewards, err := gasHelper.FeeHistory(tc.BlockRange, tc.NewestBlock, tc.RewardPercentiles)

			if tc.Error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, oldestBlock == &tc.ExpectedOldestBlock)
				require.True(t, baseFeePerGas == &tc.ExpectedBaseFeePerGas)
				require.True(t, gasUsedRatio == &tc.ExpectedGasUsedRatio)
				require.True(t, rewards == &tc.ExpectedRewards)
			}
		})
	}
}

var _ Blockchain = (*backendMock)(nil)

func (b *backendMock) GetBlockByNumber(number uint64, full bool) (*types.Block, bool) {
	if len(b.blocks) == 0 {
		args := b.Called(number, full)

		return args.Get(0).(*types.Block), args.Get(1).(bool) //nolint:forcetypeassert
	}

	block, exists := b.blocksByNumber[number]

	return block, exists
}
func uint64ToHash(n uint64) types.Hash {
	return types.BytesToHash(big.NewInt(int64(n)).Bytes())
}
