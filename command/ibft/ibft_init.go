package ibft

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/secrets/hashicorpvault"
	"github.com/0xPolygon/polygon-sdk/secrets/local"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/network"
)

// IbftInit is the command to query the snapshot
type IbftInit struct {
	helper.Meta
}

var (
	ErrorInvalidSecretsConfig = errors.New("invalid secrets configuration file")
)

func (i *IbftInit) DefineFlags() {
	if i.FlagMap == nil {
		// Flag map not initialized
		i.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	i.FlagMap["data-dir"] = helper.FlagDescriptor{
		Description: "Sets the directory for the Polygon SDK data",
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	i.FlagMap["secrets-config"] = helper.FlagDescriptor{
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
func (p *IbftInit) GetHelperText() string {
	return "Initializes IBFT for the Polygon SDK, in the specified directory"
}

// Help implements the cli.IbftInit interface
func (p *IbftInit) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftInit interface
func (p *IbftInit) Synopsis() string {
	return p.GetHelperText()
}

func (p *IbftInit) GetBaseCommand() string {
	return "ibft init"
}

// generateAlreadyInitializedError generates an output for when the IBFT directory
// has already been initialized in the past
func generateAlreadyInitializedError(directory string) string {
	return fmt.Sprintf("Directory %s has previously initialized IBFT data\n", directory)
}

// constructInitError is a wrapper function for the error output to CLI
func constructInitError(err string) string {
	output := "\n[IBFT INIT ERROR]\n"
	output += err

	return output
}

var (
	consensusDir = "consensus"
	libp2pDir    = "libp2p"
)

// setupLocalSM is a helper method for boilerplate local secrets manager setup
func setupLocalSM(dataDir string) (secrets.SecretsManager, error) {
	// Check if the sub-directories exist / are already populated
	for _, subDirectory := range []string{consensusDir, libp2pDir} {
		if helper.DirectoryExists(filepath.Join(dataDir, subDirectory)) {
			return nil, errors.New(generateAlreadyInitializedError(dataDir))
		}
	}

	// Set up the local directories
	if err := common.SetupDataDir(dataDir, []string{consensusDir, libp2pDir}); err != nil {
		return nil, err
	}

	localSecretsManager, factoryErr := local.SecretsManagerFactory(&secrets.SecretsManagerParams{
		Logger: nil,
		Params: map[string]interface{}{
			secrets.Path: dataDir,
		},
	})

	return localSecretsManager, factoryErr
}

// setupHashicorpVault is a helper method for boilerplate hashicorp vault secrets manager setup
func setupHashicorpVault(
	secretsConfig *secrets.SecretsManagerConfig,
) (secrets.SecretsManager, error) {
	params := make(map[string]interface{})

	// Check if the token is present
	if secretsConfig.Token == "" {
		return nil, ErrorInvalidSecretsConfig
	}
	params[secrets.Token] = secretsConfig.Token

	// Check if the server URL is present
	if secretsConfig.ServerURL == "" {
		return nil, ErrorInvalidSecretsConfig
	}
	params[secrets.Server] = secretsConfig.ServerURL

	// Check if the node name is present
	if secretsConfig.Name == "" {
		return nil, ErrorInvalidSecretsConfig
	}
	params[secrets.Name] = secretsConfig.Name

	vaultSecretsManager, factoryErr := hashicorpvault.SecretsManagerFactory(
		&secrets.SecretsManagerParams{
			Logger: nil,
			Params: params,
		},
	)

	return vaultSecretsManager, factoryErr
}

// Run implements the cli.IbftInit interface
func (p *IbftInit) Run(args []string) int {
	flags := flag.NewFlagSet(p.GetBaseCommand(), flag.ContinueOnError)
	var dataDir string
	var configPath string

	flags.StringVar(&dataDir, "data-dir", "", "")
	flags.StringVar(&configPath, "secrets-config", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if dataDir == "" {
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

	output := "\n[IBFT INIT]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", crypto.PubKeyToAddress(&validatorKey.PublicKey)),
		fmt.Sprintf("Node ID|%s", nodeId.String()),
	})

	output += "\n"

	p.UI.Output(output)

	return 0
}
