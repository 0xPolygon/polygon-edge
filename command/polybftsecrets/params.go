package polybftsecrets

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

const (
	accountFlag            = "account"
	privateKeyFlag         = "private"
	insecureLocalStoreFlag = "insecure"
	networkFlag            = "network"
	numFlag                = "num"
	outputFlag             = "output"

	// maxInitNum is the maximum value for "num" flag
	maxInitNum = 30
)

type initParams struct {
	accountDir    string
	accountConfig string

	generatesAccount bool
	generatesNetwork bool

	printPrivateKey bool

	numberOfSecrets int

	insecureLocalStore bool

	output bool
}

func (ip *initParams) validateFlags() error {
	if ip.numberOfSecrets < 1 || ip.numberOfSecrets > maxInitNum {
		return ErrInvalidNum
	}

	if ip.accountDir == "" && ip.accountConfig == "" {
		return ErrInvalidParams
	}

	return nil
}

func (ip *initParams) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&ip.accountDir,
		AccountDirFlag,
		"",
		AccountDirFlagDesc,
	)

	cmd.Flags().StringVar(
		&ip.accountConfig,
		AccountConfigFlag,
		"",
		AccountConfigFlagDesc,
	)

	cmd.Flags().IntVar(
		&ip.numberOfSecrets,
		numFlag,
		1,
		"the flag indicating how many secrets should be created, only for the local FS",
	)

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(AccountDirFlag, AccountConfigFlag)

	// num flag should be used with data-dir flag only so it should not be used with config flag.
	cmd.MarkFlagsMutuallyExclusive(numFlag, AccountConfigFlag)

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
		insecureLocalStoreFlag,
		false,
		"the flag indicating should the secrets stored on the local storage be encrypted",
	)

	cmd.Flags().BoolVar(
		&ip.output,
		outputFlag,
		false,
		"the flag indicating to output existing secrets",
	)
}

func (ip *initParams) Execute() (Results, error) {
	results := make(Results, ip.numberOfSecrets)

	for i := 0; i < ip.numberOfSecrets; i++ {
		configDir, dataDir := ip.accountConfig, ip.accountDir

		if ip.numberOfSecrets > 1 {
			dataDir = fmt.Sprintf("%s%d", ip.accountDir, i+1)
		}

		if configDir != "" && ip.numberOfSecrets > 1 {
			configDir = fmt.Sprintf("%s%d", ip.accountConfig, i+1)
		}

		secretManager, err := GetSecretsManager(dataDir, configDir, ip.insecureLocalStore)
		if err != nil {
			return results, err
		}

		var gen []string
		if !ip.output {
			gen, err = ip.initKeys(secretManager)
			if err != nil {
				return results, err
			}
		}

		res, err := ip.getResult(secretManager, gen)
		if err != nil {
			return results, err
		}

		results[i] = res
	}

	return results, nil
}

func (ip *initParams) initKeys(secretsManager secrets.SecretsManager) ([]string, error) {
	var generated []string

	if ip.generatesNetwork {
		if !secretsManager.HasSecret(secrets.NetworkKey) {
			if _, err := helper.InitNetworkingPrivateKey(secretsManager); err != nil {
				return generated, fmt.Errorf("error initializing network-key: %w", err)
			}

			generated = append(generated, secrets.NetworkKey)
		}
	}

	if ip.generatesAccount {
		var (
			a   *wallet.Account
			err error
		)

		if !secretsManager.HasSecret(secrets.ValidatorKey) && !secretsManager.HasSecret(secrets.ValidatorBLSKey) {
			a, err = wallet.GenerateAccount()
			if err != nil {
				return generated, fmt.Errorf("error generating account: %w", err)
			}

			if err = a.Save(secretsManager); err != nil {
				return generated, fmt.Errorf("error saving account: %w", err)
			}

			generated = append(generated, secrets.ValidatorKey, secrets.ValidatorBLSKey)
		}
	}

	return generated, nil
}

// getResult gets keys from secret manager and return result to display
func (ip *initParams) getResult(
	secretsManager secrets.SecretsManager,
	generated []string,
) (command.CommandResult, error) {
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

		res.Generated = strings.Join(generated, ", ")

		if ip.printPrivateKey {
			pk, err := account.Ecdsa.MarshallPrivateKey()
			if err != nil {
				return nil, err
			}

			res.PrivateKey = hex.EncodeToString(pk)

			blspk, err := account.Bls.Marshal()
			if err != nil {
				return nil, err
			}

			res.BLSPrivateKey = string(blspk)
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
