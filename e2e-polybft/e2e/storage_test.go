package e2e

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestE2E_Storage(t *testing.T) {
	// premine an account in the genesis file
	sender, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithPremine(types.Address(sender.Address())),
		framework.WithBurnContract(&polybft.BurnContractInfo{BlockNumber: 0, Address: types.ZeroAddress}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	client := cluster.Servers[0].JSONRPC().Eth()

	num := 20

	receivers := []ethgo.Address{}

	for i := 0; i < num; i++ {
		key, err := wallet.GenerateKey()
		require.NoError(t, err)

		receivers = append(receivers, key.Address())
	}

	txs := []*framework.TestTxn{}

	for i := 0; i < num; i++ {
		func(i int, to ethgo.Address) {
			txn := &ethgo.Transaction{
				From:  sender.Address(),
				To:    &to,
				Gas:   30000, // enough to send a transfer
				Value: big.NewInt(int64(i)),
				Nonce: uint64(i),
			}

			// Send every second transaction as a dynamic fees one
			if i%2 == 0 {
				txn.Type = ethgo.TransactionDynamicFee
				txn.MaxFeePerGas = big.NewInt(1000000000)
				txn.MaxPriorityFeePerGas = big.NewInt(100000000)

				chainID, err := client.ChainID()
				require.NoError(t, err)

				txn.ChainID = chainID
			} else {
				txn.Type = ethgo.TransactionLegacy
				txn.GasPrice = ethgo.Gwei(2).Uint64()
			}

			tx := cluster.SendTxn(t, sender, txn)
			err = tx.Wait()
			require.NoError(t, err)

			txs = append(txs, tx)
		}(i, receivers[i])
	}

	err = cluster.WaitUntil(2*time.Minute, 2*time.Second, func() bool {
		for i, receiver := range receivers {
			balance, err := client.GetBalance(receiver, ethgo.Latest)
			if err != nil {
				return true
			}

			t.Logf("Balance %s %s", receiver, balance)

			if balance.Uint64() != uint64(i) {
				return false
			}
		}

		return true
	})
	require.NoError(t, err)

	checkStorage(t, txs, client)
}

func checkStorage(t *testing.T, txs []*framework.TestTxn, client *jsonrpc.Eth) {
	t.Helper()

	for i, tx := range txs {
		bn, err := client.GetBlockByNumber(ethgo.BlockNumber(tx.Receipt().BlockNumber), true)
		require.NoError(t, err)
		assert.NotNil(t, bn)

		bh, err := client.GetBlockByHash(bn.Hash, true)
		require.NoError(t, err)
		assert.NotNil(t, bh)

		if !reflect.DeepEqual(bn, bh) {
			t.Fatal("blocks dont match")
		}

		bt, err := client.GetTransactionByHash(tx.Receipt().TransactionHash)
		require.NoError(t, err)
		assert.NotNil(t, bt)
		assert.Equal(t, tx.Txn().Value.Uint64(), bt.Value.Uint64())
		assert.Equal(t, tx.Txn().Gas, bt.Gas)
		assert.Equal(t, tx.Txn().Nonce, bt.Nonce)
		assert.Equal(t, tx.Receipt().TransactionIndex, bt.TxnIndex)
		assert.NotEmpty(t, bt.V)
		assert.NotEmpty(t, bt.R)
		assert.NotEmpty(t, bt.S)
		assert.Equal(t, tx.Txn().From, bt.From)
		assert.Equal(t, tx.Txn().To, bt.To)

		if i%2 == 0 {
			assert.Equal(t, ethgo.TransactionDynamicFee, bt.Type)
			assert.Equal(t, uint64(0), bt.GasPrice)
			assert.NotNil(t, bt.ChainID)
		} else {
			// assert.Equal(t, ethgo.TransactionLegacy, bt.Type)
			assert.Equal(t, ethgo.Gwei(2).Uint64(), bt.GasPrice)
		}

		r, err := client.GetTransactionReceipt(tx.Receipt().TransactionHash)
		require.NoError(t, err)
		assert.NotNil(t, r)
		assert.Equal(t, bt.TxnIndex, r.TransactionIndex)
		assert.Equal(t, bt.Hash, r.TransactionHash)
		assert.Equal(t, bt.BlockHash, r.BlockHash)
		assert.Equal(t, bt.BlockNumber, r.BlockNumber)
		assert.NotEmpty(t, r.LogsBloom)
		assert.Equal(t, bt.To, r.To)
	}
}
