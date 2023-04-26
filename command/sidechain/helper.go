package sidechain

import (
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/txrelayer"
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
// TODO - @goran-ethernal depricate this function once we change e2e tests
//
//nolint:godox
func GetValidatorInfo(validatorAddr ethgo.Address, txRelayer txrelayer.TxRelayer) (*polybft.ValidatorInfo, error) {
	return nil, nil
}

// GetDelegatorReward queries delegator reward for given validator and delegator addresses
// TODO - @goran-ethernal depricate this function once we change e2e tests
//
//nolint:godox
func GetDelegatorReward(validatorAddr ethgo.Address, delegatorAddr ethgo.Address,
	txRelayer txrelayer.TxRelayer) (*big.Int, error) {
	return nil, nil
}
