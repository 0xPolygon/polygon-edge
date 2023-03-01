package e2e

import (
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

type e2eStateProvider struct {
	txRelayer txrelayer.TxRelayer
}

func (s *e2eStateProvider) Call(contractAddr ethgo.Address, input []byte, opts *contract.CallOpts) ([]byte, error) {
	response, err := s.txRelayer.Call(ethgo.Address(types.ZeroAddress), contractAddr, input)
	if err != nil {
		return nil, err
	}

	return hex.DecodeHex(response)
}

func (s *e2eStateProvider) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	return nil, errors.New("send txn is not supported")
}

// isExitEventProcessed queries ExitHelper and as a result returns indication whether given exit event id is processed
func isExitEventProcessed(exitEventID uint64, exitHelper ethgo.Address, rootTxRelayer txrelayer.TxRelayer) (bool, error) {
	result, err := ABICall(
		rootTxRelayer,
		contractsapi.ExitHelper,
		exitHelper,
		ethgo.ZeroAddress,
		"processedExits",
		new(big.Int).SetUint64(exitEventID))
	if err != nil {
		return false, err
	}

	isProcessed, err := types.ParseUint64orHex(&result)
	if err != nil {
		return false, err
	}

	return isProcessed == uint64(1), nil
}

// getRootchainValidators queries rootchain validator set
func getRootchainValidators(relayer txrelayer.TxRelayer, checkpointManagerAddr ethgo.Address) ([]*polybft.ValidatorInfo, error) {
	validatorsCountRaw, err := ABICall(relayer, contractsapi.CheckpointManager,
		checkpointManagerAddr, ethgo.ZeroAddress, "currentValidatorSetLength")
	if err != nil {
		return nil, err
	}

	validatorsCount, err := types.ParseUint64orHex(&validatorsCountRaw)
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
			Address:    results["_address"].(ethgo.Address),
			TotalStake: results["votingPower"].(*big.Int),
		}
	}

	return validators, nil
}
