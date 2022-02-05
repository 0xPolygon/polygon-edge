package init

import (
	"crypto/ecdsa"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var (
	params = &initParams{}
)

type initParams struct {
	dataDir    string
	configPath string

	secretsManager secrets.SecretsManager
	secretsConfig  *secrets.SecretsManagerConfig

	validatorPrivateKey  *ecdsa.PrivateKey
	networkingPrivateKey libp2pCrypto.PrivKey

	nodeID peer.ID
}

func (ip *initParams) areValidParams() bool {
	return ip.dataDir != "" || ip.configPath != ""
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

	vault, err := helper.SetupHashicorpVault(ip.secretsConfig)
	if err != nil {
		return err
	}

	ip.secretsManager = vault

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
	validatorKey, err := helper.InitValidatorKey(ip.secretsManager)
	if err != nil {
		return err
	}

	ip.validatorPrivateKey = validatorKey

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

func (ip *initParams) getResult() output.CommandResult {
	return &SecretsInitResult{
		Address: crypto.PubKeyToAddress(&ip.validatorPrivateKey.PublicKey),
		NodeID:  ip.nodeID.String(),
	}
}
