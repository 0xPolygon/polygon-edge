package gasprice

import (
	"math/rand"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
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
			Name:              "blockRange < 1",
			Error:             true,
			BlockRange:        0,
			NewestBlock:       30,
			RewardPercentiles: []float64{10, 15},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)

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
		{
			Name:                "rewardPercentile not set",
			BlockRange:          5,
			NewestBlock:         30,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 26,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 5, 500)

				return backend
			},
		},
		{
			Name:                "blockRange > newestBlock",
			BlockRange:          20,
			NewestBlock:         10,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 1,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 5, 500)

				return backend
			},
		},
		{
			Name:                "blockRange == newestBlock",
			BlockRange:          10,
			NewestBlock:         10,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 1,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee, chain.GenesisBaseFee, chain.GenesisBaseFee,
				chain.GenesisBaseFee, chain.GenesisBaseFee, chain.GenesisBaseFee,
				chain.GenesisBaseFee, chain.GenesisBaseFee, chain.GenesisBaseFee,
				chain.GenesisBaseFee, chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 5, 500)

				return backend
			},
		},
		{
			Name:                "rewardPercentile requested",
			BlockRange:          5,
			NewestBlock:         10,
			RewardPercentiles:   []float64{10, 25},
			ExpectedOldestBlock: 6,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
				{0x2e90edd000, 0x2e90edd000},
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)
				rand.Seed(time.Now().UTC().UnixNano())

				senderKey, sender := tests.GenerateKeyAndAddr(t)

				for _, b := range backend.blocksByNumber {
					signer := crypto.NewSigner(backend.Config().Forks.At(b.Number()),
						uint64(backend.Config().ChainID))

					b.Transactions = make([]*types.Transaction, 3)
					b.Header.Miner = sender.Bytes()

					for i := 0; i < 3; i++ {
						tx := &types.Transaction{
							From:      sender,
							Value:     ethgo.Ether(1),
							To:        &types.ZeroAddress,
							Type:      types.DynamicFeeTx,
							GasTipCap: ethgo.Gwei(uint64(200)),
							GasFeeCap: ethgo.Gwei(uint64(200 + 200)),
						}

						tx, err := signer.SignTx(tx, senderKey)
						require.NoError(t, err)
						b.Transactions[i] = tx
					}
				}

				return backend
			},
		},
		{
			Name:                "BaseFeePerGas sanity check",
			BlockRange:          5,
			NewestBlock:         10,
			RewardPercentiles:   []float64{},
			ExpectedOldestBlock: 6,
			ExpectedBaseFeePerGas: []uint64{
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
				chain.GenesisBaseFee,
			},
			ExpectedGasUsedRatio: []float64{
				0, 0, 0, 0, 0,
			},
			ExpectedRewards: [][]uint64{
				nil, nil, nil, nil, nil,
			},
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)

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
			history, err := gasHelper.FeeHistory(tc.BlockRange, tc.NewestBlock, tc.RewardPercentiles)

			if tc.Error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.ExpectedOldestBlock, history.OldestBlock)
				require.Equal(t, tc.ExpectedBaseFeePerGas, history.BaseFeePerGas)
				require.Equal(t, tc.ExpectedGasUsedRatio, history.GasUsedRatio)
				require.Equal(t, tc.ExpectedRewards, history.Reward)
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
