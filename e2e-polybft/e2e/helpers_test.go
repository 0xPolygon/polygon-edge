package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/addresslist"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

// getCheckpointManagerValidators queries rootchain validator set on CheckpointManager contract
func getCheckpointManagerValidators(relayer txrelayer.TxRelayer, checkpointManagerAddr ethgo.Address) ([]*polybft.ValidatorInfo, error) {
	validatorsCountRaw, err := ABICall(relayer, contractsapi.CheckpointManager,
		checkpointManagerAddr, ethgo.ZeroAddress, "currentValidatorSetLength")
	if err != nil {
		return nil, err
	}

	validatorsCount, err := common.ParseUint64orHex(&validatorsCountRaw)
	if err != nil {
		return nil, err
	}

	currentValidatorSetMethod := contractsapi.CheckpointManager.Abi.GetMethod("currentValidatorSet")
	validators := make([]*polybft.ValidatorInfo, validatorsCount)

	for i := 0; i < int(validatorsCount); i++ {
		validatorRaw, err := ABICall(relayer, contractsapi.CheckpointManager,
			checkpointManagerAddr, ethgo.ZeroAddress, "currentValidatorSet", i)
		if err != nil {
			return nil, err
		}

		validatorSetRaw, err := hex.DecodeString(validatorRaw[2:])
		if err != nil {
			return nil, err
		}

		decodedResults, err := currentValidatorSetMethod.Outputs.Decode(validatorSetRaw)
		if err != nil {
			return nil, err
		}

		results, ok := decodedResults.(map[string]interface{})
		if !ok {
			return nil, errors.New("failed to decode validator")
		}

		//nolint:forcetypeassert
		validators[i] = &polybft.ValidatorInfo{
			Address: results["_address"].(ethgo.Address),
			Stake:   results["votingPower"].(*big.Int),
		}
	}

	return validators, nil
}

func ABICall(relayer txrelayer.TxRelayer, artifact *artifact.Artifact, contractAddress ethgo.Address, senderAddr ethgo.Address, method string, params ...interface{}) (string, error) {
	input, err := artifact.Abi.GetMethod(method).Encode(params)
	if err != nil {
		return "", err
	}

	return relayer.Call(senderAddr, contractAddress, input)
}

func ABITransaction(relayer txrelayer.TxRelayer, key ethgo.Key, artifact *artifact.Artifact, contractAddress ethgo.Address, method string, params ...interface{}) (*ethgo.Receipt, error) {
	input, err := artifact.Abi.GetMethod(method).Encode(params)
	if err != nil {
		return nil, err
	}

	return relayer.SendTransaction(&ethgo.Transaction{
		To:    &contractAddress,
		Input: input,
	}, key)
}

func getExitProof(rpcAddress string, exitID uint64) (types.Proof, error) {
	query := struct {
		Jsonrpc string   `json:"jsonrpc"`
		Method  string   `json:"method"`
		Params  []string `json:"params"`
		ID      int      `json:"id"`
	}{
		"2.0",
		"bridge_generateExitProof",
		[]string{fmt.Sprintf("0x%x", exitID)},
		1,
	}

	d, err := json.Marshal(query)
	if err != nil {
		return types.Proof{}, err
	}

	resp, err := http.Post(rpcAddress, "application/json", bytes.NewReader(d))
	if err != nil {
		return types.Proof{}, err
	}

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.Proof{}, err
	}

	rspProof := struct {
		Result types.Proof `json:"result"`
	}{}

	err = json.Unmarshal(s, &rspProof)
	if err != nil {
		return types.Proof{}, err
	}

	return rspProof.Result, nil
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
func getCheckpointBlockNumber(l1Relayer txrelayer.TxRelayer, checkpointManagerAddr ethgo.Address) (uint64, error) {
	checkpointBlockNumRaw, err := ABICall(l1Relayer, contractsapi.CheckpointManager,
		checkpointManagerAddr, ethgo.ZeroAddress, "currentCheckpointBlockNumber")
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
			ethgo.Address(checkpointManager), ethgo.ZeroAddress, "currentEpoch")
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
	role addresslist.Role, aclAdmin ethgo.Key) {
	t.Helper()

	var updateRoleFn *abi.Method

	switch role {
	case addresslist.AdminRole:
		updateRoleFn = addresslist.SetAdminFunc

		break
	case addresslist.EnabledRole:
		updateRoleFn = addresslist.SetEnabledFunc

		break
	case addresslist.NoRole:
		updateRoleFn = addresslist.SetNoneFunc

		break
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

	balanceRaw, err := relayer.Call(ethgo.ZeroAddress, ethgo.Address(tokenAddr), balanceOfInput)
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

	ownerRaw, err := relayer.Call(ethgo.ZeroAddress, ethgo.Address(tokenAddr), ownerOfInput)
	require.NoError(t, err)

	return types.StringToAddress(ownerRaw)
}

// queryNativeERC20Metadata returns some meta data user requires from native erc20 token
func queryNativeERC20Metadata(t *testing.T, funcName string, abiType *abi.Type, relayer txrelayer.TxRelayer) interface{} {
	t.Helper()

	valueHex, err := ABICall(relayer, contractsapi.NativeERC20Mintable,
		ethgo.Address(contracts.NativeERC20TokenContract),
		ethgo.ZeroAddress, funcName)
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

	childTokenRaw, err := relayer.Call(ethgo.ZeroAddress, ethgo.Address(predicateAddr), input)
	require.NoError(t, err)

	return types.StringToAddress(childTokenRaw)
}

func getLastExitEventID(t *testing.T, relayer txrelayer.TxRelayer) uint64 {
	t.Helper()

	exitEventsCounterFn := contractsapi.L2StateSender.Abi.Methods["counter"]

	input, err := exitEventsCounterFn.Encode([]interface{}{})
	require.NoError(t, err)

	exitEventIDRaw, err := relayer.Call(ethgo.ZeroAddress, ethgo.Address(contracts.L2StateSenderContract), input)
	require.NoError(t, err)

	exitEventID, err := common.ParseUint64orHex(&exitEventIDRaw)
	require.NoError(t, err)

	return exitEventID
}
