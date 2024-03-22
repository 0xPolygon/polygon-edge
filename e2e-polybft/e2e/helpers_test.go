package e2e

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/addresslist"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const nativeTokenNonMintableConfig = "Blade:BLD:18:false"

func ABICall(relayer txrelayer.TxRelayer, artifact *contracts.Artifact, contractAddress types.Address, senderAddr types.Address, method string, params ...interface{}) (string, error) {
	input, err := artifact.Abi.GetMethod(method).Encode(params)
	if err != nil {
		return "", err
	}

	return relayer.Call(senderAddr, contractAddress, input)
}

func ABITransaction(relayer txrelayer.TxRelayer, key crypto.Key, artifact *contracts.Artifact, contractAddress types.Address, method string, params ...interface{}) (*ethgo.Receipt, error) {
	input, err := artifact.Abi.GetMethod(method).Encode(params)
	if err != nil {
		return nil, err
	}

	tx := types.NewTx(types.NewLegacyTx(
		types.WithTo(&contractAddress),
		types.WithInput(input),
	))

	return relayer.SendTransaction(tx, key)
}

// checkStateSyncResultLogs is helper function which parses given StateSyncResultEvent event's logs,
// extracts status topic value and makes assertions against it.
func checkStateSyncResultLogs(
	t *testing.T,
	logs []*ethgo.Log,
	expectedCount int,
) {
	t.Helper()
	require.Equal(t, expectedCount, len(logs))

	var stateSyncResultEvent contractsapi.StateSyncResultEvent
	for _, log := range logs {
		doesMatch, err := stateSyncResultEvent.ParseLog(log)
		require.NoError(t, err)
		require.True(t, doesMatch)

		t.Logf("Block Number=%d, Decoded Log=%+v\n", log.BlockNumber, stateSyncResultEvent)

		require.True(t, stateSyncResultEvent.Status)
	}
}

// getCheckpointBlockNumber gets current checkpoint block number from checkpoint manager smart contract
func getCheckpointBlockNumber(l1Relayer txrelayer.TxRelayer, checkpointManagerAddr types.Address) (uint64, error) {
	checkpointBlockNumRaw, err := ABICall(l1Relayer, contractsapi.CheckpointManager,
		checkpointManagerAddr, types.ZeroAddress, "currentCheckpointBlockNumber")
	if err != nil {
		return 0, err
	}

	actualCheckpointBlock, err := common.ParseUint64orHex(&checkpointBlockNumRaw)
	if err != nil {
		return 0, err
	}

	return actualCheckpointBlock, nil
}

// waitForRootchainEpoch blocks for some predefined timeout to reach target epoch
func waitForRootchainEpoch(targetEpoch uint64, timeout time.Duration,
	rootchainTxRelayer txrelayer.TxRelayer, checkpointManager types.Address) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return errors.New("root chain hasn't progressed to the desired epoch")
		case <-ticker.C:
		}

		rootchainEpochRaw, err := ABICall(rootchainTxRelayer, contractsapi.CheckpointManager,
			checkpointManager, types.ZeroAddress, "currentEpoch")
		if err != nil {
			return err
		}

		rootchainEpoch, err := common.ParseUint64orHex(&rootchainEpochRaw)
		if err != nil {
			return err
		}

		if rootchainEpoch >= targetEpoch {
			return nil
		}
	}
}

// setAccessListRole sets access list role to appropriate access list precompile
func setAccessListRole(t *testing.T, cluster *framework.TestCluster, precompile, account types.Address,
	role addresslist.Role, aclAdmin *crypto.ECDSAKey) {
	t.Helper()

	var updateRoleFn *abi.Method

	switch role {
	case addresslist.AdminRole:
		updateRoleFn = addresslist.SetAdminFunc
	case addresslist.EnabledRole:
		updateRoleFn = addresslist.SetEnabledFunc
	case addresslist.NoRole:
		updateRoleFn = addresslist.SetNoneFunc
	}

	input, err := updateRoleFn.Encode([]interface{}{account})
	require.NoError(t, err)

	enableSetTxn := cluster.MethodTxn(t, aclAdmin, precompile, input)
	require.NoError(t, enableSetTxn.Wait())

	expectRole(t, cluster, precompile, account, role)
}

func expectRole(t *testing.T, cluster *framework.TestCluster, contract types.Address, addr types.Address, role addresslist.Role) {
	t.Helper()
	out := cluster.Call(t, contract, addresslist.ReadAddressListFunc, addr)

	num, ok := out["0"].(*big.Int)
	if !ok {
		t.Fatal("unexpected")
	}

	require.Equal(t, role.Uint64(), num.Uint64())
}

