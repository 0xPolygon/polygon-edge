package sidechain

import (
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

const (
	AmountFlag = "amount"

	DefaultGasPrice = 1879048192 // 0x70000000
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

// GetValidatorInfo queries ChildValidatorSet smart contract and retrieves validator info for given address
func GetValidatorInfo(supernetManager, validatorAddr ethgo.Address,
	rootRelayer, childRelayer txrelayer.TxRelayer) (*polybft.ValidatorInfo, error) {
	getValidatorMethod := contractsapi.CustomSupernetManager.Abi.GetMethod("getValidator")

	encode, err := getValidatorMethod.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err := rootRelayer.Call(ethgo.ZeroAddress, supernetManager, encode)
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

	withdrawableFn := contractsapi.ValidatorSet.Abi.GetMethod("withdrawable")

	encode, err = withdrawableFn.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err = childRelayer.Call(ethgo.ZeroAddress, ethgo.Address(contracts.ValidatorSetContract), encode)
	if err != nil {
		return nil, err
	}

	withdrawableRewards, err := types.ParseUint256orHex(&response)
	if err != nil {
		return nil, err
	}

	//nolint:forcetypeassert
	vInfo := &polybft.ValidatorInfo{
		Address:             validatorAddr.Address(),
		Stake:               decodedOutputsMap["stake"].(*big.Int),
		WithdrawableRewards: withdrawableRewards,
		IsActive:            decodedOutputsMap["isActive"].(bool),
		IsWhitelisted:       decodedOutputsMap["isWhitelisted"].(bool),
	}

	return vInfo, nil
}
