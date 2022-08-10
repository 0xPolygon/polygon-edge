package init

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
)

const (
	dataDirFlag = "data-dir"
	configFlag  = "config"
	ecdsaFlag   = "ecdsa"
	blsFlag     = "bls"
	networkFlag = "network"
)

var (
	params = &initParams{}
)

var (
	errInvalidConfig   = errors.New("invalid secrets configuration")
	errInvalidParams   = errors.New("no config file or data directory passed in")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

type initParams struct {
	dataDir          string
	configPath       string
	generatesECDSA   bool
	generatesBLS     bool
	generatesNetwork bool

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig
}

func (ip *initParams) validateFlags() error {
	if ip.dataDir == "" && ip.configPath == "" {
		return errInvalidParams
	}

	return nil
}

func (ip *initParams) initSecrets() error {
	if err := ip.initSecretsManager(); err != nil {
		return err
	}

	if err := ip.initValidatorKey(); err != nil {
		return err
	}

	return ip.initNetworkingKey()
}

func (ip *initParams) initSecretsManager() error {
	if ip.hasConfigPath() {
		return ip.initFromConfig()
	}

	return ip.initLocalSecretsManager()
}

func (ip *initParams) hasConfigPath() bool {
	return ip.configPath != ""
}

func (ip *initParams) initFromConfig() error {
	if err := ip.parseConfig(); err != nil {
		return err
	}

	var secretsManager secrets.SecretsManager

	switch ip.secretsConfig.Type {
	case secrets.HashicorpVault:
		vault, err := helper.SetupHashicorpVault(ip.secretsConfig)
		if err != nil {
			return err
		}

		secretsManager = vault
	case secrets.AWSSSM:
		AWSSSM, err := helper.SetupAWSSSM(ip.secretsConfig)
		if err != nil {
			return err
		}

		secretsManager = AWSSSM
	case secrets.GCPSSM:
		GCPSSM, err := helper.SetupGCPSSM(ip.secretsConfig)
		if err != nil {
			return err
		}

		secretsManager = GCPSSM
	default:
		return errUnsupportedType
	}

	ip.secretsManager = secretsManager

	return nil
}

func (ip *initParams) parseConfig() error {
	secretsConfig, readErr := secrets.ReadConfig(ip.configPath)
	if readErr != nil {
		return errInvalidConfig
	}

	if !secrets.SupportedServiceManager(secretsConfig.Type) {
		return errUnsupportedType
	}

	ip.secretsConfig = secretsConfig

	return nil
}

func (ip *initParams) initLocalSecretsManager() error {
	local, err := helper.SetupLocalSecretsManager(ip.dataDir)
	if err != nil {
		return err
	}

	ip.secretsManager = local

	return nil
}

func (ip *initParams) initValidatorKey() error {
	var err error

	if ip.generatesECDSA {
		if _, err = helper.InitECDSAValidatorKey(ip.secretsManager); err != nil {
			return err
		}
	}

	if ip.generatesBLS {
		if _, err = helper.InitBLSValidatorKey(ip.secretsManager); err != nil {
			return err
		}
	}

	return nil
}

func (ip *initParams) initNetworkingKey() error {
	if ip.generatesNetwork {
		if _, err := helper.InitNetworkingPrivateKey(ip.secretsManager); err != nil {
			return err
		}
	}

	return nil
}

// getResult gets keys from secret manager and return result to display
func (ip *initParams) getResult() (command.CommandResult, error) {
	var (
		res = &SecretsInitResult{}
		err error
	)

	if res.Address, err = loadValidatorAddress(ip.secretsManager); err != nil {
		return nil, err
	}

	if res.BLSPubkey, err = loadBLSPublicKey(ip.secretsManager); err != nil {
		return nil, err
	}

	if res.NodeID, err = loadNodeID(ip.secretsManager); err != nil {
		return nil, err
	}

	return res, nil
}
