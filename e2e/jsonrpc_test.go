package e2e

import (
	"context"
	"encoding/hex"

	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

var (
	one = big.NewInt(1)
)

func TestJsonRPC(t *testing.T) {
	fund, err := wallet.GenerateKey()
	require.NoError(t, err)

	bytecode, err := hex.DecodeString(sampleByteCode)
	require.NoError(t, err)

	ibftManager := framework.NewIBFTServersManager(
		t,
		1,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.Premine(types.Address(fund.Address()), ethgo.Ether(10))
			config.SetBlockTime(1)
		},
	)

	ibftManager.StartServers(context.Background())
	defer ibftManager.StopServers()

	srv := ibftManager.GetServer(0)

	client := srv.JSONRPC().Eth()

	t.Run("eth_getBalance", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		// Test. return zero if the account does not exists
		balance1, err := client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, big.NewInt(0))

		// Test. return the balance of an account
		newBalance := ethgo.Ether(1)
		txn, err := srv.Txn(fund).Transfer(key1.Address(), newBalance).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		balance1, err = client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, newBalance)

		// Test. return 0 if the balance of an existing account is empty
		gasPrice, err := client.GasPrice()
		require.NoError(t, err)

		toAddr := key1.Address()
		msg := &ethgo.CallMsg{
			From:     fund.Address(),
			To:       &toAddr,
			Value:    newBalance,
			GasPrice: gasPrice,
		}

		estimatedGas, err := client.EstimateGas(msg)
		require.NoError(t, err)
		txPrice := gasPrice * estimatedGas
		// subtract gasPrice * estimatedGas from the balance and transfer the rest to the other account
		// in order to leave no funds on the account
		amountToSend := new(big.Int).Sub(newBalance, big.NewInt(int64(txPrice)))
		txn, err = srv.Txn(key1).Transfer(fund.Address(), amountToSend).
			GasLimit(estimatedGas).
			Send()
		require.NoError(t, err)
		txn.NoFail(t)

		balance1, err = client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(0), balance1)
	})

	t.Run("eth_getTransactionCount", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		nonce, err := client.GetNonce(key1.Address(), ethgo.Latest)
		require.Equal(t, uint64(0), nonce)
		require.NoError(t, err)

		txn, err := srv.Txn(fund).Transfer(key1.Address(), big.NewInt(10000000000000000)).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		// Test. increase the nonce with new transactions
		txn = srv.Txn(key1)
		txn, err = txn.Send()
		require.NoError(t, err)
		txn.NoFail(t)

		nonce1, err := client.GetNonce(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(1))

		// Test. you can query the nonce at any block number in time
		nonce1, err = client.GetNonce(key1.Address(), ethgo.BlockNumber(txn.Receipt().BlockNumber-1))
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(0))

		block, err := client.GetBlockByNumber(ethgo.BlockNumber(txn.Receipt().BlockNumber)-1, false)
		require.NoError(t, err)

		_, err = client.GetNonce(key1.Address(), ethgo.BlockNumber(block.Number))
		require.NoError(t, err)
	})

	t.Run("eth_getStorage", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		txn := srv.Txn(fund)
		txn, err = txn.Transfer(key1.Address(), one).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		txn = srv.Txn(fund)
		txn, err = txn.Deploy(bytecode).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		resp, err := client.GetStorageAt(txn.Receipt().ContractAddress, ethgo.Hash{}, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp.String())
	})

	t.Run("eth_getCode", func(t *testing.T) {
		// we use a predefined private key so that the deployed contract address is deterministic.
		// Note that in order to work, this private key should only be used for this test.
		priv, err := hex.DecodeString("2c15bd0dc992a47ca660983ae4b611f4ffb6178e14e04e2b34d153f3a74ce741")
		require.NoError(t, err)
		key1, err := wallet.NewWalletFromPrivKey(priv)
		require.NoError(t, err)

		// fund the account so that it can deploy a contract
		txn, err := srv.Txn(fund).Transfer(key1.Address(), big.NewInt(10000000000000000)).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		codeAddr := ethgo.HexToAddress("0xDBfca0c43cA12759256a7Dd587Dc4c6EEC1D89A5")

		// Test. We get empty code from an empty contract
		code, err := client.GetCode(codeAddr, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, code, "0x")

		txn = srv.Txn(key1)
		txn, err = txn.Deploy(bytecode).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		receipt := txn.Receipt()

		// Test. The deployed address is the one expected
		require.Equal(t, codeAddr, receipt.ContractAddress)

		// Test. We can retrieve the code by address on latest, block number and block hash
		cases := []ethgo.BlockNumberOrHash{
			ethgo.Latest,
			ethgo.BlockNumber(receipt.BlockNumber),
		}
		for _, c := range cases {
			code, err = client.GetCode(codeAddr, c)
			require.NoError(t, err)
			require.NotEqual(t, code, "0x")
		}

		// Test. We can query in past state (when the code was empty)
		code, err = client.GetCode(codeAddr, ethgo.BlockNumber(receipt.BlockNumber-1))
		require.NoError(t, err)
		require.Equal(t, code, "0x")

		// Test. Query by pending should default to latest
		code, err = client.GetCode(codeAddr, ethgo.Pending)
		require.NoError(t, err)
		require.NotEqual(t, code, "0x")
	})

	t.Run("eth_getBlockByHash", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)
		txn := srv.Txn(fund)
		txn, err = txn.Transfer(key1.Address(), one).Send()
		require.NoError(t, err)
		txn.NoFail(t)
		txReceipt := txn.Receipt()

		block, err := client.GetBlockByHash(txReceipt.BlockHash, false)
		require.NoError(t, err)
		require.Equal(t, txReceipt.BlockNumber, block.Number)
		require.Equal(t, txReceipt.BlockHash, block.Hash)
	})

	t.Run("eth_getBlockByNumber", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)
		txn := srv.Txn(fund)
		txn, err = txn.Transfer(key1.Address(), one).Send()
		require.NoError(t, err)
		txn.NoFail(t)
		txReceipt := txn.Receipt()

		block, err := client.GetBlockByNumber(ethgo.BlockNumber(txReceipt.BlockNumber), false)
		require.NoError(t, err)
		require.Equal(t, txReceipt.BlockNumber, block.Number)
		require.Equal(t, txReceipt.BlockHash, block.Hash)
	})

	t.Run("eth_getTransactionReceipt", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		txn := srv.Txn(fund)
		txn, err = txn.Transfer(key1.Address(), one).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		// Test. We cannot retrieve a receipt of an empty hash
		emptyReceipt, err := client.GetTransactionReceipt(ethgo.ZeroHash)
		require.NoError(t, err) // Note that ethgo does not return an error when the item does not exists
		require.Nil(t, emptyReceipt)

		// Test. We can retrieve the receipt by the hash
		receipt, err := client.GetTransactionReceipt(txn.Receipt().TransactionHash)
		require.NoError(t, err)

		// Test. The populated fields match with the block
		block, err := client.GetBlockByHash(receipt.BlockHash, false)
		require.NoError(t, err)

		require.Equal(t, receipt.TransactionHash, txn.Receipt().TransactionHash)
		require.Equal(t, receipt.BlockNumber, block.Number)
		require.Equal(t, receipt.BlockHash, block.Hash)

		// Test. The receipt of a deployed contract has the 'ContractAddress' field.
		txn = srv.Txn(fund)
		txn, err = txn.Deploy(bytecode).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		require.NotEqual(t, txn.Receipt().ContractAddress, ethgo.ZeroAddress)
	})

	t.Run("eth_getTransactionByHash", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		// Test. We should be able to query the transaction by its hash
		txn := srv.Txn(fund)
		txn, err = txn.Transfer(key1.Address(), one).Send()
		require.NoError(t, err)
		txn.NoFail(t)

		ethTxn, err := client.GetTransactionByHash(txn.Receipt().TransactionHash)
		require.NoError(t, err)

		// Test. The dynamic 'from' field is populated
		require.NotEqual(t, ethTxn.From, ethgo.ZeroAddress)
	})
}
