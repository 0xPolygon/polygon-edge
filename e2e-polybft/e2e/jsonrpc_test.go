package e2e

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	bladeRPC "github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	one = big.NewInt(1)
)

func TestE2E_JsonRPC(t *testing.T) {
	senderKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithPremine(senderKey.Address()),
		framework.WithHTTPS("/etc/ssl/certs/localhost.pem", "/etc/ssl/private/localhost.key"),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	jsonRPC := cluster.Servers[0].JSONRPC()
	client := jsonRPC.Eth()
	debug := jsonRPC.Debug()

	// Test eth_call with override in state diff
	t.Run("eth_call state override", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, senderKey, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress

		input := contractsapi.TestSimple.Abi.GetMethod("getValue").ID()

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

	// Test eth_call with zero account balance
	t.Run("eth_call with zero-balance account", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, senderKey, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress

		input := contractsapi.TestSimple.Abi.GetMethod("getValue").ID()

		acctZeroBalance, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		resp, err := client.Call(&ethgo.CallMsg{
			From: ethgo.Address(acctZeroBalance.Address()),
			To:   &target,
			Data: input,
		}, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp)
	})

	t.Run("eth_estimateGas", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, senderKey, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress

		input := contractsapi.TestSimple.Abi.GetMethod("getValue").ID()

		estimatedGas, err := client.EstimateGas(&ethgo.CallMsg{
			From: ethgo.Address(senderKey.Address()),
			To:   &target,
			Data: input,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(0x5bb7), estimatedGas)
	})

	t.Run("eth_estimateGas by zero-balance account", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, senderKey, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress
		input := contractsapi.TestSimple.Abi.GetMethod("getValue").ID()

		acctZeroBalance, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		resp, err := client.EstimateGas(&ethgo.CallMsg{
			From: ethgo.Address(acctZeroBalance.Address()),
			To:   &target,
			Data: input,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(0x5bb7), resp)
	})

	t.Run("eth_estimateGas by zero-balance account - simple value transfer", func(t *testing.T) {
		acctZeroBalance, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		fundedAccountAddress := senderKey.Address()
		nonFundedAccountAddress := acctZeroBalance.Address()

		estimateGasFn := func(value *big.Int) {
			resp, err := client.EstimateGas(&ethgo.CallMsg{
				From:  ethgo.Address(nonFundedAccountAddress),
				To:    (*ethgo.Address)(&fundedAccountAddress),
				Value: value,
			})

			require.NoError(t, err)
			require.Equal(t, state.TxGas, resp)
		}

		estimateGasFn(ethgo.Gwei(1))

		// transfer some funds to zero balance account
		valueTransferTxn := cluster.SendTxn(t, senderKey,
			types.NewTx(types.NewLegacyTx(
				types.WithFrom(fundedAccountAddress),
				types.WithTo(&nonFundedAccountAddress),
				types.WithValue(ethgo.Gwei(10)),
			)))

		require.NoError(t, valueTransferTxn.Wait())
		require.True(t, valueTransferTxn.Succeed())

		// now call estimate gas again for the now funded account
		estimateGasFn(ethgo.Gwei(1))
	})

	t.Run("eth_getBalance", func(t *testing.T) {
		recipientKey, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		recipientAddr := ethgo.Address(recipientKey.Address())

		// Test. return zero if the account does not exist
		balance1, err := client.GetBalance(recipientAddr, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, big.NewInt(0))

		// Test. return the balance of an account
		newBalance := ethgo.Ether(1)
		txn := cluster.Transfer(t, senderKey, recipientKey.Address(), newBalance)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		balance1, err = client.GetBalance(recipientAddr, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, balance1, newBalance)

		// Test. return 0 if the balance of an existing account is empty
		gasPrice, err := client.GasPrice()
		require.NoError(t, err)

		estimatedGas, err := client.EstimateGas(&ethgo.CallMsg{
			From:  ethgo.Address(senderKey.Address()),
			To:    &recipientAddr,
			Value: newBalance,
		})
		require.NoError(t, err)

		txPrice := gasPrice * estimatedGas
		// subtract gasPrice * estimatedGas from the balance and transfer the rest to the other account
		// in order to leave no funds on the account
		amountToSend := new(big.Int).Sub(newBalance, big.NewInt(int64(txPrice)))
		targetAddr := senderKey.Address()
		txn = cluster.SendTxn(t, recipientKey,
			types.NewTx(types.NewLegacyTx(
				types.WithTo(&targetAddr),
				types.WithValue(amountToSend),
				types.WithGas(estimatedGas),
			)))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		balance1, err = client.GetBalance(ethgo.Address(recipientKey.Address()), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(0), balance1)
	})

	t.Run("eth_getTransactionCount", func(t *testing.T) {
		recipientKey, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		recipient := ethgo.Address(recipientKey.Address())

		nonce, err := client.GetNonce(recipient, ethgo.Latest)
		require.Equal(t, uint64(0), nonce)
		require.NoError(t, err)

		txn := cluster.Transfer(t, senderKey, types.Address(recipient), big.NewInt(10000000000000000))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		// Test. increase the nonce with new transactions
		txn = cluster.Transfer(t, recipientKey, types.ZeroAddress, big.NewInt(0))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		nonce1, err := client.GetNonce(recipient, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(1))

		// Test. you can query the nonce at any block number in time
		nonce1, err = client.GetNonce(recipient, ethgo.BlockNumber(txn.Receipt().BlockNumber-1))
		require.NoError(t, err)
		require.Equal(t, nonce1, uint64(0))

		block, err := client.GetBlockByNumber(ethgo.BlockNumber(txn.Receipt().BlockNumber)-1, false)
		require.NoError(t, err)

		_, err = client.GetNonce(recipient, ethgo.BlockNumber(block.Number))
		require.NoError(t, err)
	})

	t.Run("eth_getStorageAt", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		txn = cluster.Deploy(t, senderKey, contractsapi.TestSimple.Bytecode)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		target := txn.Receipt().ContractAddress

		resp, err := client.GetStorageAt(target, ethgo.Hash{}, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp.String())

		setValueFn := contractsapi.TestSimple.Abi.GetMethod("setValue")

		newVal := big.NewInt(1)

		input, err := setValueFn.Encode([]interface{}{newVal})
		require.NoError(t, err)

		txn = cluster.SendTxn(t, senderKey,
			types.NewTx(
				types.NewLegacyTx(types.WithInput(input), types.WithTo((*types.Address)(&target))),
			))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		resp, err = client.GetStorageAt(target, ethgo.Hash{}, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000001", resp.String())
	})

	t.Run("eth_getCode", func(t *testing.T) {
		// we use a predefined private key so that the deployed contract address is deterministic.
		// Note that in order to work, this private key should only be used for this test.
		priv, err := hex.DecodeString("2c15bd0dc992a47ca660983ae4b611f4ffb6178e14e04e2b34d153f3a74ce741")
		require.NoError(t, err)
		key1, err := crypto.NewECDSAKeyFromRawPrivECDSA(priv)
		require.NoError(t, err)

		// fund the account so that it can deploy a contract
		txn := cluster.Transfer(t, senderKey, key1.Address(), ethgo.Ether(1))
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
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)
		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())
		txReceipt := txn.Receipt()

		block, err := client.GetBlockByHash(txReceipt.BlockHash, false)
		require.NoError(t, err)
		require.Equal(t, txReceipt.BlockNumber, block.Number)
		require.Equal(t, txReceipt.BlockHash, block.Hash)
	})

	t.Run("eth_getBlockByNumber", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)
		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())
		txReceipt := txn.Receipt()

		block, err := client.GetBlockByNumber(ethgo.BlockNumber(txReceipt.BlockNumber), false)
		require.NoError(t, err)
		require.Equal(t, txReceipt.BlockNumber, block.Number)
		require.Equal(t, txReceipt.BlockHash, block.Hash)
	})

	t.Run("eth_getTransactionReceipt", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)
		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
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
		txn = cluster.Deploy(t, senderKey, contractsapi.TestSimple.Bytecode)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		require.NotEqual(t, txn.Receipt().ContractAddress, ethgo.ZeroAddress)
	})

	t.Run("eth_getTransactionByHash", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		// Test. We should be able to query the transaction by its hash
		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		ethTxn, err := client.GetTransactionByHash(txn.Receipt().TransactionHash)
		require.NoError(t, err)

		// Test. The dynamic 'from' field is populated
		require.NotEqual(t, ethTxn.From, ethgo.ZeroAddress)
	})

	t.Run("eth_getHeaderByNumber", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())
		txReceipt := txn.Receipt()

		var header types.Header
		err = jsonRPC.Call("eth_getHeaderByNumber", &header, ethgo.BlockNumber(txReceipt.BlockNumber))
		require.NoError(t, err)

		require.Equal(t, txReceipt.BlockNumber, header.Number)
		require.Equal(t, txReceipt.BlockHash, ethgo.Hash(header.Hash))
	})

	t.Run("eth_getHeaderByHash", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())
		txReceipt := txn.Receipt()

		var header types.Header
		err = jsonRPC.Call("eth_getHeaderByHash", &header, txReceipt.BlockHash)
		require.NoError(t, err)

		require.Equal(t, txReceipt.BlockNumber, header.Number)
		require.Equal(t, txReceipt.BlockHash, ethgo.Hash(header.Hash))
	})

	t.Run("debug_traceTransaction", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		txn := cluster.Transfer(t, senderKey, key1.Address(), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		txReceipt := txn.Receipt()

		// Use a wrapper function from "jsonrpc" package when the config is introduced.
		trace, err := debug.TraceTransaction(txReceipt.TransactionHash, jsonrpc.TraceTransactionOptions{})
		require.NoError(t, err)
		require.Equal(t, txReceipt.GasUsed, trace.Gas)
	})
}

