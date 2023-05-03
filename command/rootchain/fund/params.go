package fund

import (
	"errors"
	"fmt"
	"math/big"
	"os"

	cmdhelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	amountFlag        = "amount"
	jsonRPCFlag       = "json-rpc"
	mintRootTokenFlag = "mint"
)

var (
	errInvalidConfig   = errors.New("invalid secrets configuration")
	errInvalidParams   = errors.New("no config file or data directory passed in")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

type fundParams struct {
	dataDir             string
	configPath          string
	amount              string
	nativeRootTokenAddr string
	deployerPrivateKey  string
	mintRootToken       bool
	jsonRPCAddress      string

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig

	amountValue *big.Int
}

func (fp *fundParams) validateFlags() (err error) {
	if fp.amountValue, err = cmdhelper.ParseAmount(fp.amount); err != nil {
		return err
	}

	if fp.dataDir == "" && fp.configPath == "" {
		return errInvalidParams
	}

	if fp.dataDir != "" {
		if _, err := os.Stat(fp.dataDir); err != nil {
			return fmt.Errorf("invalid validators secrets path ('%s') provided. Error: %w", fp.dataDir, err)
		}
	}

	if fp.configPath != "" {
		if _, err := os.Stat(fp.configPath); err != nil {
			return fmt.Errorf("invalid validators secrets config path ('%s') provided. Error: %w", fp.configPath, err)
		}
	}

	return nil
}

func (fp *fundParams) hasConfigPath() bool {
	return fp.configPath != ""
}

func (fp *fundParams) initSecretsManager() error {
	var err error
	if fp.hasConfigPath() {
		if err = fp.parseConfig(); err != nil {
			return err
		}

		fp.secretsManager, err = helper.InitCloudSecretsManager(fp.secretsConfig)

		return err
	}

	return fp.initLocalSecretsManager()
}

func (fp *fundParams) parseConfig() error {
	secretsConfig, readErr := secrets.ReadConfig(fp.configPath)
	if readErr != nil {
		return errInvalidConfig
	}

	if !secrets.SupportedServiceManager(secretsConfig.Type) {
		return errUnsupportedType
	}

	fp.secretsConfig = secretsConfig

	return nil
}

func (fp *fundParams) initLocalSecretsManager() error {
	local, err := helper.SetupLocalSecretsManager(fp.dataDir)
	if err != nil {
		return err
	}

	fp.secretsManager = local

	return nil
}

func (fp *fundParams) getValidatorAccount() (types.Address, error) {
	return helper.LoadValidatorAddress(fp.secretsManager)
}
