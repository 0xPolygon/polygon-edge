package init

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	dataDirFlag = "data-dir"
	configFlag  = "config"
	keyTypeFlag = "key-type"
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
	dataDir    string
	configPath string
	rawKeyType string
	keyType    crypto.KeyType

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig

	validatorAddress     []byte // ECDSA: Address, BLS: PubKey
	networkingPrivateKey libp2pCrypto.PrivKey

	nodeID peer.ID
}

func (ip *initParams) validateFlags() error {
	if ip.dataDir == "" && ip.configPath == "" {
		return errInvalidParams
	}
	if ip.rawKeyType == "" {
		return errInvalidParams
	}

	return nil
}

func (ip *initParams) initSecrets() error {
	if err := ip.initKeyType(); err != nil {
		return err
	}

	if err := ip.initSecretsManager(); err != nil {
		return err
	}

	if err := ip.initValidatorKey(); err != nil {
		return err
	}

	return ip.initNetworkingKey()
}

func (ip *initParams) initKeyType() error {
	key, err := crypto.ToKeyType(ip.rawKeyType)
	if err != nil {
		return err
	}

	ip.keyType = key

	return nil
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
	address, err := helper.InitValidatorKey(ip.secretsManager, ip.keyType)
	if err != nil {
		return err
	}

	ip.validatorAddress = address

	return nil
}

func (ip *initParams) initNetworkingKey() error {
	networkingKey, err := helper.InitNetworkingPrivateKey(ip.secretsManager)
	if err != nil {
		return err
	}

	ip.networkingPrivateKey = networkingKey

	return ip.initNodeID()
}

func (ip *initParams) initNodeID() error {
	nodeID, err := peer.IDFromPrivateKey(ip.networkingPrivateKey)
	if err != nil {
		return err
	}

	ip.nodeID = nodeID

	return nil
}

func (ip *initParams) getResult() command.CommandResult {
	return &SecretsInitResult{
		Address: ip.validatorAddress,
		NodeID:  ip.nodeID.String(),
	}
}
