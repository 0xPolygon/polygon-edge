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

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	helperCommon "github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state/runtime/addresslist"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	chainConfigFileName = "genesis.json"
)

func TestE2E_Bridge_RootchainTokensTransfers(t *testing.T) {
	const (
		transfersCount        = 5
		numBlockConfirmations = 2
		// make epoch size long enough, so that all exit events are processed within the same epoch
		epochSize            = 40
		sprintSize           = uint64(5)
		numberOfAttempts     = 7
		stateSyncedLogsCount = 2 // map token and deposit
	)

	var (
		bridgeAmount      = ethgo.Ether(2)
		stateSyncedResult contractsapi.StateSyncResultEvent
	)

	receiversAddrs := make([]types.Address, transfersCount)
	receivers := make([]string, transfersCount)
	amounts := make([]string, transfersCount)
	receiverKeys := make([]string, transfersCount)

	for i := 0; i < transfersCount; i++ {
		key, err := crypto.GenerateECDSAKey()
		require.NoError(t, err)

		rawKey, err := key.MarshallPrivateKey()
		require.NoError(t, err)

		receiverKeys[i] = hex.EncodeToString(rawKey)
		receiversAddrs[i] = types.Address(key.Address())
		receivers[i] = types.Address(key.Address()).String()
		amounts[i] = fmt.Sprintf("%d", bridgeAmount)

		t.Logf("Receiver#%d=%s\n", i+1, receivers[i])
	}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithTestRewardToken(),
		framework.WithNumBlockConfirmations(numBlockConfirmations),
		framework.WithEpochSize(epochSize),
		framework.WithBridge(),
		framework.WithSecretsCallback(func(addrs []types.Address, tcc *framework.TestClusterConfig) {
			for i := 0; i < len(addrs); i++ {
				tcc.StakeAmounts = append(tcc.StakeAmounts, ethgo.Ether(10))
				// premine receivers, so that they are able to do withdrawals
			}

			tcc.Premine = append(tcc.Premine, receivers...)
		}))
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]

	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
	require.NoError(t, err)

	deployerKey, err := bridgeHelper.DecodePrivateKey("")
	require.NoError(t, err)

	deployTx := types.NewTx(types.NewLegacyTx(
		types.WithTo(nil),
		types.WithInput(contractsapi.RootERC20.Bytecode),
	))

	receipt, err := rootchainTxRelayer.SendTransaction(deployTx, deployerKey)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	rootERC20Token := types.Address(receipt.ContractAddress)
	t.Log("Rootchain token address:", rootERC20Token)

	// wait for a couple of sprints
	finalBlockNum := 1 * sprintSize
	require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

	t.Run("bridge ERC20 tokens", func(t *testing.T) {
		// DEPOSIT ERC20 TOKENS
		// send a few transactions to the bridge
		require.NoError(t,
			cluster.Bridge.Deposit(
				common.ERC20,
				rootERC20Token,
				polybftCfg.Bridge.RootERC20PredicateAddr,
				bridgeHelper.TestAccountPrivKey,
				strings.Join(receivers[:], ","),
				strings.Join(amounts[:], ","),
				"",
				cluster.Bridge.JSONRPCAddr(),
				bridgeHelper.TestAccountPrivKey,
				false,
			))

		finalBlockNum := 10 * sprintSize
		// wait for a couple of sprints
		require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

		// the bridge transactions are processed and there should be a success state sync events
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum, childEthEndpoint)
		require.NoError(t, err)

		// assert that all deposits are executed successfully
		// because of the token mapping with the first deposit
		checkStateSyncResultLogs(t, logs, transfersCount+1)

		// get child token address
		childERC20Token := getChildToken(t, contractsapi.RootERC20Predicate.Abi,
			polybftCfg.Bridge.RootERC20PredicateAddr, rootERC20Token, rootchainTxRelayer)

		// check receivers balances got increased by deposited amount
		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), childERC20Token, txRelayer)
			require.Equal(t, bridgeAmount, balance)
		}

		t.Log("Deposits were successfully processed")

		require.NoError(t, cluster.WaitForBlock(uint64(epochSize), 2*time.Minute))

		// WITHDRAW ERC20 TOKENS
		// send withdraw transaction
		for i, senderKey := range receiverKeys {
			err = cluster.Bridge.Withdraw(
				common.ERC20,
				senderKey,
				receivers[i],
				amounts[i],
				"",
				validatorSrv.JSONRPCAddr(),
				contracts.ChildERC20PredicateContract,
				childERC20Token,
				false)
			require.NoError(t, err)
		}

		require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
			for i := range receivers {
				if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, uint64(i+1)) {
					return false
				}
			}

			return true
		}))

		for _, receiver := range receivers {
			// assert that receiver's balance on RootERC20 smart contract is as expected
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), rootERC20Token, rootchainTxRelayer)
			require.True(t, bridgeAmount.Cmp(balance) == 0)
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
		commitmentIDRaw, err := txRelayer.Call(types.ZeroAddress, contracts.StateReceiverContract, lastCommittedIDInput)
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
			rootERC20Token,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			bridgeHelper.TestAccountPrivKey,
			strings.Join(receivers[:depositsSubset], ","),
			strings.Join(amounts[:depositsSubset], ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			bridgeHelper.TestAccountPrivKey,
			false),
		)

		// wait for a few more sprints
		midBlockNumber := initialBlockNum + 2*sprintSize
		require.NoError(t, cluster.WaitForBlock(midBlockNumber, 2*time.Minute))

		// check that we submitted the minimal commitment to smart contract
		commitmentIDRaw, err = txRelayer.Call(types.ZeroAddress,
			contracts.StateReceiverContract, lastCommittedIDInput)
		require.NoError(t, err)

		lastCommittedID, err := helperCommon.ParseUint64orHex(&commitmentIDRaw)
		require.NoError(t, err)
		require.Equal(t, initialCommittedID+depositsSubset, lastCommittedID)

		// send some more transactions to the bridge to build another commitment in epoch
		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			rootERC20Token,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			bridgeHelper.TestAccountPrivKey,
			strings.Join(receivers[depositsSubset:], ","),
			strings.Join(amounts[depositsSubset:], ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			bridgeHelper.TestAccountPrivKey,
			false),
		)

		finalBlockNum := midBlockNumber + 5*sprintSize
		// wait for a few more sprints
		require.NoError(t, cluster.WaitForBlock(midBlockNumber+5*sprintSize, 3*time.Minute))

		// check that we submitted the minimal commitment to smart contract
		commitmentIDRaw, err = txRelayer.Call(types.ZeroAddress, contracts.StateReceiverContract, lastCommittedIDInput)
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
}

