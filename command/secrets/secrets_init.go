package secrets

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/secrets/hashicorpvault"
	"github.com/0xPolygon/polygon-sdk/secrets/local"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
)

// SecretsInit is the command to query the snapshot
type SecretsInit struct {
	helper.Meta
}

var (
	ErrorInvalidSecretsConfig = errors.New("invalid secrets configuration file")
)

func (i *SecretsInit) DefineFlags() {
	if i.FlagMap == nil {
		// Flag map not initialized
		i.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	i.FlagMap["data-dir"] = helper.FlagDescriptor{
		Description: "Sets the directory for the Polygon SDK data if the local FS is used",
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	i.FlagMap["config"] = helper.FlagDescriptor{
		Description: "Sets the path to the SecretsManager config file. Used for Hashicorp Vault. " +
			"If omitted, the local FS secrets manager is used",
		Arguments: []string{
			"SECRETS_CONFIG",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (p *SecretsInit) GetHelperText() string {
	return "Initializes private keys for the Polygon SDK (Validator + Networking) to the specified Secrets Manager"
}

// Help implements the cli.SecretsInit interface
func (p *SecretsInit) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.SecretsInit interface
func (p *SecretsInit) Synopsis() string {
	return p.GetHelperText()
}

func (p *SecretsInit) GetBaseCommand() string {
	return "secrets init"
}

// constructInitError is a wrapper function for the error output to CLI
func constructInitError(err string) string {
	output := "\n[SECRETS INIT ERROR]\n"
	output += err

	return output
}

// generateAlreadyInitializedError generates an output for when the secrets directory
// has already been initialized in the past
func generateAlreadyInitializedError(directory string) string {
	return fmt.Sprintf("Directory %s has previously initialized secrets data\n", directory)
}

// setupLocalSM is a helper method for boilerplate local secrets manager setup
func setupLocalSM(dataDir string) (secrets.SecretsManager, error) {
	subDirectories := []string{secrets.ConsensusFolderLocal, secrets.NetworkFolderLocal}

	// Check if the sub-directories exist / are already populated
	for _, subDirectory := range subDirectories {
		if common.DirectoryExists(filepath.Join(dataDir, subDirectory)) {
			return nil, errors.New(generateAlreadyInitializedError(dataDir))
		}
	}

	return local.SecretsManagerFactory(
		nil, // Local secrets manager doesn't require a config
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
			Extra: map[string]interface{}{
				secrets.Path: dataDir,
			},
		})
}

// setupHashicorpVault is a helper method for boilerplate hashicorp vault secrets manager setup
func setupHashicorpVault(
	secretsConfig *secrets.SecretsManagerConfig,
) (secrets.SecretsManager, error) {
	return hashicorpvault.SecretsManagerFactory(
		secretsConfig,
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
		},
	)
}

// Run implements the cli.SecretsInit interface
func (p *SecretsInit) Run(args []string) int {
	flags := flag.NewFlagSet(p.GetBaseCommand(), flag.ContinueOnError)
	var dataDir string
	var configPath string

	flags.StringVar(&dataDir, "data-dir", "", "")
	flags.StringVar(&configPath, "config", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if dataDir == "" && configPath == "" {
		p.UI.Error("required argument (data directory) not passed in")
		return 1
	}

	var secretsManager secrets.SecretsManager
	if configPath == "" {
		// No secrets manager config specified,
		// use the local secrets manager
		localSecretsManager, setupErr := setupLocalSM(dataDir)
		if setupErr != nil {
			p.UI.Error(constructInitError(setupErr.Error()))
			return 1
		}

		secretsManager = localSecretsManager
	} else {
		// Config file passed in
		secretsConfig, readErr := secrets.ReadConfig(configPath)
		if readErr != nil {
			p.UI.Error(constructInitError(fmt.Sprintf("Unable to read config file, %v", readErr)))
			return 1
		}

		// Set up the corresponding secrets manager
		switch secretsConfig.Type {
		case secrets.HashicorpVault:
			vaultSecretsManager, setupErr := setupHashicorpVault(secretsConfig)
			if setupErr != nil {
				p.UI.Error(constructInitError(setupErr.Error()))
				return 1
			}

			secretsManager = vaultSecretsManager
		default:
			p.UI.Error(constructInitError("Unknown secrets manager type"))
			return 1
		}
	}

	// Generate the IBFT validator private key
	validatorKey, validatorKeyEncoded, keyErr := crypto.GenerateAndEncodePrivateKey()
	if keyErr != nil {
		p.UI.Error(keyErr.Error())
		return 1
	}

	// Write the validator private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(secrets.ValidatorKey, validatorKeyEncoded); setErr != nil {
		p.UI.Error(setErr.Error())
		return 1
	}

	// Generate the libp2p private key
	libp2pKey, libp2pKeyEncoded, keyErr := network.GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		p.UI.Error(keyErr.Error())
		return 1
	}

	// Write the networking private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
		p.UI.Error(setErr.Error())
		return 1
	}

	nodeId, err := peer.IDFromPrivateKey(libp2pKey)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	output := "\n[SECRETS INIT]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", crypto.PubKeyToAddress(&validatorKey.PublicKey)),
		fmt.Sprintf("Node ID|%s", nodeId.String()),
	})

	output += "\n"

	p.UI.Output(output)

	return 0
}
