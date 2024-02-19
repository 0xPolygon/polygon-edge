package polybft

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestBlockBuilder_BuildBlockTxOneFailedTxAndOneTakesTooMuchGas(t *testing.T) {
	t.Parallel()

	const (
		amount        = 1_000
		gasPrice      = 1_000
		gasLimit      = 21000
		blockGasLimit = 21000 * 3
		chainID       = 100
	)

	type account struct {
		privKey *ecdsa.PrivateKey
		address types.Address
	}

	accounts := [6]*account{}

	for i := range accounts {
		ecdsaKey, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		accounts[i] = &account{
			privKey: ecdsaKey,
			address: crypto.PubKeyToAddress(&ecdsaKey.PublicKey),
		}
	}

	forks := &chain.Forks{}
	logger := hclog.NewNullLogger()
	signer := crypto.NewSigner(forks.At(0), chainID)

	mchain := &chain.Chain{
		Params: &chain.Params{
			ChainID: chainID,
			Forks:   forks,
		},
	}

	mstate := itrie.NewState(itrie.NewMemoryStorage())
	executor := state.NewExecutor(mchain.Params, mstate, logger)

	executor.GetHash = func(header *types.Header) func(i uint64) types.Hash {
		return func(i uint64) (res types.Hash) {
			return types.BytesToHash(common.EncodeUint64ToBytes(i))
		}
	}

	balanceMap := map[types.Address]*chain.GenesisAccount{}

	for i, acc := range accounts {
		// the third tx will fail because of insufficient balance
		if i != 2 {
			balanceMap[acc.address] = &chain.GenesisAccount{Balance: ethgo.Ether(1)}
		}
	}

	hash, err := executor.WriteGenesis(balanceMap, types.ZeroHash)

	require.NoError(t, err)
	require.NotEqual(t, types.ZeroHash, hash)

	// Gas Limit is important to be high for tx pool
	parentHeader := &types.Header{StateRoot: hash, GasLimit: 1e15}

	txPool := &txPoolMock{}
	txPool.On("Prepare").Once()

	for i, acc := range accounts {
		gas := uint64(gasLimit)
		// fifth tx will cause filling to stop
		if i == 4 {
			gas = blockGasLimit - 1
		}

		tx := types.NewTx(&types.MixedTxn{
			Value:    big.NewInt(amount),
			GasPrice: big.NewInt(gasPrice),
			Gas:      gas,
			Nonce:    0,
			To:       &acc.address,
		})

		tx, err = signer.SignTx(tx, acc.privKey)
		require.NoError(t, err)

		// all tx until the fifth will be retrieved from the pool
		if i <= 4 {
			txPool.On("Peek").Return(tx).Once()
		}

		// first two and fourth will be added to the block, third will be demoted
		if i == 2 {
			txPool.On("Demote", tx)
		} else if i < 4 {
			txPool.On("Pop", tx)
		}
	}

	bb := NewBlockBuilder(&BlockBuilderParams{
		BlockTime: time.Millisecond * 200,
		Parent:    parentHeader,
		Coinbase:  types.ZeroAddress,
		Executor:  executor,
		GasLimit:  blockGasLimit,
		TxPool:    txPool,
		Logger:    logger,
	})

	require.NoError(t, bb.Reset())

	bb.Fill()

	fb, err := bb.Build(func(h *types.Header) {
		// fake the logs for bloom
		rs := bb.Receipts()

		if len(rs) == 3 {
			rs[0].Logs = []*types.Log{
				{Address: types.StringToAddress("ff7783")},
			}
			rs[1].Logs = []*types.Log{
				{Address: types.StringToAddress("03bbcc")},
				{Address: types.StringToAddress("112233")},
			}
		}
	})
	require.NoError(t, err)

	txPool.AssertExpectations(t)
	require.Len(t, bb.txns, 3, "Should have 3 transactions but has %d", len(bb.txns))
	require.Len(t, bb.Receipts(), 3)

	logsBloom := fb.Block.Header.LogsBloom

	// assert logs bloom
	for _, r := range bb.Receipts() {
		for _, l := range r.Logs {
			assert.True(t, logsBloom.IsLogInBloom(l))
		}
	}

	assert.False(t, logsBloom.IsLogInBloom(
		&types.Log{Address: types.StringToAddress("999911117777")}))
	assert.False(t, logsBloom.IsLogInBloom(
		&types.Log{Address: types.StringToAddress("111177779999")}))
}