func TestE2E_Bridge_ERC721Transfer(t *testing.T) {
	const (
		transfersCount       = 4
		epochSize            = 5
		numberOfAttempts     = 4
		stateSyncedLogsCount = 2
	)

	receiverKeys := make([]string, transfersCount)
	receivers := make([]string, transfersCount)
	receiversAddrs := make([]types.Address, transfersCount)
	tokenIDs := make([]string, transfersCount)

	for i := 0; i < transfersCount; i++ {
		key, err := crypto.GenerateECDSAKey()
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
		framework.WithPremine(receiversAddrs...),
		framework.WithBridge(),
		framework.WithSecretsCallback(func(addrs []types.Address, tcc *framework.TestClusterConfig) {
			for i := 0; i < len(addrs); i++ {
				tcc.StakeAmounts = append(tcc.StakeAmounts, ethgo.Ether(10))
			}
		}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	rootchainDeployer, err := bridgeHelper.DecodePrivateKey("")
	require.NoError(t, err)

	deployTx := types.NewTx(&types.LegacyTx{
		BaseTx: &types.BaseTx{
			To:    nil,
			Input: contractsapi.RootERC721.Bytecode,
		},
	})

	// deploy root ERC 721 token
	receipt, err := rootchainTxRelayer.SendTransaction(deployTx, rootchainDeployer)
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
			bridgeHelper.TestAccountPrivKey,
			strings.Join(receivers[:], ","),
			"",
			strings.Join(tokenIDs[:], ","),
			cluster.Bridge.JSONRPCAddr(),
			bridgeHelper.TestAccountPrivKey,
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

	require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
		for i := 1; i <= transfersCount; i++ {
			if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, uint64(i)) {
				return false
			}
		}

		return true
	}))

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

	receiverKeys := make([]string, transfersCount)
	receivers := make([]string, transfersCount)
	receiversAddrs := make([]types.Address, transfersCount)
	amounts := make([]string, transfersCount)
	tokenIDs := make([]string, transfersCount)

	for i := 0; i < transfersCount; i++ {
		key, err := crypto.GenerateECDSAKey()
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
		framework.WithPremine(receiversAddrs...),
		framework.WithBridge(),
		framework.WithSecretsCallback(func(addrs []types.Address, tcc *framework.TestClusterConfig) {
			for i := 0; i < len(addrs); i++ {
				tcc.StakeAmounts = append(tcc.StakeAmounts, ethgo.Ether(10))
			}
		}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	rootchainDeployer, err := bridgeHelper.DecodePrivateKey("")
	require.NoError(t, err)

	deployTx := types.NewTx(&types.LegacyTx{
		BaseTx: &types.BaseTx{
			To:    nil,
			Input: contractsapi.RootERC1155.Bytecode,
		},
	})

	// deploy root ERC 1155 token
	receipt, err := rootchainTxRelayer.SendTransaction(deployTx, rootchainDeployer)
	require.NoError(t, err)

	rootERC1155Addr := types.Address(receipt.ContractAddress)

	// DEPOSIT ERC1155 TOKENS
	// send a few transactions to the bridge
	require.NoError(
		t,
		cluster.Bridge.Deposit(
			common.ERC1155,
			types.Address(rootERC1155Addr),
			polybftCfg.Bridge.RootERC1155PredicateAddr,
			bridgeHelper.TestAccountPrivKey,
			strings.Join(receivers[:], ","),
			strings.Join(amounts[:], ","),
			strings.Join(tokenIDs[:], ","),
			cluster.Bridge.JSONRPCAddr(),
			bridgeHelper.TestAccountPrivKey,
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

		balanceRaw, err := txRelayer.Call(types.ZeroAddress, l2ChildTokenAddr, balanceInput)
		require.NoError(t, err)

		balance, err := helperCommon.ParseUint256orHex(&balanceRaw)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(int64(amount)), balance)
	}

	t.Log("Deposits were successfully processed")

	// WITHDRAW ERC1155 TOKENS
	senderAccount, err := validatorHelper.GetAccountFromDir(cluster.Servers[0].DataDir())
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

	require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
		for i := 1; i <= transfersCount; i++ {
			if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, uint64(i)) {
				return false
			}
		}

		return true
	}))

	// assert that receiver's balances on RootERC1155 smart contract are expected
	for i, receiver := range receivers {
		balanceOfFn := &contractsapi.BalanceOfRootERC1155Fn{
			Account: types.StringToAddress(receiver),
			ID:      big.NewInt(int64(i + 1)),
		}

		balanceInput, err := balanceOfFn.EncodeAbi()
		require.NoError(t, err)

		balanceRaw, err := rootchainTxRelayer.Call(types.ZeroAddress, rootERC1155Addr, balanceInput)
		require.NoError(t, err)

		balance, err := helperCommon.ParseUint256orHex(&balanceRaw)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(amount), balance)
	}
}

