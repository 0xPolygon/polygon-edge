package e2e

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	helperCommon "github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state/runtime/addresslist"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	chainConfigFileName = "genesis.json"
)

func TestE2E_Bridge_Transfers(t *testing.T) {
	const (
		transfersCount        = 5
		amount                = 100
		numBlockConfirmations = 2
		// make epoch size long enough, so that all exit events are processed within the same epoch
		epochSize            = 30
		sprintSize           = uint64(5)
		numberOfAttempts     = 7
		stateSyncedLogsCount = 1
	)

	receiversAddrs := make([]types.Address, transfersCount)
	receivers := make([]string, transfersCount)
	amounts := make([]string, transfersCount)
	receiverKeys := make([]string, transfersCount)

	for i := 0; i < transfersCount; i++ {
		key, err := wallet.GenerateKey()
		require.NoError(t, err)

		rawKey, err := key.MarshallPrivateKey()
		require.NoError(t, err)

		receiverKeys[i] = hex.EncodeToString(rawKey)
		receiversAddrs[i] = types.Address(key.Address())
		receivers[i] = types.Address(key.Address()).String()
		amounts[i] = fmt.Sprintf("%d", amount)

		t.Logf("Receiver#%d=%s\n", i+1, receivers[i])
	}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithTestRewardToken(),
		framework.WithNumBlockConfirmations(numBlockConfirmations),
		framework.WithEpochSize(epochSize))
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]
	senderAccount, err := sidechain.GetAccountFromDir(validatorSrv.DataDir())
	require.NoError(t, err)

	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	// bridge some tokens for first validator to child chain
	tokensToDeposit := ethgo.Ether(10)

	require.NoError(t, cluster.Bridge.Deposit(
		common.ERC20,
		polybftCfg.Bridge.RootNativeERC20Addr,
		polybftCfg.Bridge.RootERC20PredicateAddr,
		rootHelper.TestAccountPrivKey,
		senderAccount.Address().String(),
		tokensToDeposit.String(),
		"",
		cluster.Bridge.JSONRPCAddr(),
		rootHelper.TestAccountPrivKey,
		false),
	)

	// wait for a couple of sprints
	finalBlockNum := 5 * sprintSize
	require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

	// the transaction is processed and there should be a success event
	var stateSyncedResult contractsapi.StateSyncResultEvent

	logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum, childEthEndpoint)
	require.NoError(t, err)

	// assert that all deposits are executed successfully
	checkStateSyncResultLogs(t, logs, stateSyncedLogsCount)

	// check validator balance got increased by deposited amount
	balance, err := childEthEndpoint.GetBalance(ethgo.Address(senderAccount.Address()), ethgo.Latest)
	require.NoError(t, err)
	// because we premined validators
	expectedBalance := new(big.Int).Add(tokensToDeposit, command.DefaultPremineBalance)
	require.Equal(t, expectedBalance, balance)

	t.Run("bridge ERC 20 tokens", func(t *testing.T) {
		// DEPOSIT ERC20 TOKENS
		// send a few transactions to the bridge
		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			polybftCfg.Bridge.RootNativeERC20Addr,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join(receivers[:], ","),
			strings.Join(amounts[:], ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false,
		))

		finalBlockNum := 10 * sprintSize
		// wait for a couple of sprints
		require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

		// the transactions are processed and there should be a success events
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum, childEthEndpoint)
		require.NoError(t, err)

		// assert that all deposits are executed successfully
		checkStateSyncResultLogs(t, logs, transfersCount+1) // because of the first deposit for the first validator

		// check receivers balances got increased by deposited amount
		for _, receiver := range receivers {
			balance, err := childEthEndpoint.GetBalance(ethgo.Address(types.StringToAddress(receiver)), ethgo.Latest)
			require.NoError(t, err)
			require.Equal(t, big.NewInt(amount), balance)
		}

		t.Log("Deposits were successfully processed")

		// WITHDRAW ERC20 TOKENS
		rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
		require.NoError(t, err)

		senderAccount, err := sidechain.GetAccountFromDir(validatorSrv.DataDir())
		require.NoError(t, err)

		t.Logf("Withdraw sender: %s\n", senderAccount.Ecdsa.Address())

		rawKey, err := senderAccount.Ecdsa.MarshallPrivateKey()
		require.NoError(t, err)

		// send withdraw transaction
		err = cluster.Bridge.Withdraw(
			common.ERC20,
			hex.EncodeToString(rawKey),
			strings.Join(receivers[:], ","),
			strings.Join(amounts[:], ","),
			"",
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC20PredicateContract,
			contracts.NativeERC20TokenContract,
			false)
		require.NoError(t, err)

		currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
		require.NoError(t, err)

		t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

		currentEpoch := currentExtra.Checkpoint.EpochNumber

		exitHelper := polybftCfg.Bridge.ExitHelperAddr
		childJSONRPC := validatorSrv.JSONRPCAddr()

		successfullExitTransactions := make([]bool, transfersCount)

		for i := 0; i < numberOfAttempts; i++ {
			t.Log("Number of attempts: ", i+1)

			require.NoError(t, waitForRootchainEpoch(currentEpoch+uint64(i), 3*time.Minute,
				rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

			for exitEventID := uint64(1); exitEventID <= transfersCount; exitEventID++ {
				if successfullExitTransactions[exitEventID-1] {
					continue
				}

				// send exit transaction to exit helper
				if err = cluster.Bridge.SendExitTransaction(exitHelper, exitEventID, childJSONRPC); err == nil {
					successfullExitTransactions[exitEventID-1] = true

					continue
				}

				if i == numberOfAttempts-1 {
					require.NoError(t, err)
				}
			}

			allExitsSuccessfull := true
			for _, isSuccessfull := range successfullExitTransactions {
				if !isSuccessfull {
					allExitsSuccessfull = false

					break
				}
			}

			if allExitsSuccessfull {
				break
			}
		}

		// assert that receiver's balances on RootERC20 smart contract are expected
		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver),
				polybftCfg.Bridge.RootNativeERC20Addr, rootchainTxRelayer)
			require.Equal(t, big.NewInt(amount), balance)
		}
	})

	t.Run("multiple deposit batches per epoch", func(t *testing.T) {
		const (
			depositsSubset = 1
		)

		txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
		require.NoError(t, err)

		lastCommittedIDMethod := contractsapi.StateReceiver.Abi.GetMethod("lastCommittedId")
		lastCommittedIDInput, err := lastCommittedIDMethod.Encode([]interface{}{})
		require.NoError(t, err)

		// check that we submitted the minimal commitment to smart contract
		commitmentIDRaw, err := txRelayer.Call(ethgo.ZeroAddress, ethgo.Address(contracts.StateReceiverContract), lastCommittedIDInput)
		require.NoError(t, err)

		initialCommittedID, err := helperCommon.ParseUint64orHex(&commitmentIDRaw)
		require.NoError(t, err)

		initialBlockNum, err := childEthEndpoint.BlockNumber()
		require.NoError(t, err)

		// wait for next sprint block as the starting point,
		// in order to be able to make assertions against blocks offsetted by sprints
		initialBlockNum = initialBlockNum + sprintSize - (initialBlockNum % sprintSize)
		require.NoError(t, cluster.WaitForBlock(initialBlockNum, 1*time.Minute))

		// send two transactions to the bridge so that we have a minimal commitment
		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			polybftCfg.Bridge.RootNativeERC20Addr,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join(receivers[:depositsSubset], ","),
			strings.Join(amounts[:depositsSubset], ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false),
		)

		// wait for a few more sprints
		midBlockNumber := initialBlockNum + 2*sprintSize
		require.NoError(t, cluster.WaitForBlock(midBlockNumber, 2*time.Minute))

		// check that we submitted the minimal commitment to smart contract
		commitmentIDRaw, err = txRelayer.Call(ethgo.ZeroAddress,
			ethgo.Address(contracts.StateReceiverContract), lastCommittedIDInput)
		require.NoError(t, err)

		lastCommittedID, err := helperCommon.ParseUint64orHex(&commitmentIDRaw)
		require.NoError(t, err)
		require.Equal(t, initialCommittedID+depositsSubset, lastCommittedID)

		// send some more transactions to the bridge to build another commitment in epoch
		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			polybftCfg.Bridge.RootNativeERC20Addr,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join(receivers[depositsSubset:], ","),
			strings.Join(amounts[depositsSubset:], ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false),
		)

		finalBlockNum := midBlockNumber + 5*sprintSize
		// wait for a few more sprints
		require.NoError(t, cluster.WaitForBlock(midBlockNumber+5*sprintSize, 3*time.Minute))

		// check that we submitted the minimal commitment to smart contract
		commitmentIDRaw, err = txRelayer.Call(ethgo.ZeroAddress, ethgo.Address(contracts.StateReceiverContract), lastCommittedIDInput)
		require.NoError(t, err)

		// check that the second (larger commitment) was also submitted in epoch
		lastCommittedID, err = helperCommon.ParseUint64orHex(&commitmentIDRaw)
		require.NoError(t, err)
		require.Equal(t, initialCommittedID+uint64(transfersCount), lastCommittedID)

		// the transactions are mined and state syncs should be executed by the relayer
		// and there should be a success events
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), initialBlockNum, finalBlockNum, childEthEndpoint)
		require.NoError(t, err)

		// assert that all state syncs are executed successfully
		checkStateSyncResultLogs(t, logs, transfersCount)
	})

	t.Run("non native ERC20 deposit and withdraw", func(t *testing.T) {
		rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
		require.NoError(t, err)

		childchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
		require.NoError(t, err)

		// Deploy ERC20 contract to the rootchain
		rootchainDeployer, err := rootHelper.DecodePrivateKey("")
		require.NoError(t, err)

		txn := &ethgo.Transaction{To: nil, Input: contractsapi.RootERC20.Bytecode}
		receipt, err := rootchainTxRelayer.SendTransaction(txn, rootchainDeployer)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

		rootTokenAddr := receipt.ContractAddress

		t.Log("Rootchain token address:", rootTokenAddr)

		// wait for next sprint block as the starting point,
		// in order to be able to make assertions against blocks offsetted by sprints
		initialBlockNum, err := childEthEndpoint.BlockNumber()
		require.NoError(t, err)

		initialBlockNum = initialBlockNum + sprintSize - (initialBlockNum % sprintSize)
		require.NoError(t, cluster.WaitForBlock(initialBlockNum, 1*time.Minute))

		t.Log("Initial block number:", initialBlockNum)

		// send a few transactions to the bridge
		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			types.Address(rootTokenAddr),
			polybftCfg.Bridge.RootERC20PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join(receivers[:], ","),
			strings.Join(amounts[:], ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false,
		))

		// wait for a few more sprints
		finalBlockNumber := initialBlockNum + 10*sprintSize
		require.NoError(t, cluster.WaitForBlock(finalBlockNumber, 2*time.Minute))

		t.Log("Final block number:", finalBlockNumber)

		// the transactions are processed and there should be a success events
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), initialBlockNum, finalBlockNumber, childEthEndpoint)
		require.NoError(t, err)

		// assert that all deposits are executed successfully
		// map token action is executed along with the first deposit transaction
		checkStateSyncResultLogs(t, logs, transfersCount+1)

		// retrieve child token address
		childTokenAddr := getChildToken(
			t,
			contractsapi.ChildERC20Predicate.Abi,
			contracts.ChildERC20PredicateContract,
			types.Address(rootTokenAddr),
			childchainTxRelayer,
		)

		t.Log("Childchain token address:", childTokenAddr)

		// check receivers balances got increased by deposited amount
		for _, receiver := range receiversAddrs {
			balance := erc20BalanceOf(t, receiver, childTokenAddr, childchainTxRelayer)
			require.Equal(t, big.NewInt(int64(amount)), balance)
		}

		t.Log("Deposits were successfully processed")

		// get initial exit id
		initialExitEventID := getLastExitEventID(t, childchainTxRelayer)

		// WITHDRAW ERC20 TOKENS
		for i, receiverKey := range receiverKeys {
			t.Logf("Withdraw to: %s\n", receivers[i])

			require.NoError(t, cluster.Bridge.Withdraw(
				common.ERC20,
				receiverKey,
				receivers[i],
				amounts[i],
				"",
				validatorSrv.JSONRPCAddr(),
				contracts.ChildERC20PredicateContract,
				childTokenAddr,
				false,
			))
		}

		currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
		require.NoError(t, err)

		t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

		require.NoError(t, waitForRootchainEpoch(
			currentExtra.Checkpoint.EpochNumber+1,
			3*time.Minute,
			rootchainTxRelayer,
			polybftCfg.Bridge.CheckpointManagerAddr,
		))

		exitHelper := polybftCfg.Bridge.ExitHelperAddr
		childJSONRPC := validatorSrv.JSONRPCAddr()

		initialExitEventID++
		for i := initialExitEventID; i < initialExitEventID+transfersCount; i++ {
			// send exit transaction to exit helper
			err = cluster.Bridge.SendExitTransaction(exitHelper, i, childJSONRPC)
			require.NoError(t, err)
		}

		// assert that receiver's balances on RootERC20 smart contract are expected
		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), types.Address(rootTokenAddr), rootchainTxRelayer)
			require.Equal(t, big.NewInt(amount), balance)
		}
	})
}

