package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

func TestJsonRPC(t *testing.T) {
	// TODO: Reuse the same tests for websockets and IPC.
	fund, err := wallet.GenerateKey()
	require.NoError(t, err)

	ibftManager := framework.NewIBFTServersManager(
		t,
		1,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.Premine(types.Address(fund.Address()), framework.EthToWei(10))
			config.SetBlockTime(1)
		},
	)

	ibftManager.StartServers(context.Background())
	defer ibftManager.StopServers()

	srv := ibftManager.GetServer(0)
	client := srv.JSONRPC().Eth()

	t.Run("eth_getBalance", func(t *testing.T) {
		key1, _ := wallet.GenerateKey()

		// 1. return zero if the account does not exists
		balance1, err := client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, big.NewInt(0))

		// 2. return the balance of an account
		newBalance := big.NewInt(22000)

		receipt, err := srv.Transfer(fund, key1.Address(), newBalance)
		require.NoError(t, err)
		require.Equal(t, receipt.Status, uint64(1))

		balance1, err = client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, newBalance)

		// 3. return 0 if the balance of an existing account is empty
		// TODO
	})

	t.Run("eth_getTransactionCount", func(t *testing.T) {
		key1, _ := wallet.GenerateKey()
		srv.Transfer(fund, key1.Address(), big.NewInt(10000000000000000))

		// 1. return zero if the account does not exists
		nonce1, err := client.GetNonce(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(0))

		// 2. increase the nonce with new transactions
		receipt, err := srv.Transfer(key1, ethgo.Address{}, big.NewInt(1))
		require.NoError(t, err)
		require.Equal(t, receipt.Status, uint64(1))

		nonce1, err = client.GetNonce(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(1))

		// 3. you can query the nonce at any block number in time
		nonce1, err = client.GetNonce(key1.Address(), ethgo.BlockNumber(receipt.BlockNumber-1))
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(0))

		// 4. TODO. you can query the nonce at any block has in time
		block, err := client.GetBlockByNumber(ethgo.BlockNumber(receipt.BlockNumber)-1, false)
		require.NoError(t, err)
		client.GetNonce(key1.Address(), block.Hash)
	})

	t.Run("eth_getStorage", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("eth_getCode", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("eth_getBlockByX", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("eth_getTransactionReceipt", func(t *testing.T) {
		t.Skip("TODO")
	})
}
