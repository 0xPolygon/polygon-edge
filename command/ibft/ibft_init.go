package ibft

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/0xPolygon/polygon-sdk/consensus/ibft"
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
	i.FlagMap["key-dir"] = helper.FlagDescriptor{
		Description: "Set the directory for the validator and libp2p keys",
		Arguments: []string{
			"KEY_DIRECTORY",
		},
		ArgumentsOptional: true,
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
	var keyDir string
	flags.StringVar(&keyDir, "key-dir", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if keyDir == "" {
		p.UI.Error("required argument (key directory) not passed in")
		return 1
	}

	// Check if the sub-directories exist / are already populated
	for _, subDirectory := range []string{consensusDir, libp2pDir} {
		if helper.DirectoryExists(filepath.Join(keyDir, subDirectory)) {
			p.UI.Error(generateAlreadyInitializedError(keyDir))
			return 1
		}
	}

	if err := server.SetupDir(keyDir, []string{consensusDir, libp2pDir}); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to write the ibft private key
	key, err := crypto.GenerateOrReadPrivateKey(filepath.Join(keyDir, consensusDir, ibft.IbftKeyName))
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to create also a libp2p address
	libp2pKey, err := network.ReadLibp2pKey(filepath.Join(keyDir, libp2pDir))
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	nodeId, err := peer.IDFromPrivateKey(libp2pKey)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	output := "\n[IBFT INIT]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", crypto.PubKeyToAddress(&key.PublicKey)),
		fmt.Sprintf("Node ID|%s", nodeId.String()),
	})

	output += "\n"

	p.UI.Output(output)

	return 0
}
