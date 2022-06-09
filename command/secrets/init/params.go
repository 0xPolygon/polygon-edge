package init

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
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
	errEmptyKeyType    = errors.New("key type is not specified")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

type initParams struct {
	dataDir    string
	configPath string
	rawKeyType string
	keyType    crypto.KeyType

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig

	validatorAddress types.Address
	blsPublicKey     []byte

	networkingPrivateKey libp2pCrypto.PrivKey

	nodeID peer.ID
}

func (ip *initParams) validateFlags() error {
	if ip.dataDir == "" && ip.configPath == "" {
		return errInvalidParams
	}

	if ip.rawKeyType == "" {
		return errEmptyKeyType
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
	address, err := helper.InitValidatorKey(ip.secretsManager)
	if err != nil {
		return err
	}

	ip.validatorAddress = address

	if ip.keyType == crypto.KeyBLS {
		if ip.blsPublicKey, err = helper.GetBLSPubkeyFromValidatorKey(ip.secretsManager); err != nil {
			return err
		}
	}

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
		Address:   ip.validatorAddress,
		BLSPubkey: ip.blsPublicKey,
		NodeID:    ip.nodeID.String(),
	}
}