func TestE2E_Bridge_ERC721Transfer(t *testing.T) {
	const (
		transfersCount       = 4
		epochSize            = 5
		numberOfAttempts     = 4
		stateSyncedLogsCount = 2
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	receiverKeys := make([]string, transfersCount)
	receivers := make([]string, transfersCount)
	receiversAddrs := make([]types.Address, transfersCount)
	tokenIDs := make([]string, transfersCount)

	for i := 0; i < transfersCount; i++ {
		key, err := wallet.GenerateKey()
		require.NoError(t, err)

		rawKey, err := key.MarshallPrivateKey()
		require.NoError(t, err)

		receiverKeys[i] = hex.EncodeToString(rawKey)
		receivers[i] = types.Address(key.Address()).String()
		receiversAddrs[i] = types.Address(key.Address())
		tokenIDs[i] = fmt.Sprintf("%d", i)

		t.Logf("Receiver#%d=%s\n", i+1, receivers[i])
	}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize),
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
		framework.WithPremine(types.Address(minter.Address())),
		framework.WithPremine(receiversAddrs...))
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	rootchainDeployer, err := rootHelper.DecodePrivateKey("")
	require.NoError(t, err)

	// deploy root ERC 721 token
	deployTxn := &ethgo.Transaction{To: nil, Input: contractsapi.RootERC721.Bytecode}
	receipt, err := rootchainTxRelayer.SendTransaction(deployTxn, rootchainDeployer)
	require.NoError(t, err)

	rootERC721Addr := receipt.ContractAddress

	// DEPOSIT ERC721 TOKENS
	// send a few transactions to the bridge
	require.NoError(
		t,
		cluster.Bridge.Deposit(
			common.ERC721,
			types.Address(rootERC721Addr),
			polybftCfg.Bridge.RootERC721PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join(receivers[:], ","),
			"",
			strings.Join(tokenIDs[:], ","),
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(50, 4*time.Minute))

	validatorSrv := cluster.Servers[0]
	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	// the transactions are processed and there should be a success events
	var stateSyncedResult contractsapi.StateSyncResultEvent

	for i := 0; i < numberOfAttempts; i++ {
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, uint64(50+i*epochSize), childEthEndpoint)
		require.NoError(t, err)

		if len(logs) == stateSyncedLogsCount || i == numberOfAttempts-1 {
			// assert that all deposits are executed successfully.
			// All deposits are sent using a single transaction, so arbitrary message bridge emits two state sync events:
			// MAP_TOKEN_SIG and DEPOSIT_BATCH_SIG state sync events
			checkStateSyncResultLogs(t, logs, stateSyncedLogsCount)

			break
		}

		require.NoError(t, cluster.WaitForBlock(uint64(50+(i+1)*epochSize), 1*time.Minute))
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
	require.NoError(t, err)

	// retrieve child token address (from both chains, and assert they are the same)
	l1ChildTokenAddr := getChildToken(t, contractsapi.RootERC721Predicate.Abi, polybftCfg.Bridge.RootERC721PredicateAddr,
		types.Address(rootERC721Addr), rootchainTxRelayer)
	l2ChildTokenAddr := getChildToken(t, contractsapi.ChildERC721Predicate.Abi, contracts.ChildERC721PredicateContract,
		types.Address(rootERC721Addr), txRelayer)

	t.Log("L1 child token", l1ChildTokenAddr)
	t.Log("L2 child token", l2ChildTokenAddr)
	require.Equal(t, l1ChildTokenAddr, l2ChildTokenAddr)

	for i, receiver := range receiversAddrs {
		owner := erc721OwnerOf(t, big.NewInt(int64(i)), l2ChildTokenAddr, txRelayer)
		require.Equal(t, receiver, owner)
	}

	t.Log("Deposits were successfully processed")

	// WITHDRAW ERC721 TOKENS
	for i, receiverKey := range receiverKeys {
		// send withdraw transactions
		err = cluster.Bridge.Withdraw(
			common.ERC721,
			receiverKey,
			receivers[i],
			"",
			tokenIDs[i],
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC721PredicateContract,
			l2ChildTokenAddr,
			false)
		require.NoError(t, err)
	}

	currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
	require.NoError(t, err)

	t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

	currentEpoch := currentExtra.Checkpoint.EpochNumber
	require.NoError(t, waitForRootchainEpoch(currentEpoch, 3*time.Minute, rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

	exitHelper := polybftCfg.Bridge.ExitHelperAddr
	childJSONRPC := validatorSrv.JSONRPCAddr()

	for exitEventID := uint64(1); exitEventID <= transfersCount; exitEventID++ {
		// send exit transaction to exit helper
		err = cluster.Bridge.SendExitTransaction(exitHelper, exitEventID, childJSONRPC)
		require.NoError(t, err)
	}

	// assert that owners of given token ids are the accounts on the root chain ERC 721 token
	for i, receiver := range receiversAddrs {
		owner := erc721OwnerOf(t, big.NewInt(int64(i)), types.Address(rootERC721Addr), rootchainTxRelayer)
		require.Equal(t, receiver, owner)
	}
}

func TestE2E_Bridge_ERC1155Transfer(t *testing.T) {
	const (
		transfersCount       = 5
		amount               = 100
		epochSize            = 5
		numberOfAttempts     = 4
		stateSyncedLogsCount = 2
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	receiverKeys := make([]string, transfersCount)
	receivers := make([]string, transfersCount)
	receiversAddrs := make([]types.Address, transfersCount)
	amounts := make([]string, transfersCount)
	tokenIDs := make([]string, transfersCount)

	for i := 0; i < transfersCount; i++ {
		key, err := wallet.GenerateKey()
		require.NoError(t, err)

		rawKey, err := key.MarshallPrivateKey()
		require.NoError(t, err)

		receiverKeys[i] = hex.EncodeToString(rawKey)
		receivers[i] = types.Address(key.Address()).String()
		receiversAddrs[i] = types.Address(key.Address())
		amounts[i] = fmt.Sprintf("%d", amount)
		tokenIDs[i] = fmt.Sprintf("%d", i+1)

		t.Logf("Receiver#%d=%s\n", i+1, receivers[i])
	}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNumBlockConfirmations(0),
		framework.WithEpochSize(epochSize),
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
		framework.WithPremine(types.Address(minter.Address())),
		framework.WithPremine(receiversAddrs...))
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	rootchainDeployer, err := rootHelper.DecodePrivateKey("")
	require.NoError(t, err)

	// deploy root ERC 1155 token
	deployTxn := &ethgo.Transaction{To: nil, Input: contractsapi.RootERC1155.Bytecode}
	receipt, err := rootchainTxRelayer.SendTransaction(deployTxn, rootchainDeployer)
	require.NoError(t, err)

	rootERC1155Addr := receipt.ContractAddress

	// DEPOSIT ERC1155 TOKENS
	// send a few transactions to the bridge
	require.NoError(
		t,
		cluster.Bridge.Deposit(
			common.ERC1155,
			types.Address(rootERC1155Addr),
			polybftCfg.Bridge.RootERC1155PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join(receivers[:], ","),
			strings.Join(amounts[:], ","),
			strings.Join(tokenIDs[:], ","),
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false),
	)

	// wait for a few more sprints
	require.NoError(t, cluster.WaitForBlock(50, 4*time.Minute))

	validatorSrv := cluster.Servers[0]
	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	// the transactions are processed and there should be a success events
	var stateSyncedResult contractsapi.StateSyncResultEvent

	for i := 0; i < numberOfAttempts; i++ {
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, uint64(50+i*epochSize), childEthEndpoint)
		require.NoError(t, err)

		if len(logs) == stateSyncedLogsCount || i == numberOfAttempts-1 {
			// assert that all deposits are executed successfully.
			// All deposits are sent using a single transaction, so arbitrary message bridge emits two state sync events:
			// MAP_TOKEN_SIG and DEPOSIT_BATCH_SIG state sync events
			checkStateSyncResultLogs(t, logs, stateSyncedLogsCount)

			break
		}

		require.NoError(t, cluster.WaitForBlock(uint64(50+(i+1)*epochSize), 1*time.Minute))
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
	require.NoError(t, err)

	// retrieve child token address
	l1ChildTokenAddr := getChildToken(t, contractsapi.RootERC1155Predicate.Abi, polybftCfg.Bridge.RootERC1155PredicateAddr,
		types.Address(rootERC1155Addr), rootchainTxRelayer)
	l2ChildTokenAddr := getChildToken(t, contractsapi.ChildERC1155Predicate.Abi, contracts.ChildERC1155PredicateContract,
		types.Address(rootERC1155Addr), txRelayer)

	t.Log("L1 child token", l1ChildTokenAddr)
	t.Log("L2 child token", l2ChildTokenAddr)
	require.Equal(t, l1ChildTokenAddr, l2ChildTokenAddr)

	// check receivers balances got increased by deposited amount
	for i, receiver := range receivers {
		balanceOfFn := &contractsapi.BalanceOfChildERC1155Fn{
			Account: types.StringToAddress(receiver),
			ID:      big.NewInt(int64(i + 1)),
		}

		balanceInput, err := balanceOfFn.EncodeAbi()
		require.NoError(t, err)

		balanceRaw, err := txRelayer.Call(ethgo.ZeroAddress, ethgo.Address(l2ChildTokenAddr), balanceInput)
		require.NoError(t, err)

		balance, err := helperCommon.ParseUint256orHex(&balanceRaw)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(int64(amount)), balance)
	}

	t.Log("Deposits were successfully processed")

	// WITHDRAW ERC1155 TOKENS
	senderAccount, err := sidechain.GetAccountFromDir(cluster.Servers[0].DataDir())
	require.NoError(t, err)

	t.Logf("Withdraw sender: %s\n", senderAccount.Ecdsa.Address())

	for i, receiverKey := range receiverKeys {
		// send withdraw transactions
		err = cluster.Bridge.Withdraw(
			common.ERC1155,
			receiverKey,
			receivers[i],
			amounts[i],
			tokenIDs[i],
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC1155PredicateContract,
			l2ChildTokenAddr,
			false)
		require.NoError(t, err)
	}

	currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
	require.NoError(t, err)

	currentEpoch := currentExtra.Checkpoint.EpochNumber
	t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

	require.NoError(t, waitForRootchainEpoch(currentEpoch, 3*time.Minute,
		rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

	exitHelper := polybftCfg.Bridge.ExitHelperAddr
	childJSONRPC := validatorSrv.JSONRPCAddr()

	for exitEventID := uint64(1); exitEventID <= transfersCount; exitEventID++ {
		// send exit transaction to exit helper
		err = cluster.Bridge.SendExitTransaction(exitHelper, exitEventID, childJSONRPC)
		require.NoError(t, err)
	}

	// assert that receiver's balances on RootERC1155 smart contract are expected
	for i, receiver := range receivers {
		balanceOfFn := &contractsapi.BalanceOfRootERC1155Fn{
			Account: types.StringToAddress(receiver),
			ID:      big.NewInt(int64(i + 1)),
		}

		balanceInput, err := balanceOfFn.EncodeAbi()
		require.NoError(t, err)

		balanceRaw, err := rootchainTxRelayer.Call(ethgo.ZeroAddress, rootERC1155Addr, balanceInput)
		require.NoError(t, err)

		balance, err := helperCommon.ParseUint256orHex(&balanceRaw)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(amount), balance)
	}
}

func TestE2E_Bridge_ChildChainMintableTokensTransfer(t *testing.T) {
	const (
		transfersCount = uint64(4)
		amount         = 100
		// make epoch size long enough, so that all exit events are processed within the same epoch
		epochSize        = 30
		sprintSize       = uint64(5)
		numberOfAttempts = 4
	)

	// init private keys and amounts
	depositorKeys := make([]string, transfersCount)
	depositors := make([]types.Address, transfersCount)
	amounts := make([]string, transfersCount)
	funds := make([]*big.Int, transfersCount)
	singleToken := ethgo.Ether(1)

	admin, err := wallet.GenerateKey()
	require.NoError(t, err)

	adminAddr := types.Address(admin.Address())

	for i := uint64(0); i < transfersCount; i++ {
		key, err := wallet.GenerateKey()
		require.NoError(t, err)

		rawKey, err := key.MarshallPrivateKey()
		require.NoError(t, err)

		depositorKeys[i] = hex.EncodeToString(rawKey)
		depositors[i] = types.Address(key.Address())
		funds[i] = singleToken
		amounts[i] = fmt.Sprintf("%d", amount)

		t.Logf("Depositor#%d=%s\n", i+1, depositors[i])
	}

	// setup cluster
	cluster := framework.NewTestCluster(t, 5,
		framework.WithNumBlockConfirmations(0),
		framework.WithEpochSize(epochSize),
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, adminAddr)),
		framework.WithBridgeBlockListAdmin(adminAddr),
		framework.WithPremine(append(depositors, adminAddr)...)) //nolint:makezero
	defer cluster.Stop()

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]
	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	// fund accounts on rootchain
	require.NoError(t, validatorSrv.RootchainFundFor(depositors, funds, polybftCfg.Bridge.StakeTokenAddr))

	cluster.WaitForReady(t)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	childchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
	require.NoError(t, err)

	var mintableTokenMapped contractsapi.MintableTokenMappedEvent

	t.Run("bridge native tokens", func(t *testing.T) {
		// rootToken represents deposit token (basically native mintable token from the Supernets)
		rootToken := contracts.NativeERC20TokenContract

		// block list first depositor
		setAccessListRole(t, cluster, contracts.BlockListBridgeAddr, depositors[0], addresslist.EnabledRole, admin)

		// try sending a single native token deposit transaction
		// it should fail, because first depositor is added to bridge transactions block list
		err = cluster.Bridge.Deposit(
			common.ERC20,
			rootToken,
			contracts.RootMintableERC20PredicateContract,
			depositorKeys[0],
			depositors[0].String(),
			amounts[0],
			"",
			validatorSrv.JSONRPCAddr(),
			"",
			true)
		require.Error(t, err)

		// remove first depositor from bridge transactions block list
		setAccessListRole(t, cluster, contracts.BlockListBridgeAddr, depositors[0], addresslist.NoRole, admin)

		// allow list each depositor and make sure deposit is successfully executed
		for i, key := range depositorKeys {
			// make sure deposit is successfully executed
			err = cluster.Bridge.Deposit(
				common.ERC20,
				rootToken,
				contracts.RootMintableERC20PredicateContract,
				key,
				depositors[i].String(),
				amounts[i],
				"",
				validatorSrv.JSONRPCAddr(),
				"",
				true)
			require.NoError(t, err)
		}

		latestBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		extra, err := polybft.GetIbftExtra(latestBlock.ExtraData)
		require.NoError(t, err)

		// wait for checkpoint to get submitted before invoking exit transactions
		require.NoError(t,
			waitForRootchainEpoch(extra.Checkpoint.EpochNumber, 2*time.Minute, rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

		// first exit event is mapping child token on a rootchain
		// remaining ones are deposits
		for exitEventID := uint64(1); exitEventID <= transfersCount+1; exitEventID++ {
			require.NoError(t,
				cluster.Bridge.SendExitTransaction(polybftCfg.Bridge.ExitHelperAddr, exitEventID, validatorSrv.JSONRPCAddr()))
		}

		rootchainLatestBlock, err := rootchainTxRelayer.Client().Eth().BlockNumber()
		require.NoError(t, err)

		// retrieve child mintable token address from both chains and make sure they are the same
		l1ChildToken := getChildToken(t, contractsapi.ChildMintableERC20Predicate.Abi, polybftCfg.Bridge.ChildMintableERC20PredicateAddr,
			rootToken, rootchainTxRelayer)
		l2ChildToken := getChildToken(t, contractsapi.RootMintableERC20Predicate.Abi, contracts.RootMintableERC20PredicateContract,
			rootToken, childchainTxRelayer)

		t.Log("L1 child token", l1ChildToken)
		t.Log("L2 child token", l2ChildToken)
		require.Equal(t, l1ChildToken, l2ChildToken)

		logs, err := getFilteredLogs(mintableTokenMapped.Sig(), 0, rootchainLatestBlock, rootchainTxRelayer.Client().Eth())
		require.NoError(t, err)
		require.Len(t, logs, 1)

		// check that balances on rootchain have increased by deposited amounts
		for _, depositor := range depositors {
			balance := erc20BalanceOf(t, depositor, l1ChildToken, rootchainTxRelayer)
			require.Equal(t, big.NewInt(amount), balance)
		}

		balancesBefore := make([]*big.Int, transfersCount)
		for i := uint64(0); i < transfersCount; i++ {
			balancesBefore[i], err = childEthEndpoint.GetBalance(ethgo.Address(depositors[i]), ethgo.Latest)
			require.NoError(t, err)
		}

		// withdraw child token on the rootchain
		for i, depositorKey := range depositorKeys {
			err = cluster.Bridge.Withdraw(
				common.ERC20,
				depositorKey,
				depositors[i].String(),
				amounts[i],
				"",
				cluster.Bridge.JSONRPCAddr(),
				polybftCfg.Bridge.ChildMintableERC20PredicateAddr,
				l1ChildToken,
				true)
			require.NoError(t, err)
		}

		allSuccessful := false

		for it := 0; it < numberOfAttempts && !allSuccessful; it++ {
			blockNum, err := childEthEndpoint.BlockNumber()
			require.NoError(t, err)

			// wait a couple of sprints to finalize state sync events
			require.NoError(t, cluster.WaitForBlock(blockNum+3*sprintSize, 2*time.Minute))

			allSuccessful = true

			// check that balances on the child chain are correct
			for i, receiver := range depositors {
				balance := erc20BalanceOf(t, receiver, contracts.NativeERC20TokenContract, childchainTxRelayer)
				t.Log("Attempt", it+1, "Balance before", balancesBefore[i], "Balance after", balance)
				if balance.Cmp(balancesBefore[i].Add(balancesBefore[i], big.NewInt(amount))) != 0 {
					allSuccessful = false

					break
				}
			}
		}

		require.True(t, allSuccessful)
	})

	t.Run("bridge ERC 721 tokens", func(t *testing.T) {
		rootchainInitialBlock, err := rootchainTxRelayer.Client().Eth().BlockNumber()
		require.NoError(t, err)

		// get initial exit id
		initialExitEventID := getLastExitEventID(t, childchainTxRelayer)

		erc721DeployTxn := cluster.Deploy(t, admin, contractsapi.RootERC721.Bytecode)
		require.NoError(t, erc721DeployTxn.Wait())
		require.True(t, erc721DeployTxn.Succeed())
		rootERC721Token := erc721DeployTxn.Receipt().ContractAddress

		for _, depositor := range depositors {
			// mint all the depositors in advance
			mintFn := &contractsapi.MintRootERC721Fn{To: depositor}
			mintInput, err := mintFn.EncodeAbi()
			require.NoError(t, err)

			mintTxn := cluster.MethodTxn(t, admin, types.Address(rootERC721Token), mintInput)
			require.NoError(t, mintTxn.Wait())
			require.True(t, mintTxn.Succeed())

			// add all depositors to bride block list
			setAccessListRole(t, cluster, contracts.BlockListBridgeAddr, depositor, addresslist.EnabledRole, admin)
		}

		// deposit should fail because depositors are in bridge block list
		err = cluster.Bridge.Deposit(
			common.ERC721,
			types.Address(rootERC721Token),
			contracts.RootMintableERC721PredicateContract,
			depositorKeys[0],
			depositors[0].String(),
			"",
			fmt.Sprintf("%d", 0),
			validatorSrv.JSONRPCAddr(),
			"",
			true)
		require.Error(t, err)

		for i, depositorKey := range depositorKeys {
			// remove all depositors from the bridge block list
			setAccessListRole(t, cluster, contracts.BlockListBridgeAddr, depositors[i], addresslist.NoRole, admin)

			// deposit (without minting, as it was already done beforehand)
			err = cluster.Bridge.Deposit(
				common.ERC721,
				types.Address(rootERC721Token),
				contracts.RootMintableERC721PredicateContract,
				depositorKey,
				depositors[i].String(),
				"",
				fmt.Sprintf("%d", i),
				validatorSrv.JSONRPCAddr(),
				"",
				true)
			require.NoError(t, err)
		}

		childChainBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		childChainBlockExtra, err := polybft.GetIbftExtra(childChainBlock.ExtraData)
		require.NoError(t, err)

		// wait for checkpoint to be submitted
		require.NoError(t,
			waitForRootchainEpoch(childChainBlockExtra.Checkpoint.EpochNumber, 2*time.Minute, rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

		// first exit event is mapping child token on a rootchain
		// remaining ones are the deposits
		initialExitEventID++
		for i := initialExitEventID; i <= initialExitEventID+transfersCount; i++ {
			require.NoError(t,
				cluster.Bridge.SendExitTransaction(polybftCfg.Bridge.ExitHelperAddr, i, validatorSrv.JSONRPCAddr()))
		}

		latestRootchainBlock, err := rootchainTxRelayer.Client().Eth().BlockNumber()
		require.NoError(t, err)

		// retrieve child token addresses on both chains and make sure they are the same
		l1ChildToken := getChildToken(t, contractsapi.ChildMintableERC721Predicate.Abi, polybftCfg.Bridge.ChildMintableERC721PredicateAddr,
			types.Address(rootERC721Token), rootchainTxRelayer)
		l2ChildToken := getChildToken(t, contractsapi.RootMintableERC721Predicate.Abi, contracts.RootMintableERC721PredicateContract,
			types.Address(rootERC721Token), childchainTxRelayer)

		t.Log("L1 child token", l1ChildToken)
		t.Log("L2 child token", l2ChildToken)
		require.Equal(t, l1ChildToken, l2ChildToken)

		logs, err := getFilteredLogs(mintableTokenMapped.Sig(), rootchainInitialBlock, latestRootchainBlock,
			rootchainTxRelayer.Client().Eth())
		require.NoError(t, err)
		require.Len(t, logs, 1)

		// check owner on the rootchain
		for i := uint64(0); i < transfersCount; i++ {
			owner := erc721OwnerOf(t, new(big.Int).SetUint64(i), l1ChildToken, rootchainTxRelayer)
			t.Log("ChildERC721 owner", owner)
			require.Equal(t, depositors[i], owner)
		}

		// withdraw tokens
		for i, depositorKey := range depositorKeys {
			err = cluster.Bridge.Withdraw(
				common.ERC721,
				depositorKey,
				depositors[i].String(),
				"",
				fmt.Sprintf("%d", i),
				cluster.Bridge.JSONRPCAddr(),
				polybftCfg.Bridge.ChildMintableERC721PredicateAddr,
				l1ChildToken,
				true)
			require.NoError(t, err)
		}

		allSuccessful := false

		for it := 0; it < numberOfAttempts && !allSuccessful; it++ {
			childChainBlockNum, err := childEthEndpoint.BlockNumber()
			require.NoError(t, err)

			// wait for commitment execution
			require.NoError(t, cluster.WaitForBlock(childChainBlockNum+3*sprintSize, 2*time.Minute))

			allSuccessful = true
			// check owners on the child chain
			for i, receiver := range depositors {
				owner := erc721OwnerOf(t, big.NewInt(int64(i)), types.Address(rootERC721Token), childchainTxRelayer)
				t.Log("Attempt:", it+1, " Owner:", owner, " Receiver:", receiver)
				if receiver != owner {
					allSuccessful = false

					break
				}
			}
		}

		require.True(t, allSuccessful)
	})
}

func TestE2E_CheckpointSubmission(t *testing.T) {
	// spin up a cluster with epoch size set to 5 blocks
	cluster := framework.NewTestCluster(t, 5, framework.WithEpochSize(5), framework.WithTestRewardToken())
	defer cluster.Stop()

	// initialize tx relayer used to query CheckpointManager smart contract
	rootChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	checkpointManagerAddr := ethgo.Address(polybftCfg.Bridge.CheckpointManagerAddr)

	testCheckpointBlockNumber := func(expectedCheckpointBlock uint64) (bool, error) {
		actualCheckpointBlock, err := getCheckpointBlockNumber(rootChainRelayer, checkpointManagerAddr)
		if err != nil {
			return false, err
		}

		t.Logf("Checkpoint block: %d\n", actualCheckpointBlock)

		return actualCheckpointBlock == expectedCheckpointBlock, nil
	}

	// wait for a single epoch to be checkpointed
	require.NoError(t, cluster.WaitForBlock(7, 30*time.Second))

	// checking last checkpoint block before rootchain server stop
	err = cluster.Bridge.WaitUntil(2*time.Second, 30*time.Second, func() (bool, error) {
		return testCheckpointBlockNumber(5)
	})
	require.NoError(t, err)

	// stop rootchain server
	cluster.Bridge.Stop()

	// wait for a couple of epochs so that there are pending checkpoint (epoch-ending) blocks
	require.NoError(t, cluster.WaitForBlock(21, 2*time.Minute))

	// restart rootchain server
	require.NoError(t, cluster.Bridge.Start())

	// check if pending checkpoint blocks were submitted (namely the last checkpointed block must be block 20)
	err = cluster.Bridge.WaitUntil(2*time.Second, 50*time.Second, func() (bool, error) {
		return testCheckpointBlockNumber(20)
	})
	require.NoError(t, err)
}

func TestE2E_Bridge_ChangeVotingPower(t *testing.T) {
	const (
		votingPowerChanges = 2
		epochSize          = 5
	)

	minter, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize),
		framework.WithEpochReward(1000000),
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, minter.Address())),
		framework.WithPremine(types.Address(minter.Address())),
	)
	defer cluster.Stop()

	// load polybft config
	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSecretFiles, err := genesis.GetValidatorKeyFiles(cluster.Config.TmpDir, cluster.Config.ValidatorPrefix)
	require.NoError(t, err)

	votingPowerChangeValidators := make([]ethgo.Address, votingPowerChanges)

	for i := 0; i < votingPowerChanges; i++ {
		validator, err := sidechain.GetAccountFromDir(path.Join(cluster.Config.TmpDir, validatorSecretFiles[i]))
		require.NoError(t, err)

		votingPowerChangeValidators[i] = validator.Ecdsa.Address()
	}

	// child chain tx relayer
	childRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	// root chain tx relayer
	rootRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	// waiting two epochs, so that some rewards get accumulated
	require.NoError(t, cluster.WaitForBlock(2*epochSize, 1*time.Minute))

	queryValidators := func(handler func(idx int, validatorInfo *polybft.ValidatorInfo)) {
		for i, validatorAddr := range votingPowerChangeValidators {
			// query validator info
			validatorInfo, err := sidechain.GetValidatorInfo(
				validatorAddr,
				polybftCfg.Bridge.CustomSupernetManagerAddr,
				polybftCfg.Bridge.StakeManagerAddr,
				polybftCfg.SupernetID,
				rootRelayer,
				childRelayer)
			require.NoError(t, err)

			handler(i, validatorInfo)
		}
	}

	// validatorsMap holds only changed validators
	validatorsMap := make(map[ethgo.Address]*polybft.ValidatorInfo, votingPowerChanges)

	queryValidators(func(idx int, validator *polybft.ValidatorInfo) {
		t.Logf("[Validator#%d] Voting power (original)=%d, rewards=%d\n",
			idx+1, validator.Stake, validator.WithdrawableRewards)

		validatorsMap[validator.Address] = validator
		validatorSrv := cluster.Servers[idx]

		// fund validators (send accumulated rewards amount)
		require.NoError(t, validatorSrv.RootchainFund(polybftCfg.Bridge.StakeTokenAddr, validator.WithdrawableRewards))

		// stake previously funded amount
		require.NoError(t, validatorSrv.Stake(polybftCfg, validator.WithdrawableRewards))
	})

	queryValidators(func(idx int, validator *polybft.ValidatorInfo) {
		t.Logf("[Validator#%d] Voting power (after stake)=%d\n", idx+1, validator.Stake)

		previousValidatorInfo := validatorsMap[validator.Address]
		stakedAmount := new(big.Int).Add(previousValidatorInfo.WithdrawableRewards, previousValidatorInfo.Stake)

		// assert that total stake has increased by staked amount
		require.Equal(t, stakedAmount, validator.Stake)

		validatorsMap[validator.Address] = validator
	})

	currentBlockNum, err := childRelayer.Client().Eth().BlockNumber()
	require.NoError(t, err)

	// wait for next epoch-ending block as the starting point,
	// in order to be able to easier track checkpoints submission
	endOfEpochBlockNum := currentBlockNum + epochSize - (currentBlockNum % epochSize)
	require.NoError(t, cluster.WaitForBlock(endOfEpochBlockNum, 1*time.Minute))

	currentBlock, err := childRelayer.Client().Eth().GetBlockByNumber(ethgo.Latest, false)
	require.NoError(t, err)

	currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
	require.NoError(t, err)

	targetEpoch := currentExtra.Checkpoint.EpochNumber + 2
	require.NoError(t, waitForRootchainEpoch(targetEpoch, 2*time.Minute,
		rootRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

	// make sure that correct validator set is submitted to the checkpoint manager
	checkpointValidators, err := getCheckpointManagerValidators(rootRelayer, ethgo.Address(polybftCfg.Bridge.CheckpointManagerAddr))
	require.NoError(t, err)

	for _, checkpointValidator := range checkpointValidators {
		if validator, ok := validatorsMap[checkpointValidator.Address]; ok {
			require.Equal(t, validator.Stake, checkpointValidator.Stake)
		} else {
			require.Equal(t, command.DefaultPremineBalance, checkpointValidator.Stake)
		}
	}
}

func TestE2E_Bridge_Transfers_AccessLists(t *testing.T) {
	var (
		transfersCount = 5
		depositAmount  = ethgo.Ether(5)
		withdrawAmount = ethgo.Ether(1)
		// make epoch size long enough, so that all exit events are processed within the same epoch
		epochSize            = 30
		sprintSize           = uint64(5)
		stateSyncedLogsCount = 1
	)

	receivers := make([]string, transfersCount)
	depositAmounts := make([]string, transfersCount)
	withdrawAmounts := make([]string, transfersCount)

	admin, _ := wallet.GenerateKey()
	adminAddr := types.Address(admin.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNumBlockConfirmations(0),
		framework.WithEpochSize(epochSize),
		framework.WithTestRewardToken(),
		framework.WithBridgeAllowListAdmin(adminAddr),
		framework.WithBridgeBlockListAdmin(adminAddr),
		framework.WithSecretsCallback(func(a []types.Address, tcc *framework.TestClusterConfig) {
			for i := 0; i < len(a); i++ {
				receivers[i] = a[i].String()
				depositAmounts[i] = fmt.Sprintf("%d", depositAmount)
				withdrawAmounts[i] = fmt.Sprintf("%d", withdrawAmount)

				t.Logf("Receiver#%d=%s\n", i+1, receivers[i])
			}
		}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]
	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	var stateSyncedResult contractsapi.StateSyncResultEvent

	// fund admin on rootchain
	require.NoError(t, cluster.Servers[0].RootchainFundFor([]types.Address{adminAddr}, []*big.Int{ethgo.Ether(10)},
		polybftCfg.Bridge.StakeTokenAddr))

	adminBalanceOnChild := ethgo.Ether(5)

	// bridge some tokens for admin to child chain
	require.NoError(
		t, cluster.Bridge.Deposit(
			common.ERC20,
			polybftCfg.Bridge.RootNativeERC20Addr,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			rootHelper.TestAccountPrivKey,
			adminAddr.String(),
			adminBalanceOnChild.String(),
			"",
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false),
	)

	// wait for a couple of sprints
	finalBlockNum := 5 * sprintSize
	require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

	// the transaction is processed and there should be a success events
	logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum, childEthEndpoint)
	require.NoError(t, err)

	// assert that all deposits are executed successfully
	checkStateSyncResultLogs(t, logs, stateSyncedLogsCount)

	// check admin balance got increased by deposited amount
	balance, err := childEthEndpoint.GetBalance(ethgo.Address(adminAddr), ethgo.Latest)
	require.NoError(t, err)
	require.Equal(t, adminBalanceOnChild, balance)

	t.Run("bridge native (ERC 20) tokens", func(t *testing.T) {
		// DEPOSIT ERC20 TOKENS
		// send a few transactions to the bridge
		require.NoError(
			t,
			cluster.Bridge.Deposit(
				common.ERC20,
				polybftCfg.Bridge.RootNativeERC20Addr,
				polybftCfg.Bridge.RootERC20PredicateAddr,
				rootHelper.TestAccountPrivKey,
				strings.Join(receivers[:], ","),
				strings.Join(depositAmounts[:], ","),
				"",
				cluster.Bridge.JSONRPCAddr(),
				rootHelper.TestAccountPrivKey,
				false),
		)

		finalBlockNum := 10 * sprintSize
		// wait for a couple of sprints
		require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

		// the transactions are processed and there should be a success events
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum, childEthEndpoint)
		require.NoError(t, err)

		// because of the admin deposit on child chain at the beginning
		totalTransfers := transfersCount + 1

		// assert that all deposits are executed successfully
		checkStateSyncResultLogs(t, logs, totalTransfers)

		// check receivers balances got increased by deposited amount
		// because we premined validators
		expectedBalance := new(big.Int).Add(command.DefaultPremineBalance, depositAmount)

		for _, receiver := range receivers {
			balance, err := childEthEndpoint.GetBalance(ethgo.Address(types.StringToAddress(receiver)), ethgo.Latest)
			require.NoError(t, err)
			require.Equal(t, expectedBalance, balance)
		}

		t.Log("Deposits were successfully processed")

		// WITHDRAW ERC20 TOKENS
		rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
		require.NoError(t, err)

		senderAccount, err := sidechain.GetAccountFromDir(validatorSrv.DataDir())
		require.NoError(t, err)

		t.Logf("Withdraw sender: %s\n", senderAccount.Ecdsa.Address())

		rawKey, err := senderAccount.Ecdsa.MarshallPrivateKey()
		require.NoError(t, err)

		// send withdraw transaction.
		// It should fail because sender is not allow-listed.
		err = cluster.Bridge.Withdraw(
			common.ERC20,
			hex.EncodeToString(rawKey),
			strings.Join(receivers[:], ","),
			strings.Join(withdrawAmounts[:], ","),
			"",
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC20PredicateContract,
			contracts.NativeERC20TokenContract,
			false)
		require.Error(t, err)

		// add account to bridge allow list
		setAccessListRole(t, cluster, contracts.AllowListBridgeAddr, senderAccount.Address(), addresslist.EnabledRole, admin)

		// try to withdraw again
		err = cluster.Bridge.Withdraw(
			common.ERC20,
			hex.EncodeToString(rawKey),
			strings.Join(receivers[:], ","),
			strings.Join(withdrawAmounts[:], ","),
			"",
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC20PredicateContract,
			contracts.NativeERC20TokenContract,
			false)
		require.NoError(t, err)

		// add account to bridge block list
		setAccessListRole(t, cluster, contracts.BlockListBridgeAddr, senderAccount.Address(), addresslist.EnabledRole, admin)

		// it should fail now because sender accont is in the block list
		err = cluster.Bridge.Withdraw(
			common.ERC20,
			hex.EncodeToString(rawKey),
			strings.Join(receivers[:], ","),
			strings.Join(withdrawAmounts[:], ","),
			"",
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC20PredicateContract,
			contracts.NativeERC20TokenContract,
			false)
		require.ErrorContains(t, err, "failed to send withdraw transaction")

		currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
		require.NoError(t, err)

		t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

		currentEpoch := currentExtra.Checkpoint.EpochNumber

		require.NoError(t, waitForRootchainEpoch(currentEpoch, 3*time.Minute,
			rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

		exitHelper := polybftCfg.Bridge.ExitHelperAddr
		childJSONRPC := validatorSrv.JSONRPCAddr()

		oldBalances := map[types.Address]*big.Int{}
		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), polybftCfg.Bridge.RootNativeERC20Addr, rootchainTxRelayer)
			oldBalances[types.StringToAddress(receiver)] = balance
		}

		for exitEventID := uint64(1); exitEventID <= uint64(transfersCount); exitEventID++ {
			// send exit transaction to exit helper
			err = cluster.Bridge.SendExitTransaction(exitHelper, exitEventID, childJSONRPC)
			require.NoError(t, err)
		}

		// assert that receiver's balances on RootERC20 smart contract are expected
		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), polybftCfg.Bridge.RootNativeERC20Addr, rootchainTxRelayer)
			require.Equal(t, oldBalances[types.StringToAddress(receiver)].Add(
				oldBalances[types.StringToAddress(receiver)], withdrawAmount), balance)
		}
	})
}

