package sidechain

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	secretsHelper "github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/umbracle/ethgo"
)

const (
	AccountDirFlag = "account"
	SelfFlag       = "self"
	AmountFlag     = "amount"

	DefaultGasPrice = 1879048192 // 0x70000000
)

func CheckIfDirectoryExist(dir string) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided directory '%s' doesn't exist", dir)
	}

	return nil
}

func GetAccountFromDir(dir string) (*wallet.Account, error) {
	secretsManager, err := secretsHelper.SetupLocalSecretsManager(dir)
	if err != nil {
		return nil, err
	}

	return wallet.NewAccountFromSecret(secretsManager)
}

// GetValidatorInfo queries ChildValidatorSet smart contract and retrieves validator info for given address
func GetValidatorInfo(validatorAddr ethgo.Address, txRelayer txrelayer.TxRelayer) (map[string]interface{}, error) {
	getValidatorMethod := polybft.StateFunctionsABI.GetMethod("getValidator")

	encode, err := getValidatorMethod.Encode([]interface{}{validatorAddr})
	if err != nil {
		return nil, err
	}

	response, err := txRelayer.Call(ethgo.Address(contracts.SystemCaller),
		ethgo.Address(contracts.ValidatorSetContract), encode)
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

	decodedValidatorInfoMap, ok := decodedOutputsMap["0"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert validator info result to a map")
	}

	return decodedValidatorInfoMap, nil
}
