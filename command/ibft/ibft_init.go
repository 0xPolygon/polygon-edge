package ibft

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/secrets/local"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/server"
)

// IbftInit is the command to query the snapshot
type IbftInit struct {
	helper.Meta
}

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
	output := "\n[IBFT INIT ERROR]\n"
	output += fmt.Sprintf("Directory %s has previously initialized IBFT data\n", directory)
	return output
}

var (
	consensusDir = "consensus"
	libp2pDir    = "libp2p"
)

// Run implements the cli.IbftInit interface
func (p *IbftInit) Run(args []string) int {
	flags := flag.NewFlagSet(p.GetBaseCommand(), flag.ContinueOnError)
	var dataDir string
	flags.StringVar(&dataDir, "data-dir", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if dataDir == "" {
		p.UI.Error("required argument (data directory) not passed in")
		return 1
	}

	// Check if the sub-directories exist / are already populated
	for _, subDirectory := range []string{consensusDir, libp2pDir} {
		if helper.DirectoryExists(filepath.Join(dataDir, subDirectory)) {
			p.UI.Error(generateAlreadyInitializedError(dataDir))
			return 1
		}
	}

	// TODO branch for the type of secrets manager

	// Local (FS) SecretsManager
	if err := server.SetupDataDir(dataDir, []string{consensusDir, libp2pDir}); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	localSecretsManager, factoryErr := local.SecretsManagerFactory(&secrets.SecretsManagerParams{
		Logger: nil,
		Params: map[string]interface{}{
			secrets.Path: dataDir,
		},
	})
	if factoryErr != nil {
		p.UI.Error(factoryErr.Error())
		return 1
	}

	// Generate the IBFT validator private key
	validatorKey, validatorKeyEncoded, keyErr := crypto.GenerateAndEncodePrivateKey()
	if keyErr != nil {
		p.UI.Error(keyErr.Error())
		return 1
	}

	// Write the validator private key to disk
	if setErr := localSecretsManager.SetSecret(secrets.ValidatorKey, validatorKeyEncoded); setErr != nil {
		p.UI.Error(setErr.Error())
		return 1
	}

	// Generate the libp2p private key
	libp2pKey, libp2pKeyEncoded, keyErr := network.GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		p.UI.Error(keyErr.Error())
		return 1
	}

	// Write the networking private key to disk
	if setErr := localSecretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
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