func TestE2E_Bridge_Transfers_WithRootTrackerPollInterval(t *testing.T) {
	var (
		numBlockConfirmations = uint64(2)
		epochSize             = 30
		sprintSize            = uint64(5)
		rootPollInterval      = 5 * time.Second
		numberOfAttempts      = uint64(4)
		stateSyncedLogsCount  = 1
	)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize),
		framework.WithNumBlockConfirmations(numBlockConfirmations),
		framework.WithRootTrackerPollInterval(rootPollInterval),
		framework.WithTestRewardToken(),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]
	senderAccount, err := sidechain.GetAccountFromDir(validatorSrv.DataDir())
	require.NoError(t, err)

	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	// bridge some tokens for first validator to child chain
	tokensToDeposit := ethgo.Ether(10)

	require.NoError(t, cluster.Bridge.Deposit(
		common.ERC20,
		polybftCfg.Bridge.RootNativeERC20Addr,
		polybftCfg.Bridge.RootERC20PredicateAddr,
		rootHelper.TestAccountPrivKey,
		senderAccount.Address().String(),
		tokensToDeposit.String(),
		"",
		cluster.Bridge.JSONRPCAddr(),
		rootHelper.TestAccountPrivKey,
		false),
	)

	// wait for a couple of sprints
	finalBlockNum := 5 * sprintSize

	// the transaction is processed and there should be a success event
	var stateSyncedResult contractsapi.StateSyncResultEvent

	for i := uint64(0); i < numberOfAttempts; i++ {
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum+i*sprintSize, childEthEndpoint)
		require.NoError(t, err)

		if len(logs) == stateSyncedLogsCount || i == numberOfAttempts-1 {
			// assert that all deposits are executed successfully
			checkStateSyncResultLogs(t, logs, stateSyncedLogsCount)

			break
		}

		require.NoError(t, cluster.WaitForBlock(finalBlockNum+(i+1)*sprintSize, 2*time.Minute))
	}

	// check validator balance got increased by deposited amount
	balance, err := childEthEndpoint.GetBalance(ethgo.Address(senderAccount.Address()), ethgo.Latest)
	require.NoError(t, err)

	// because we premined the validators
	expectedBalance := new(big.Int).Add(command.DefaultPremineBalance, tokensToDeposit)
	require.Equal(t, expectedBalance, balance)
}

