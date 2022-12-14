package sidechain

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	secretsHelper "github.com/0xPolygon/polygon-edge/secrets/helper"
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