func TestE2E_Bridge_ChildchainTokensTransfer(t *testing.T) {
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

	admin, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	adminAddr := types.Address(admin.Address())

	for i := uint64(0); i < transfersCount; i++ {
		key, err := crypto.GenerateECDSAKey()
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
		framework.WithBridge(),
		framework.WithBridgeBlockListAdmin(adminAddr),
		framework.WithPremine(append(depositors, adminAddr)...)) //nolint:makezero
	defer cluster.Stop()

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]
	childEthEndpoint := validatorSrv.JSONRPC().Eth()

	// fund accounts on rootchain
	require.NoError(t, validatorSrv.RootchainFundFor(depositors, funds))

	cluster.WaitForReady(t)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	childchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
	require.NoError(t, err)

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
		require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
			for i := uint64(1); i <= transfersCount+1; i++ {
				if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, i) {
					return false
				}
			}

			return true
		}))

		// retrieve child mintable token address from both chains and make sure they are the same
		l1ChildToken := getChildToken(t, contractsapi.ChildMintableERC20Predicate.Abi, polybftCfg.Bridge.ChildMintableERC20PredicateAddr,
			rootToken, rootchainTxRelayer)
		l2ChildToken := getChildToken(t, contractsapi.RootMintableERC20Predicate.Abi, contracts.RootMintableERC20PredicateContract,
			rootToken, childchainTxRelayer)

		t.Log("L1 child token", l1ChildToken)
		t.Log("L2 child token", l2ChildToken)
		require.Equal(t, l1ChildToken, l2ChildToken)

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

		require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
			for i := initialExitEventID; i <= initialExitEventID+transfersCount; i++ {
				if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, i) {
					return false
				}
			}

			return true
		}))

		// retrieve child token addresses on both chains and make sure they are the same
		l1ChildToken := getChildToken(t, contractsapi.ChildMintableERC721Predicate.Abi, polybftCfg.Bridge.ChildMintableERC721PredicateAddr,
			types.Address(rootERC721Token), rootchainTxRelayer)
		l2ChildToken := getChildToken(t, contractsapi.RootMintableERC721Predicate.Abi, contracts.RootMintableERC721PredicateContract,
			types.Address(rootERC721Token), childchainTxRelayer)

		t.Log("L1 child token", l1ChildToken)
		t.Log("L2 child token", l2ChildToken)
		require.Equal(t, l1ChildToken, l2ChildToken)

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
	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(5),
		framework.WithTestRewardToken(),
		framework.WithBridge())
	defer cluster.Stop()

	// initialize tx relayer used to query CheckpointManager smart contract
	rootChainRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	checkpointManagerAddr := polybftCfg.Bridge.CheckpointManagerAddr

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