func TestE2E_Bridge_NonMintableERC20Token_WithPremine(t *testing.T) {
	var (
		numBlockConfirmations = uint64(2)
		epochSize             = 10
		sprintSize            = uint64(5)
		numberOfAttempts      = uint64(4)
		stateSyncedLogsCount  = 2
		exitEventsCount       = uint64(2)
		tokensToTransfer      = ethgo.Gwei(10)
		bigZero               = big.NewInt(0)
	)

	nonValidatorKey, err := wallet.GenerateKey()
	require.NoError(t, err)

	nonValidatorKeyRaw, err := nonValidatorKey.MarshallPrivateKey()
	require.NoError(t, err)

	// start cluster with default, non-mintable native erc20 root token
	// with london fork enabled
	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(epochSize),
		framework.WithNumBlockConfirmations(numBlockConfirmations),
		// this enables London (EIP-1559) fork
		framework.WithBurnContract(&polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.StringToAddress("0xBurnContractAddress")}),
		framework.WithTestRewardToken(),
		framework.WithSecretsCallback(func(_ []types.Address, tcc *framework.TestClusterConfig) {
			nonValidatorKeyString := hex.EncodeToString(nonValidatorKeyRaw)

			// do premine to a non validator address
			tcc.Premine = append(tcc.Premine,
				fmt.Sprintf("%s:%s:%s",
					nonValidatorKey.Address(),
					command.DefaultPremineBalance.String(),
					nonValidatorKeyString))
		}),
	)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	childEthEndpoint := cluster.Servers[0].JSONRPC().Eth()

	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	checkBalancesFn := func(address types.Address, rootExpected, childExpected *big.Int) {
		t.Log("Checking balance of native ERC20 token on root and child", "Address", address,
			"Root expected", rootExpected, "Child Expected", childExpected)

		balance := erc20BalanceOf(t, address,
			polybftCfg.Bridge.RootNativeERC20Addr, rootchainTxRelayer)
		t.Log("Balance of native ERC20 token on root", balance, "Address", address)
		require.Equal(t, rootExpected, balance)

		balance, err = childEthEndpoint.GetBalance(ethgo.Address(address), ethgo.Latest)
		require.NoError(t, err)
		t.Log("Balance of native ERC20 token on child", balance, "Address", address)
		require.True(t, balance.Cmp(childExpected) >= 0) // because of London fork
	}

	t.Run("check the balances at the beginning", func(t *testing.T) {
		// check the balances on root and child at the beginning to see if they are as expected
		checkBalancesFn(types.Address(nonValidatorKey.Address()), bigZero, command.DefaultPremineBalance)

		for _, server := range cluster.Servers {
			validatorAccount, err := sidechain.GetAccountFromDir(server.DataDir())
			require.NoError(t, err)

			checkBalancesFn(validatorAccount.Address(), bigZero, command.DefaultPremineBalance)
		}
	})

	// this test case will check first if they can withdraw some of the premined amount of non-mintable token
	t.Run("Do a withdraw for premined validator address and premined non-validator address", func(t *testing.T) {
		validatorSrv := cluster.Servers[1]
		validatorAcc, err := sidechain.GetAccountFromDir(validatorSrv.DataDir())
		require.NoError(t, err)

		validatorRawKey, err := validatorAcc.Ecdsa.MarshallPrivateKey()
		require.NoError(t, err)

		err = cluster.Bridge.Withdraw(
			common.ERC20,
			hex.EncodeToString(validatorRawKey),
			validatorAcc.Address().String(),
			tokensToTransfer.String(),
			"",
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC20PredicateContract,
			contracts.NativeERC20TokenContract,
			false)
		require.NoError(t, err)

		validatorBalanceAfterWithdraw, err := childEthEndpoint.GetBalance(
			ethgo.Address(validatorAcc.Address()), ethgo.Latest)
		require.NoError(t, err)

		err = cluster.Bridge.Withdraw(
			common.ERC20,
			hex.EncodeToString(nonValidatorKeyRaw),
			nonValidatorKey.Address().String(),
			tokensToTransfer.String(),
			"",
			validatorSrv.JSONRPCAddr(),
			contracts.ChildERC20PredicateContract,
			contracts.NativeERC20TokenContract,
			false)
		require.NoError(t, err)

		nonValidatorBalanceAfterWithdraw, err := childEthEndpoint.GetBalance(
			nonValidatorKey.Address(), ethgo.Latest)
		require.NoError(t, err)

		currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
		require.NoError(t, err)

		t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

		currentEpoch := currentExtra.Checkpoint.EpochNumber

		exitHelper := polybftCfg.Bridge.ExitHelperAddr
		childJSONRPC := validatorSrv.JSONRPCAddr()

		successfulExitTransactions := make([]bool, exitEventsCount)

		for i := uint64(0); i < numberOfAttempts; i++ {
			t.Log("Number of attempts: ", i+1)

			require.NoError(t, waitForRootchainEpoch(currentEpoch+i, 3*time.Minute,
				rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

			for exitEventID := uint64(1); exitEventID <= exitEventsCount; exitEventID++ {
				if successfulExitTransactions[exitEventID-1] {
					continue
				}

				// send exit transaction to exit helper
				if err = cluster.Bridge.SendExitTransaction(exitHelper, exitEventID, childJSONRPC); err == nil {
					successfulExitTransactions[exitEventID-1] = true

					continue
				}

				if i == numberOfAttempts-1 {
					require.NoError(t, err)
				}
			}

			allExitsSuccessfull := true
			for _, isSuccessfull := range successfulExitTransactions {
				if !isSuccessfull {
					allExitsSuccessfull = false

					break
				}
			}

			if allExitsSuccessfull {
				break
			}
		}

		// assert that receiver's balances on RootERC20 smart contract are expected
		checkBalancesFn(validatorAcc.Address(), tokensToTransfer, validatorBalanceAfterWithdraw)
		checkBalancesFn(types.Address(nonValidatorKey.Address()), tokensToTransfer, nonValidatorBalanceAfterWithdraw)
	})

	t.Run("Do a deposit to some validator and non-validator address", func(t *testing.T) {
		validatorSrv := cluster.Servers[4]
		validatorAcc, err := sidechain.GetAccountFromDir(validatorSrv.DataDir())
		require.NoError(t, err)

		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			polybftCfg.Bridge.RootNativeERC20Addr,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			rootHelper.TestAccountPrivKey,
			strings.Join([]string{validatorAcc.Address().String(), nonValidatorKey.Address().String()}, ","),
			strings.Join([]string{tokensToTransfer.String(), tokensToTransfer.String()}, ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			rootHelper.TestAccountPrivKey,
			false),
		)

		currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		// wait for a couple of sprints
		finalBlockNum := currentBlock.Number + 5*sprintSize

		// the transaction is processed and there should be a success event
		var stateSyncedResult contractsapi.StateSyncResultEvent

		for i := uint64(0); i < numberOfAttempts; i++ {
			logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum+i*sprintSize, childEthEndpoint)
			require.NoError(t, err)

			if len(logs) == stateSyncedLogsCount || i == numberOfAttempts-1 {
				// assert that all deposits are executed successfully
				checkStateSyncResultLogs(t, logs, stateSyncedLogsCount)

				break
			}

			require.NoError(t, cluster.WaitForBlock(finalBlockNum+(i+1)*sprintSize, 2*time.Minute))
		}
	})
}
