package gasprice

import (
	"fmt"
	"math/big"
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

func TestGasHelper_MaxPriorityFeePerGas(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Name       string
		Expected   *big.Int
		Error      bool
		GetBackend func() Blockchain
	}{
		{
			Name:     "Chain just started",
			Expected: DefaultGasHelperConfig.LastPrice,
			GetBackend: func() Blockchain {
				genesis := &types.Header{
					Number: 0,
					Hash:   types.StringToHash("genesis"),
				}
				backend := new(backendMock)
				backend.On("Header").Return(genesis)
				backend.On("GetBlockByHash", mock.Anything, true).Return(&types.Block{
					Header:       genesis,
					Transactions: []*types.Transaction{},
				}, true)

				return backend
			},
		},
		{
			Name:  "Block does not exist",
			Error: true,
			GetBackend: func() Blockchain {
				header := &types.Header{
					Number: 0,
					Hash:   types.StringToHash("some header"),
				}
				backend := new(backendMock)
				backend.On("Header").Return(header)
				backend.On("GetBlockByHash", mock.Anything, true).Return(&types.Block{}, false)

				return backend
			},
		},
		{
			Name:     "Empty blocks",
			Expected: DefaultGasHelperConfig.LastPrice, // should return last (default) price
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)

				return backend
			},
		},
		{
			Name:     "All transactions by miner",
			Expected: DefaultGasHelperConfig.LastPrice, // should return last (default) price
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)
				rand.Seed(time.Now().UTC().UnixNano())

				senderKey, sender := tests.GenerateKeyAndAddr(t)

				for _, b := range backend.blocks {
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
							GasTipCap: ethgo.Gwei(uint64(rand.Intn(200))),
							GasFeeCap: ethgo.Gwei(uint64(rand.Intn(200) + 200)),
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
			Name:     "All transactions have small effective tip",
			Expected: DefaultGasHelperConfig.LastPrice, // should return last (default) price
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)
				createTestTxs(t, backend, 3, 1)

				return backend
			},
		},
		{
			Name: "Number of blocks in chain smaller than numOfBlocksToCheck",
			Expected: DefaultGasHelperConfig.LastPrice.Mul(
				DefaultGasHelperConfig.LastPrice, big.NewInt(2)), // at least two times of default last price
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 10)
				createTestTxs(t, backend, 3, 200)

				return backend
			},
		},
		{
			Name: "Number of blocks in chain higher than numOfBlocksToCheck",
			Expected: DefaultGasHelperConfig.LastPrice.Mul(
				DefaultGasHelperConfig.LastPrice, big.NewInt(2)), // at least two times of default last price
			GetBackend: func() Blockchain {
				backend := createTestBlocks(t, 30)
				createTestTxs(t, backend, 3, 200)

				return backend
			},
		},
		{
			Name: "Not enough transactions in first 20 blocks, so read some more blocks",
			Expected: DefaultGasHelperConfig.LastPrice.Mul(
				DefaultGasHelperConfig.LastPrice, big.NewInt(2)), // at least two times of default last price
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
			price, err := gasHelper.MaxPriorityFeePerGas()

			if tc.Error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, price.Cmp(tc.Expected) >= 0)
			}
		})
	}
}

func createTestBlocks(t *testing.T, numOfBlocks int) *backendMock {
	t.Helper()

	backend := &backendMock{blocks: make(map[types.Hash]*types.Block), blocksByNumber: make(map[uint64]*types.Block)}
	genesis := &types.Block{
		Header: &types.Header{
			Number:  0,
			Hash:    types.StringToHash("genesis"),
			Miner:   types.ZeroAddress.Bytes(),
			BaseFee: chain.GenesisBaseFee,
		},
	}
	backend.blocks[genesis.Hash()] = genesis
	backend.blocksByNumber[genesis.Number()] = genesis

	currentBlock := genesis

	for i := 1; i <= numOfBlocks; i++ {
		block := &types.Block{
			Header: &types.Header{
				Number:     uint64(i),
				Hash:       types.BytesToHash([]byte(fmt.Sprintf("Block %d", i))),
				Miner:      types.ZeroAddress.Bytes(),
				ParentHash: currentBlock.Hash(),
				BaseFee:    chain.GenesisBaseFee,
			},
		}
		backend.blocksByNumber[block.Number()] = block
		backend.blocks[block.Hash()] = block
		currentBlock = block
	}

	backend.On("Header").Return(currentBlock.Header)

	return backend
}

func createTestTxs(t *testing.T, backend *backendMock, numOfTxsPerBlock, txCap int) {
	t.Helper()

	rand.Seed(time.Now().UTC().UnixNano())

	for _, b := range backend.blocks {
		signer := crypto.NewSigner(backend.Config().Forks.At(b.Number()),
			uint64(backend.Config().ChainID))

		b.Transactions = make([]*types.Transaction, numOfTxsPerBlock)

		for i := 0; i < numOfTxsPerBlock; i++ {
			senderKey, sender := tests.GenerateKeyAndAddr(t)

			tx := &types.Transaction{
				From:      sender,
				Value:     ethgo.Ether(1),
				To:        &types.ZeroAddress,
				Type:      types.DynamicFeeTx,
				GasTipCap: ethgo.Gwei(uint64(rand.Intn(txCap))),
				GasFeeCap: ethgo.Gwei(uint64(rand.Intn(txCap) + txCap)),
			}

			tx, err := signer.SignTx(tx, senderKey)
			require.NoError(t, err)

			b.Transactions[i] = tx
		}
	}
}

var _ Blockchain = (*backendMock)(nil)

type backendMock struct {
	mock.Mock
	blocks         map[types.Hash]*types.Block
	blocksByNumber map[uint64]*types.Block
}

func (b *backendMock) Header() *types.Header {
	args := b.Called()

	return args.Get(0).(*types.Header) //nolint:forcetypeassert
}

func (b *backendMock) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	if len(b.blocks) == 0 {
		args := b.Called(hash, full)

		return args.Get(0).(*types.Block), args.Get(1).(bool) //nolint:forcetypeassert
	}

	block, exists := b.blocks[hash]

	return block, exists
}

func (b *backendMock) Config() *chain.Params {
	return &chain.Params{
		ChainID: 1,
		Forks:   chain.AllForksEnabled,
	}
}