func TestE2E_JsonRPC_NewEthClient(t *testing.T) {
	const epochSize = uint64(5)

	preminedAcct, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithEpochSize(int(epochSize)),
		framework.WithPremine(preminedAcct.Address()),
		framework.WithBurnContract(&polybft.BurnContractInfo{BlockNumber: 0, Address: types.ZeroAddress}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	newEthClient, err := bladeRPC.NewEthClient(cluster.Servers[0].JSONRPCAddr())
	require.NoError(t, err)

	t.Run("eth_blockNumber", func(t *testing.T) {
		require.NoError(t, cluster.WaitForBlock(epochSize, 15*time.Second))

		blockNumber, err := newEthClient.BlockNumber()
		require.NoError(t, err)
		require.GreaterOrEqual(t, blockNumber, epochSize)

		require.NoError(t, cluster.WaitForBlock(blockNumber+1, 5*time.Second))

		blockNumber, err = newEthClient.BlockNumber()
		require.NoError(t, err)
		require.GreaterOrEqual(t, blockNumber, epochSize)
	})

	t.Run("eth_getBlock", func(t *testing.T) {
		blockByNumber, err := newEthClient.GetBlockByNumber(bladeRPC.BlockNumber(epochSize), false)
		require.NoError(t, err)
		require.NotNil(t, blockByNumber)
		require.Equal(t, epochSize, blockByNumber.Number())
		require.Empty(t, len(blockByNumber.Transactions)) // since we did not ask for the full block

		blockByNumber, err = newEthClient.GetBlockByNumber(bladeRPC.BlockNumber(epochSize), true)
		require.NoError(t, err)
		require.Equal(t, epochSize, blockByNumber.Number())
		// since we asked for the full block, and epoch ending block has a transaction
		require.Equal(t, 1, len(blockByNumber.Transactions))

		blockByHash, err := newEthClient.GetBlockByHash(blockByNumber.Hash(), false)
		require.NoError(t, err)
		require.NotNil(t, blockByHash)
		require.Equal(t, epochSize, blockByHash.Number())
		require.Equal(t, blockByNumber.Hash(), blockByHash.Hash())

		blockByHash, err = newEthClient.GetBlockByHash(blockByNumber.Hash(), true)
		require.NoError(t, err)
		require.Equal(t, blockByNumber.Hash(), blockByHash.Hash())
		// since we asked for the full block, and epoch ending block has a transaction
		require.Equal(t, 1, len(blockByHash.Transactions))

		// get latest block
		latestBlock, err := newEthClient.GetBlockByNumber(bladeRPC.LatestBlockNumber, false)
		require.NoError(t, err)
		require.NotNil(t, latestBlock)
		require.GreaterOrEqual(t, latestBlock.Number(), epochSize)

		// get pending block
		pendingBlock, err := newEthClient.GetBlockByNumber(bladeRPC.PendingBlockNumber, false)
		require.NoError(t, err)
		require.NotNil(t, pendingBlock)
		require.GreaterOrEqual(t, pendingBlock.Number(), latestBlock.Number())

		// get earliest block
		earliestBlock, err := newEthClient.GetBlockByNumber(bladeRPC.EarliestBlockNumber, false)
		require.NoError(t, err)
		require.NotNil(t, earliestBlock)
		require.Equal(t, uint64(0), earliestBlock.Number())
	})

	t.Run("eth_getCode", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, preminedAcct, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress

		code, err := newEthClient.GetCode(types.Address(target), bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.NotEmpty(t, code)
	})

	t.Run("eth_getStorageAt", func(t *testing.T) {
		key1, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		txn := cluster.Transfer(t, preminedAcct, key1.Address(), ethgo.Ether(1))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		txn = cluster.Deploy(t, key1, contractsapi.TestSimple.Bytecode)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		target := types.Address(txn.Receipt().ContractAddress)

		resp, err := newEthClient.GetStorageAt(target, types.Hash{}, bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp.String())

		setValueFn := contractsapi.TestSimple.Abi.GetMethod("setValue")

		newVal := big.NewInt(1)

		input, err := setValueFn.Encode([]interface{}{newVal})
		require.NoError(t, err)

		txn = cluster.SendTxn(t, key1, types.NewTx(types.NewLegacyTx(types.WithInput(input), types.WithTo(&target))))
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		resp, err = newEthClient.GetStorageAt(types.Address(target), types.Hash{}, bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000001", resp.String())
	})

	t.Run("eth_getTransactionByHash and eth_getTransactionReceipt", func(t *testing.T) {
		txn := cluster.Transfer(t, preminedAcct, types.StringToAddress("0xDEADBEEF"), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		ethTxn, err := newEthClient.GetTransactionByHash(types.Hash(txn.Receipt().TransactionHash))
		require.NoError(t, err)

		require.Equal(t, ethTxn.From(), preminedAcct.Address())

		receipt, err := newEthClient.GetTransactionReceipt(ethTxn.Hash())
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.Equal(t, ethTxn.Hash(), receipt.TxHash)
	})

	t.Run("eth_getTransactionCount", func(t *testing.T) {
		nonce, err := newEthClient.GetNonce(preminedAcct.Address(), bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.GreaterOrEqual(t, nonce, uint64(0)) // since we used this account in previous tests

		txn := cluster.Transfer(t, preminedAcct, types.StringToAddress("0xDEADBEEF"), one)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		newNonce, err := newEthClient.GetNonce(preminedAcct.Address(), bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.Equal(t, nonce+1, newNonce)
	})

	t.Run("eth_getBalance", func(t *testing.T) {
		balance, err := newEthClient.GetBalance(preminedAcct.Address(), bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.True(t, balance.Cmp(big.NewInt(0)) >= 0)

		receiver := types.StringToAddress("0xDEADFFFF")

		tokens := ethgo.Ether(1)

		txn := cluster.Transfer(t, preminedAcct, receiver, tokens)
		require.NoError(t, txn.Wait())
		require.True(t, txn.Succeed())

		newBalance, err := newEthClient.GetBalance(receiver, bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.Equal(t, tokens, newBalance)
	})

	t.Run("eth_estimateGas", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, preminedAcct, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := types.Address(deployTxn.Receipt().ContractAddress)

		input := contractsapi.TestSimple.Abi.GetMethod("getValue").ID()

		estimatedGas, err := newEthClient.EstimateGas(&bladeRPC.CallMsg{
			From: preminedAcct.Address(),
			To:   &target,
			Data: input,
		})
		require.NoError(t, err)
		require.GreaterOrEqual(t, estimatedGas, uint64(0))
	})

	t.Run("eth_gasPrice", func(t *testing.T) {
		gasPrice, err := newEthClient.GasPrice()
		require.NoError(t, err)
		require.Greater(t, gasPrice, uint64(0)) // london fork is enabled, so gas price should be greater than 0
	})

	t.Run("eth_call", func(t *testing.T) {
		deployTxn := cluster.Deploy(t, preminedAcct, contractsapi.TestSimple.Bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := types.Address(deployTxn.Receipt().ContractAddress)

		input := contractsapi.TestSimple.Abi.GetMethod("getValue").ID()

		acctZeroBalance, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		resp, err := newEthClient.Call(&bladeRPC.CallMsg{
			From: types.Address(acctZeroBalance.Address()),
			To:   &target,
			Data: input,
		}, bladeRPC.LatestBlockNumber, nil)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp)
	})

	t.Run("eth_chainID", func(t *testing.T) {
		chainID, err := newEthClient.ChainID()
		require.NoError(t, err)
		require.Equal(t, big.NewInt(100), chainID) // default chainID
	})

	t.Run("eth_maxPriorityFeePerGas", func(t *testing.T) {
		maxPriorityFeePerGas, err := newEthClient.MaxPriorityFeePerGas()
		require.NoError(t, err)
		// london fork is enabled, so maxPriorityFeePerGas should be greater than 0
		require.True(t, maxPriorityFeePerGas.Cmp(big.NewInt(0)) > 0)
	})

	t.Run("eth_sendRawTransaction", func(t *testing.T) {
		receiver := types.StringToAddress("0xDEADFFFF")
		tokenAmount := ethgo.Ether(1)

		chainID, err := newEthClient.ChainID()
		require.NoError(t, err)

		gasPrice, err := newEthClient.GasPrice()
		require.NoError(t, err)

		newAccountKey, newAccountAddr := tests.GenerateKeyAndAddr(t)

		transferTxn := cluster.Transfer(t, preminedAcct, newAccountAddr, tokenAmount)
		require.NoError(t, transferTxn.Wait())
		require.True(t, transferTxn.Succeed())

		newAccountBalance, err := newEthClient.GetBalance(newAccountAddr, bladeRPC.LatestBlockNumberOrHash)
		require.NoError(t, err)
		require.Equal(t, tokenAmount, newAccountBalance)

		txn := types.NewTx(
			types.NewLegacyTx(
				types.WithNonce(0),
				types.WithFrom(newAccountAddr),
				types.WithTo(&receiver),
				types.WithValue(ethgo.Gwei(1)),
				types.WithGas(21000),
				types.WithGasPrice(new(big.Int).SetUint64(gasPrice)),
			))

		signedTxn, err := crypto.NewLondonSigner(chainID.Uint64()).SignTx(txn, newAccountKey)
		require.NoError(t, err)

		data := signedTxn.MarshalRLPTo(nil)

		hash, err := newEthClient.SendRawTransaction(data)
		require.NoError(t, err)
		require.NotEqual(t, types.ZeroHash, hash)
	})
}
