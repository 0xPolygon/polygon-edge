package helper

import (
	"errors"
	"fmt"
	"os"

	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	AmountFlag = "amount"
)

func CheckIfDirectoryExist(dir string) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided directory '%s' doesn't exist", dir)
	}

	return nil
}

func ValidateSecretFlags(dataDir, config string) error {
	if config == "" {
		if dataDir == "" {
			return polybftsecrets.ErrInvalidParams
		} else {
			return CheckIfDirectoryExist(dataDir)
		}
	}

	return nil
}

// GetAccount resolves secrets manager and returns an account object
func GetAccount(accountDir, accountConfig string) (*wallet.Account, error) {
	// resolve secrets manager instance and allow usage of insecure local secrets manager
	secretsManager, err := polybftsecrets.GetSecretsManager(accountDir, accountConfig, true)
	if err != nil {
		return nil, err
	}

	return wallet.NewAccountFromSecret(secretsManager)
}

// GetAccountFromDir returns an account object from local secrets manager
func GetAccountFromDir(accountDir string) (*wallet.Account, error) {
	return GetAccount(accountDir, "")
}

// GetValidatorInfo queries CustomSupernetManager, StakeManager and RewardPool smart contracts
// to retrieve validator info for given address
func GetValidatorInfo(validatorAddr types.Address, childRelayer txrelayer.TxRelayer) (*polybft.ValidatorInfo, error) {
	getValidatorMethod := contractsapi.StakeManager.Abi.GetMethod("getValidator")

	encode, err := getValidatorMethod.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err := childRelayer.Call(contracts.SystemCaller, contracts.StakeManagerContract, encode)
	if err != nil {
		return nil, err
	}

	byteResponse, err := hex.DecodeHex(response)
	if err != nil {
		return nil, fmt.Errorf("unable to decode hex response, %w", err)
	}

	decoded, err := getValidatorMethod.Outputs.Decode(byteResponse)
	if err != nil {
		return nil, err
	}

	decodedOutputsMap, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs to map")
	}

	innerMap, ok := decodedOutputsMap["0"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert decoded outputs map to inner map")
	}

	//nolint:forcetypeassert
	validatorInfo := &polybft.ValidatorInfo{
		Address:       validatorAddr,
		IsActive:      innerMap["isActive"].(bool),
		IsWhitelisted: innerMap["isWhitelisted"].(bool),
	}

	stakeOfFn := &contractsapi.StakeOfStakeManagerFn{
		Validator: types.Address(validatorAddr),
	}

	encode, err = stakeOfFn.EncodeAbi()
	if err != nil {
		return nil, err
	}

	response, err = childRelayer.Call(contracts.SystemCaller, contracts.StakeManagerContract, encode)
	if err != nil {
		return nil, err
	}

	stake, err := common.ParseUint256orHex(&response)
	if err != nil {
		return nil, err
	}

	validatorInfo.Stake = stake

	withdrawableFn := contractsapi.EpochManager.Abi.GetMethod("pendingRewards")

	encode, err = withdrawableFn.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err = childRelayer.Call(types.ZeroAddress, contracts.EpochManagerContract, encode)
	if err != nil {
		return nil, err
	}

	withdrawableRewards, err := common.ParseUint256orHex(&response)
	if err != nil {
		return nil, err
	}

	validatorInfo.WithdrawableRewards = withdrawableRewards

	return validatorInfo, nil
}
