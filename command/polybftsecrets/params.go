package polybftsecrets

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

const (
	dataPathFlag   = "data-dir"
	configFlag     = "config"
	accountFlag    = "account"
	privateKeyFlag = "private"
	networkFlag    = "network"
	numFlag        = "num"

	// maxInitNum is the maximum value for "num" flag
	maxInitNum = 30

	insecureLocalStore = "insecure"
)

var (
	errInvalidNum                     = fmt.Errorf("num flag value should be between 1 and %d", maxInitNum)
	errInvalidConfig                  = errors.New("invalid secrets configuration")
	errInvalidParams                  = errors.New("no config file or data directory passed in")
	errUnsupportedType                = errors.New("unsupported secrets manager")
	errSecureLocalStoreNotImplemented = errors.New(
		"use a secrets backend, or supply an --insecure flag " +
			"to store the private keys locally on the filesystem, " +
			"avoid doing so in production")
)

type initParams struct {
	dataPath   string
	configPath string

	generatesAccount bool
	generatesNetwork bool

	printPrivateKey bool

	numberOfSecrets int

	insecureLocalStore bool
}

func (ip *initParams) validateFlags() error {
	if ip.numberOfSecrets < 1 || ip.numberOfSecrets > maxInitNum {
		return errInvalidNum
	}

	if ip.dataPath == "" && ip.configPath == "" {
		return errInvalidParams
	}

	return nil
}

func (ip *initParams) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&ip.dataPath,
		dataPathFlag,
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&ip.configPath,
		configFlag,
		"",
		"the path to the SecretsManager config file, "+
			"if omitted, the local FS secrets manager is used",
	)

	cmd.Flags().IntVar(
		&ip.numberOfSecrets,
		numFlag,
		1,
		"the flag indicating how many secrets should be created, only for the local FS",
	)

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(dataPathFlag, configFlag)

	// num flag should be used with data-dir flag only so it should not be used with config flag.
	cmd.MarkFlagsMutuallyExclusive(numFlag, configFlag)

	cmd.Flags().BoolVar(
		&ip.generatesAccount,
		accountFlag,
		true,
		"the flag indicating whether new account is created",
	)

	cmd.Flags().BoolVar(
		&ip.generatesNetwork,
		networkFlag,
		true,
		"the flag indicating whether new Network key is created",
	)

	cmd.Flags().BoolVar(
		&ip.printPrivateKey,
		privateKeyFlag,
		false,
		"the flag indicating whether Private key is printed",
	)

	cmd.Flags().BoolVar(
		&ip.insecureLocalStore,
		insecureLocalStore,
		false,
		"the flag indicating should the secrets stored on the local storage be encrypted",
	)
}

func (ip *initParams) Execute() (Results, error) {
	results := make(Results, ip.numberOfSecrets)

	for i := 0; i < ip.numberOfSecrets; i++ {
		configDir, dataDir := ip.configPath, ip.dataPath

		if ip.numberOfSecrets > 1 {
			dataDir = fmt.Sprintf("%s%d", ip.dataPath, i+1)
		}

		if configDir != "" && ip.numberOfSecrets > 1 {
			configDir = fmt.Sprintf("%s%d", ip.configPath, i+1)
		}

		secretManager, err := getSecretsManager(dataDir, configDir, ip.insecureLocalStore)
		if err != nil {
			return results, err
		}

		err = ip.initKeys(secretManager)
		if err != nil {
			return results, err
		}

		res, err := ip.getResult(secretManager)
		if err != nil {
			return results, err
		}

		results[i] = res
	}

	return results, nil
}

func (ip *initParams) initKeys(secretsManager secrets.SecretsManager) error {
	if ip.generatesNetwork {
		if _, err := helper.InitNetworkingPrivateKey(secretsManager); err != nil {
			return err
		}
	}

	if ip.generatesAccount {
		if secretsManager.HasSecret(secrets.ValidatorKey) {
			return fmt.Errorf("secrets '%s' has been already initialized", secrets.ValidatorKey)
		}

		if secretsManager.HasSecret(secrets.ValidatorBLSKey) {
			return fmt.Errorf("secrets '%s' has been already initialized", secrets.ValidatorBLSKey)
		}

		return wallet.GenerateAccount().Save(secretsManager)
	}

	return nil
}

// getResult gets keys from secret manager and return result to display
func (ip *initParams) getResult(secretsManager secrets.SecretsManager) (command.CommandResult, error) {
	var (
		res = &SecretsInitResult{}
		err error
	)

	if ip.generatesAccount {
		account, err := wallet.NewAccountFromSecret(secretsManager)
		if err != nil {
			return nil, err
		}

		res.Address = types.Address(account.Ecdsa.Address())
		res.BLSPubkey = hex.EncodeToString(account.Bls.PublicKey().Marshal())

		if ip.printPrivateKey {
			pk, err := account.Ecdsa.MarshallPrivateKey()
			if err != nil {
				return nil, err
			}

			res.PrivateKey = hex.EncodeToString(pk)
		}
	}

	if ip.generatesNetwork {
		if res.NodeID, err = helper.LoadNodeID(secretsManager); err != nil {
			return nil, err
		}
	}

	res.Insecure = ip.insecureLocalStore

	return res, nil
}

func getSecretsManager(dataPath, configPath string, insecureLocalStore bool) (secrets.SecretsManager, error) {
	if configPath != "" {
		secretsConfig, readErr := secrets.ReadConfig(configPath)
		if readErr != nil {
			return nil, errInvalidConfig
		}

		if !secrets.SupportedServiceManager(secretsConfig.Type) {
			return nil, errUnsupportedType
		}

		return helper.InitCloudSecretsManager(secretsConfig)
	}

	//Storing secrets on a local file system should only be allowed with --insecure flag,
	//to raise awareness that it should be only used in development/testing environments.
	//Production setups should use one of the supported secrets managers
	if !insecureLocalStore {
		return nil, errSecureLocalStoreNotImplemented
	}

	return helper.SetupLocalSecretsManager(dataPath)
}
