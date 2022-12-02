package fund

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	dataDirFlag = "data-dir"
	configFlag  = "config"
	numFlag     = "num"
	jsonRPCFlag = "json-rpc"
)

var (
	errInvalidConfig   = errors.New("invalid secrets configuration")
	errInvalidParams   = errors.New("no config file or data directory passed in")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

type fundParams struct {
	dataDir    string
	configPath string

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig
}

func (fp *fundParams) validateFlags() error {
	if fp.dataDir == "" && fp.configPath == "" {
		return errInvalidParams
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