func TestE2E_Bridge_Transfers_AccessLists(t *testing.T) {
	var (
		transfersCount = 5
		depositAmount  = ethgo.Ether(5)
		withdrawAmount = ethgo.Ether(1)
		// make epoch size long enough, so that all exit events are processed within the same epoch
		epochSize  = 40
		sprintSize = uint64(5)
	)

	receivers := make([]string, transfersCount)
	depositAmounts := make([]string, transfersCount)
	withdrawAmounts := make([]string, transfersCount)

	admin, _ := crypto.GenerateECDSAKey()
	adminAddr := types.Address(admin.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNumBlockConfirmations(0),
		framework.WithEpochSize(epochSize),
		framework.WithTestRewardToken(),
		framework.WithRootTrackerPollInterval(3*time.Second),
		framework.WithBridge(),
		framework.WithBridgeAllowListAdmin(adminAddr),
		framework.WithBridgeBlockListAdmin(adminAddr),
		framework.WithSecretsCallback(func(a []types.Address, tcc *framework.TestClusterConfig) {
			for i := 0; i < len(a); i++ {
				receivers[i] = a[i].String()
				depositAmounts[i] = fmt.Sprintf("%d", depositAmount)
				withdrawAmounts[i] = fmt.Sprintf("%d", withdrawAmount)

				t.Logf("Receiver#%d=%s\n", i+1, receivers[i])

				// premine access list admin account
				tcc.Premine = append(tcc.Premine, adminAddr.String())
				tcc.StakeAmounts = append(tcc.StakeAmounts, ethgo.Ether(10))
			}
		}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	validatorSrv := cluster.Servers[0]
	childEthEndpoint := validatorSrv.JSONRPC().Eth()
	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorSrv.JSONRPC()))
	require.NoError(t, err)

	senderAccount, err := validatorHelper.GetAccountFromDir(validatorSrv.DataDir())
	require.NoError(t, err)

	// fund admin on rootchain
	require.NoError(t, validatorSrv.RootchainFundFor([]types.Address{adminAddr}, []*big.Int{ethgo.Ether(1)}))

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	deployerKey, err := bridgeHelper.DecodePrivateKey("")
	require.NoError(t, err)

	deployTx := types.NewTx(types.NewLegacyTx(
		types.WithTo(nil),
		types.WithInput(contractsapi.RootERC20.Bytecode),
	))

	// deploy root erc20 token
	receipt, err := rootchainTxRelayer.SendTransaction(deployTx, deployerKey)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)
	rootERC20Token := types.Address(receipt.ContractAddress)

	t.Run("bridge ERC 20 tokens", func(t *testing.T) {
		// DEPOSIT ERC20 TOKENS
		// send a few transactions to the bridge
		require.NoError(
			t,
			cluster.Bridge.Deposit(
				common.ERC20,
				rootERC20Token,
				polybftCfg.Bridge.RootERC20PredicateAddr,
				bridgeHelper.TestAccountPrivKey,
				strings.Join(receivers[:], ","),
				strings.Join(depositAmounts[:], ","),
				"",
				cluster.Bridge.JSONRPCAddr(),
				bridgeHelper.TestAccountPrivKey,
				false),
		)

		finalBlockNum := 10 * sprintSize
		// wait for a couple of sprints
		require.NoError(t, cluster.WaitForBlock(finalBlockNum, 2*time.Minute))

		var stateSyncedResult contractsapi.StateSyncResultEvent

		// the transactions are processed and there should be a success events
		logs, err := getFilteredLogs(stateSyncedResult.Sig(), 0, finalBlockNum, childEthEndpoint)
		require.NoError(t, err)

		// assert that all deposits are executed successfully
		// (token mapping and transferCount of deposits)
		checkStateSyncResultLogs(t, logs, transfersCount+1)

		// get child token address
		childERC20Token := getChildToken(t, contractsapi.RootERC20Predicate.Abi,
			polybftCfg.Bridge.RootERC20PredicateAddr, rootERC20Token, rootchainTxRelayer)

		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), childERC20Token, relayer)
			require.Equal(t, depositAmount, balance)
		}

		t.Log("Deposits were successfully processed")

		// WITHDRAW ERC20 TOKENS
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
			childERC20Token,
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
			childERC20Token,
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

		oldBalances := map[types.Address]*big.Int{}

		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), rootERC20Token, rootchainTxRelayer)
			oldBalances[types.StringToAddress(receiver)] = balance
		}

		require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
			for i := uint64(1); i <= uint64(transfersCount); i++ {
				if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, i) {
					return false
				}
			}

			return true
		}))

		// assert that receiver's balances on RootERC20 smart contract are expected
		for _, receiver := range receivers {
			balance := erc20BalanceOf(t, types.StringToAddress(receiver), rootERC20Token, rootchainTxRelayer)
			require.Equal(t, oldBalances[types.StringToAddress(receiver)].Add(
				oldBalances[types.StringToAddress(receiver)], withdrawAmount), balance)
		}
	})
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

	nonValidatorKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	nonValidatorKeyRaw, err := nonValidatorKey.MarshallPrivateKey()
	require.NoError(t, err)

	rewardWalletKey, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	rewardWalletKeyRaw, err := rewardWalletKey.MarshallPrivateKey()
	require.NoError(t, err)

	// start cluster with default, non-mintable native erc20 root token
	// with london fork enabled
	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithEpochSize(epochSize),
		framework.WithNumBlockConfirmations(numBlockConfirmations),
		framework.WithNativeTokenConfig(nativeTokenNonMintableConfig),
		// this enables London (EIP-1559) fork
		framework.WithBurnContract(&polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.StringToAddress("0xBurnContractAddress")}),
		framework.WithSecretsCallback(func(_ []types.Address, tcc *framework.TestClusterConfig) {
			nonValidatorKeyString := hex.EncodeToString(nonValidatorKeyRaw)
			rewardWalletKeyString := hex.EncodeToString(rewardWalletKeyRaw)

			// do premine to a non validator address
			tcc.Premine = append(tcc.Premine,
				fmt.Sprintf("%s:%s:%s",
					nonValidatorKey.Address(),
					command.DefaultPremineBalance.String(),
					nonValidatorKeyString))

			// do premine to reward wallet address
			tcc.Premine = append(tcc.Premine,
				fmt.Sprintf("%s:%s:%s",
					rewardWalletKey.Address(),
					command.DefaultPremineBalance.String(),
					rewardWalletKeyString))
		}),
	)

	rootchainTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Bridge.JSONRPCAddr()))
	require.NoError(t, err)

	childEthEndpoint := cluster.Servers[0].JSONRPC().Eth()

	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	checkBalancesFn := func(address types.Address, rootExpected, childExpected *big.Int, isValidator bool) {
		offset := ethgo.Gwei(100)
		expectedValue := new(big.Int)

		t.Log("Checking balance of native ERC20 token on root and child", "Address", address,
			"Root expected", rootExpected, "Child Expected", childExpected)

		balance := erc20BalanceOf(t, address,
			polybftCfg.Bridge.RootNativeERC20Addr, rootchainTxRelayer)
		t.Log("Balance of native ERC20 token on root", balance, "Address", address)
		require.Equal(t, rootExpected, balance)

		balance, err = childEthEndpoint.GetBalance(ethgo.Address(address), ethgo.Latest)
		require.NoError(t, err)
		t.Log("Balance of native ERC20 token on child", balance, "Address", address)

		if isValidator {
			require.True(t, balance.Cmp(childExpected) >= 0) // because of London fork
		} else {
			//this check is implemented because non-validators incur fees, potentially resulting in a balance lower than anticipated
			require.True(t, balance.Cmp(expectedValue.Sub(childExpected, offset)) >= 0)
		}
	}

	t.Run("check the balances at the beginning", func(t *testing.T) {
		// check the balances on root and child at the beginning to see if they are as expected
		checkBalancesFn(types.Address(nonValidatorKey.Address()), bigZero, command.DefaultPremineBalance, false)
		checkBalancesFn(types.Address(rewardWalletKey.Address()), bigZero, command.DefaultPremineBalance, true)

		validatorsExpectedBalance := new(big.Int).Sub(command.DefaultPremineBalance, command.DefaultStake)
		for _, server := range cluster.Servers {
			validatorAccount, err := validatorHelper.GetAccountFromDir(server.DataDir())
			require.NoError(t, err)

			checkBalancesFn(validatorAccount.Address(), bigZero, validatorsExpectedBalance, true)
		}
	})

	// this test case will check first if they can withdraw some of the premined amount of non-mintable token
	t.Run("Do a withdraw for premined validator address and premined non-validator address", func(t *testing.T) {
		validatorSrv := cluster.Servers[1]
		validatorAcc, err := validatorHelper.GetAccountFromDir(validatorSrv.DataDir())
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
			ethgo.Address(nonValidatorKey.Address()), ethgo.Latest)
		require.NoError(t, err)

		currentBlock, err := childEthEndpoint.GetBlockByNumber(ethgo.Latest, false)
		require.NoError(t, err)

		currentExtra, err := polybft.GetIbftExtra(currentBlock.ExtraData)
		require.NoError(t, err)

		t.Logf("Latest block number: %d, epoch number: %d\n", currentBlock.Number, currentExtra.Checkpoint.EpochNumber)

		currentEpoch := currentExtra.Checkpoint.EpochNumber

		require.NoError(t, waitForRootchainEpoch(currentEpoch+1, 3*time.Minute,
			rootchainTxRelayer, polybftCfg.Bridge.CheckpointManagerAddr))

		require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
			for exitEventID := uint64(1); exitEventID <= exitEventsCount; exitEventID++ {
				if !isExitEventProcessed(t, polybftCfg.Bridge.ExitHelperAddr, rootchainTxRelayer, exitEventID) {
					return false
				}
			}

			return true
		}))

		// assert that receiver's balances on RootERC20 smart contract are expected
		checkBalancesFn(validatorAcc.Address(), tokensToTransfer, validatorBalanceAfterWithdraw, true)
		checkBalancesFn(types.Address(nonValidatorKey.Address()), tokensToTransfer, nonValidatorBalanceAfterWithdraw, false)
	})

	t.Run("Do a deposit to some validator and non-validator address", func(t *testing.T) {
		validatorSrv := cluster.Servers[4]
		validatorAcc, err := validatorHelper.GetAccountFromDir(validatorSrv.DataDir())
		require.NoError(t, err)

		require.NoError(t, cluster.Bridge.Deposit(
			common.ERC20,
			polybftCfg.Bridge.RootNativeERC20Addr,
			polybftCfg.Bridge.RootERC20PredicateAddr,
			bridgeHelper.TestAccountPrivKey,
			strings.Join([]string{validatorAcc.Address().String(), nonValidatorKey.Address().String()}, ","),
			strings.Join([]string{tokensToTransfer.String(), tokensToTransfer.String()}, ","),
			"",
			cluster.Bridge.JSONRPCAddr(),
			bridgeHelper.TestAccountPrivKey,
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

func TestE2E_Bridge_L1OriginatedNativeToken_ERC20StakingToken(t *testing.T) {
	const (
		epochSize    = 5
		blockTimeout = 30 * time.Second
	)

	var (
		initialStake   = ethgo.Ether(10)
		addedStake     = ethgo.Ether(1)
		stakeTokenAddr = types.StringToAddress("0x2040")
	)

	minter, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNumBlockConfirmations(0),
		framework.WithEpochSize(epochSize),
		framework.WithBridge(),
		framework.WithBladeAdmin(minter.Address().String()),
		framework.WithSecretsCallback(func(addrs []types.Address, tcc *framework.TestClusterConfig) {
			for i := 0; i < len(addrs); i++ {
				tcc.StakeAmounts = append(tcc.StakeAmounts, initialStake)
			}
		}),
		framework.WithNativeTokenConfig(nativeTokenNonMintableConfig),
		framework.WithPredeploy(fmt.Sprintf("%s:RootERC20", stakeTokenAddr)),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	// first validator server(minter)
	firstValidator := cluster.Servers[0]
	// second validator server
	secondValidator := cluster.Servers[1]

	// validator account from second validator
	validatorAccTwo, err := validatorHelper.GetAccountFromDir(secondValidator.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(firstValidator.JSONRPCAddr()))
	require.NoError(t, err)

	mintFn := &contractsapi.MintRootERC20Fn{
		To:     validatorAccTwo.Address(),
		Amount: addedStake,
	}

	mintInput, err := mintFn.EncodeAbi()
	require.NoError(t, err)

	nonNativeErc20 := polybftCfg.StakeTokenAddr

	mintTx := types.NewTx(types.NewDynamicFeeTx(
		types.WithTo(&nonNativeErc20),
		types.WithInput(mintInput),
	))

	receipt, err := relayer.SendTransaction(mintTx, minter)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	secondValidatorInfo, err := validatorHelper.GetValidatorInfo(validatorAccTwo.Ecdsa.Address(), relayer)
	require.NoError(t, err)
	require.True(t, secondValidatorInfo.Stake.Cmp(initialStake) == 0)

	require.NoError(t, cluster.WaitForBlock(epochSize*1, blockTimeout))

	require.NoError(t, secondValidator.Stake(polybftCfg.StakeTokenAddr, addedStake))

	require.NoError(t, cluster.WaitForBlock(epochSize*3, blockTimeout))

	secondValidatorInfo, err = validatorHelper.GetValidatorInfo(validatorAccTwo.Ecdsa.Address(), relayer)
	require.NoError(t, err)

	expectedStakeAmount := new(big.Int).Add(initialStake, addedStake)
	require.Equal(t, expectedStakeAmount, secondValidatorInfo.Stake)

	require.NoError(t, secondValidator.Unstake(addedStake))

	require.NoError(t, cluster.WaitForBlock(epochSize*4, blockTimeout))

	secondValidatorInfo, err = validatorHelper.GetValidatorInfo(validatorAccTwo.Ecdsa.Address(), relayer)
	require.NoError(t, err)
	require.True(t, secondValidatorInfo.Stake.Cmp(initialStake) == 0)
}
