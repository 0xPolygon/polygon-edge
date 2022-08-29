package print

import (
	"crypto/ecdsa"
	"errors"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	dataDirFlag   = "data-dir"
	configFlag    = "config"
	validatorFlag = "validator"
	nodeIDFlag    = "node-id"
)

var (
	params = &printParams{}
)

var (
	errInvalidConfig   = errors.New("invalid secrets configuration")
	errInvalidParams   = errors.New("no config file or data directory passed in")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

type printParams struct {
	dataDir    string
	configPath string

	printNodeID    bool
	printValidator bool

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig

	validatorPrivateKey  *ecdsa.PrivateKey
	networkingPrivateKey libp2pCrypto.PrivKey

	nodeID peer.ID
}

func (ip *printParams) validateFlags() error {
	if ip.dataDir == "" && ip.configPath == "" {
		return errInvalidParams
	}

	return nil
}

func (ip *printParams) printSecrets() error {
	if err := ip.initSecretsManager(); err != nil {
		return err
	}

	if err := ip.getValidatorKey(); err != nil {
		return err
	}

	return ip.getNetworkingKey()
}

func (ip *printParams) initSecretsManager() error {
	if ip.hasConfigPath() {
		return ip.initFromConfig()
	}

	return ip.initLocalSecretsManager()
}

func (ip *printParams) hasConfigPath() bool {
	return ip.configPath != ""
}

func (ip *printParams) initFromConfig() error {
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

func (ip *printParams) parseConfig() error {
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

func (ip *printParams) initLocalSecretsManager() error {
	local, err := helper.SetupLocalSecretsManager(ip.dataDir)
	if err != nil {
		return err
	}

	ip.secretsManager = local

	return nil
}

func (ip *printParams) getValidatorKey() error {
	validatorKey, err := helper.GetValidatorKey(ip.secretsManager)
	if err != nil {
		return err
	}

	ip.validatorPrivateKey = validatorKey

	return nil
}

func (ip *printParams) getNetworkingKey() error {
	networkingKey, err := helper.GetNetworkingPrivateKey(ip.secretsManager)
	if err != nil {
		return err
	}

	ip.networkingPrivateKey = networkingKey

	return ip.initNodeID()
}

func (ip *printParams) initNodeID() error {
	nodeID, err := peer.IDFromPrivateKey(ip.networkingPrivateKey)
	if err != nil {
		return err
	}

	ip.nodeID = nodeID

	return nil
}

func (ip *printParams) getResult() command.CommandResult {
	return &SecretsPrintResult{
		Address: crypto.PubKeyToAddress(&ip.validatorPrivateKey.PublicKey),
		NodeID:  ip.nodeID.String(),

		PrintNodeID:    ip.printNodeID,
		PrintValidator: ip.printValidator,
	}
}
