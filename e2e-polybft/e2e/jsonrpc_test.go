package e2e

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
)

var (
	one = big.NewInt(1)
)

func TestE2E_JsonRPC(t *testing.T) {
	acct, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 3,
		framework.WithPremine(types.Address(acct.Address())),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	client := cluster.Servers[0].JSONRPC().Eth()

	// Test eth_call with override in state diff
	t.Run("eth_call state override", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, acct, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress

		input := abi.MustNewMethod("function getValue() public returns (uint256)").ID()

		resp, err := client.Call(&ethgo.CallMsg{To: &target, Data: input}, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp)

		override := &ethgo.StateOverride{
			target: ethgo.OverrideAccount{
				StateDiff: &map[ethgo.Hash]ethgo.Hash{
					// storage slot 0 stores the 'val' uint256 value
					{0x0}: {0x3},
				},
			},
		}

		resp, err = client.Call(&ethgo.CallMsg{To: &target, Data: input}, ethgo.Latest, override)
		require.NoError(t, err)

		require.Equal(t, "0x0300000000000000000000000000000000000000000000000000000000000000", resp)
	})

	t.Run("eth_getBalance", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		// Test. return zero if the account does not exists
		balance1, err := client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, big.NewInt(0))

		// Test. return the balance of an account
		newBalance := ethgo.Ether(1)
		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), newBalance)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		balance1, err = client.GetBalance(key1.Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, newBalance)

		// Test. return 0 if the balance of an existing account is empty
		gasPrice, err := client.GasPrice()
		require.NoError(t, err)

		toAddr := key1.Address()
		msg := &ethgo.CallMsg{
			From:     acct.Address(),
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
		targetAddr := acct.Address()
		txn = cluster.SendTxn(t, key1, &ethgo.Transaction{
			To:       &targetAddr,
			Value:    amountToSend,
			GasPrice: gasPrice,
			Gas:      estimatedGas,
		})
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

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

		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), big.NewInt(10000000000000000))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		// Test. increase the nonce with new transactions
		txn = cluster.Transfer(t, key1, types.ZeroAddress, big.NewInt(0))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

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

		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		txn = cluster.Deploy(t, acct, contractsapi.TestSimple.Bytecode)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

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
		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), big.NewInt(10000000000000000))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		codeAddr := ethgo.HexToAddress("0xDBfca0c43cA12759256a7Dd587Dc4c6EEC1D89A5")

		// Test. We get empty code from an empty contract
		code, err := client.GetCode(codeAddr, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, code, "0x")

		txn = cluster.Deploy(t, key1, contractsapi.TestSimple.Bytecode)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

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
		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())
		txReceipt := txn.Receipt()

		block, err := client.GetBlockByHash(txReceipt.BlockHash, false)
		require.NoError(t, err)
		require.Equal(t, txReceipt.BlockNumber, block.Number)
		require.Equal(t, txReceipt.BlockHash, block.Hash)
	})

	t.Run("eth_getBlockByNumber", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)
		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())
		txReceipt := txn.Receipt()

		block, err := client.GetBlockByNumber(ethgo.BlockNumber(txReceipt.BlockNumber), false)
		require.NoError(t, err)
		require.Equal(t, txReceipt.BlockNumber, block.Number)
		require.Equal(t, txReceipt.BlockHash, block.Hash)
	})

	t.Run("eth_getTransactionReceipt", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)
		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

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
		txn = cluster.Deploy(t, acct, contractsapi.TestSimple.Bytecode)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		require.NotEqual(t, txn.Receipt().ContractAddress, ethgo.ZeroAddress)
	})

	t.Run("eth_getTransactionByHash", func(t *testing.T) {
		key1, err := wallet.GenerateKey()
		require.NoError(t, err)

		// Test. We should be able to query the transaction by its hash
		txn := cluster.Transfer(t, acct, types.Address(key1.Address()), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		ethTxn, err := client.GetTransactionByHash(txn.Receipt().TransactionHash)
		require.NoError(t, err)

		// Test. The dynamic 'from' field is populated
		require.NotEqual(t, ethTxn.From, ethgo.ZeroAddress)
	})
}