// getFilteredLogs retrieves Ethereum logs, described by event signature within the block range
func getFilteredLogs(eventSig ethgo.Hash, startBlock, endBlock uint64,
	ethEndpoint *jsonrpc.Eth) ([]*ethgo.Log, error) {
	filter := &ethgo.LogFilter{Topics: [][]*ethgo.Hash{{&eventSig}}}

	filter.SetFromUint64(startBlock)
	filter.SetToUint64(endBlock)

	return ethEndpoint.GetLogs(filter)
}

// erc20BalanceOf returns balance of given account on ERC 20 token
func erc20BalanceOf(t *testing.T, account types.Address, tokenAddr types.Address, relayer txrelayer.TxRelayer) *big.Int {
	t.Helper()

	balanceOfFn := &contractsapi.BalanceOfRootERC20Fn{Account: account}
	balanceOfInput, err := balanceOfFn.EncodeAbi()
	require.NoError(t, err)

	balanceRaw, err := relayer.Call(types.ZeroAddress, tokenAddr, balanceOfInput)
	require.NoError(t, err)
	balance, err := common.ParseUint256orHex(&balanceRaw)
	require.NoError(t, err)

	return balance
}

// erc721OwnerOf returns owner of given ERC 721 token
func erc721OwnerOf(t *testing.T, tokenID *big.Int, tokenAddr types.Address, relayer txrelayer.TxRelayer) types.Address {
	t.Helper()

	ownerOfFn := &contractsapi.OwnerOfChildERC721Fn{TokenID: tokenID}
	ownerOfInput, err := ownerOfFn.EncodeAbi()
	require.NoError(t, err)

	ownerRaw, err := relayer.Call(types.ZeroAddress, tokenAddr, ownerOfInput)
	require.NoError(t, err)

	return types.StringToAddress(ownerRaw)
}

// queryNativeERC20Metadata returns some meta data user requires from native erc20 token
func queryNativeERC20Metadata(t *testing.T, funcName string, abiType *abi.Type, relayer txrelayer.TxRelayer) interface{} {
	t.Helper()

	valueHex, err := ABICall(relayer, contractsapi.NativeERC20Mintable,
		contracts.NativeERC20TokenContract,
		types.ZeroAddress, funcName)
	require.NoError(t, err)

	valueRaw, err := hex.DecodeHex(valueHex)
	require.NoError(t, err)

	var decodedResult map[string]interface{}

	err = abiType.DecodeStruct(valueRaw, &decodedResult)
	require.NoError(t, err)

	return decodedResult["0"]
}

// getChildToken queries child token address for provided root token on the target predicate
func getChildToken(t *testing.T, predicateABI *abi.ABI, predicateAddr types.Address,
	rootToken types.Address, relayer txrelayer.TxRelayer) types.Address {
	t.Helper()

	rootToChildTokenFn, exists := predicateABI.Methods["rootTokenToChildToken"]
	require.True(t, exists, "rootTokenToChildToken function is not found in the provided predicate ABI definition")

	input, err := rootToChildTokenFn.Encode([]interface{}{rootToken})
	require.NoError(t, err)

	childTokenRaw, err := relayer.Call(types.ZeroAddress, predicateAddr, input)
	require.NoError(t, err)

	return types.StringToAddress(childTokenRaw)
}

func getLastExitEventID(t *testing.T, relayer txrelayer.TxRelayer) uint64 {
	t.Helper()

	exitEventsCounterFn := contractsapi.L2StateSender.Abi.Methods["counter"]

	input, err := exitEventsCounterFn.Encode([]interface{}{})
	require.NoError(t, err)

	exitEventIDRaw, err := relayer.Call(types.ZeroAddress, contracts.L2StateSenderContract, input)
	require.NoError(t, err)

	exitEventID, err := common.ParseUint64orHex(&exitEventIDRaw)
	require.NoError(t, err)

	return exitEventID
}

func isExitEventProcessed(t *testing.T, exitHelperAddr types.Address,
	relayer txrelayer.TxRelayer, exitEventID uint64) bool {
	t.Helper()

	processedExitsFn := contractsapi.ExitHelper.Abi.Methods["processedExits"]

	input, err := processedExitsFn.Encode([]interface{}{exitEventID})
	require.NoError(t, err)

	isProcessedRaw, err := relayer.Call(types.ZeroAddress, exitHelperAddr, input)
	require.NoError(t, err)

	isProcessedAsNumber, err := common.ParseUint64orHex(&isProcessedRaw)
	require.NoError(t, err)

	return isProcessedAsNumber == 1
}
