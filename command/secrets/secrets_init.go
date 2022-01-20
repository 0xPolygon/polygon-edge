package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/hashicorpvault"
	"github.com/0xPolygon/polygon-edge/secrets/local"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
)

// SecretsInit is the command to query the snapshot
type SecretsInit struct {
	helper.Base
	Formatter *helper.FormatterFlag
}

var (
	ErrInvalidSecretsConfig = errors.New("invalid secrets configuration file")
)

func (p *SecretsInit) DefineFlags() {
	p.Base.DefineFlags(p.Formatter)

	p.FlagMap["data-dir"] = helper.FlagDescriptor{
		Description: "Sets the directory for the Polygon Edge data if the local FS is used",
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	p.FlagMap["config"] = helper.FlagDescriptor{
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
	return "Initializes private keys for the Polygon Edge (Validator + Networking) to the specified Secrets Manager"
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

// generateAlreadyInitializedError generates an output for when the secrets directory
// has already been initialized in the past
func generateAlreadyInitializedError(directory string) string {
	return fmt.Sprintf("Directory %s has previously initialized secrets data", directory)
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
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter)

	var dataDir string

	var configPath string

	flags.StringVar(&dataDir, "data-dir", "", "")
	flags.StringVar(&configPath, "config", "", "")

	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	if dataDir == "" && configPath == "" {
		p.Formatter.OutputError(errors.New("required argument (data directory) not passed in"))

		return 1
	}

	var secretsManager secrets.SecretsManager

	if configPath == "" {
		// No secrets manager config specified,
		// use the local secrets manager
		localSecretsManager, setupErr := setupLocalSM(dataDir)
		if setupErr != nil {
			p.Formatter.OutputError(setupErr)

			return 1
		}

		secretsManager = localSecretsManager
	} else {
		// Config file passed in
		secretsConfig, readErr := secrets.ReadConfig(configPath)
		if readErr != nil {
			p.Formatter.OutputError(fmt.Errorf("unable to read config file, %w", readErr))

			return 1
		}

		// Set up the corresponding secrets manager
		switch secretsConfig.Type {
		case secrets.HashicorpVault:
			vaultSecretsManager, setupErr := setupHashicorpVault(secretsConfig)
			if setupErr != nil {
				p.Formatter.OutputError(setupErr)

				return 1
			}

			secretsManager = vaultSecretsManager
		default:
			p.Formatter.OutputError(errors.New("unknown secrets manager type"))

			return 1
		}
	}

	// Generate the IBFT validator private key
	validatorKey, validatorKeyEncoded, keyErr := crypto.GenerateAndEncodePrivateKey()
	if keyErr != nil {
		p.Formatter.OutputError(keyErr)

		return 1
	}

	// Write the validator private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(secrets.ValidatorKey, validatorKeyEncoded); setErr != nil {
		p.Formatter.OutputError(setErr)

		return 1
	}

	// Generate the libp2p private key
	libp2pKey, libp2pKeyEncoded, keyErr := network.GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		p.Formatter.OutputError(keyErr)

		return 1
	}

	// Write the networking private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
		p.Formatter.OutputError(setErr)

		return 1
	}

	nodeID, err := peer.IDFromPrivateKey(libp2pKey)
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	res := &SecretsInitResult{
		Address: crypto.PubKeyToAddress(&validatorKey.PublicKey),
		NodeID:  nodeID.String(),
	}
	p.Formatter.OutputResult(res)

	return 0
}

type SecretsInitResult struct {
	Address types.Address `json:"address"`
	NodeID  string        `json:"node_id"`
}

func (r *SecretsInitResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[SECRETS INIT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", r.Address),
		fmt.Sprintf("Node ID|%s", r.NodeID),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
