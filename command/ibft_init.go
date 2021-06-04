package command

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"path/filepath"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/0xPolygon/minimal/network"
)

// IbftInit is the command to query the snapshot
type IbftInit struct {
	Meta
}

func (i *IbftInit) DefineFlags() {
	if i.flagMap == nil {
		// Flag map not initialized
		i.flagMap = make(map[string]FlagDescriptor)
	}

	i.flagMap["data-dir"] = FlagDescriptor{
		description: fmt.Sprintf("Sets the directory for the Polygon SDK data"),
		arguments: []string{
			"DATA_DIRECTORY",
		},
		argumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftInit) GetHelperText() string {
	return "Initializes IBFT for the Polygon SDK, in the specified directory"
}

// Help implements the cli.IbftInit interface
func (p *IbftInit) Help() string {
	p.DefineFlags()

	usage := "ibft init --data-dir DATA_DIRECTORY"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.IbftInit interface
func (p *IbftInit) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftInit interface
func (p *IbftInit) Run(args []string) int {
	flags := p.FlagSet("ibft init")

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

	if err := minimal.SetupDataDir(dataDir, []string{"consensus", "libp2p"}); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to write the ibft private key
	key, err := crypto.ReadPrivKey(filepath.Join(dataDir, "consensus", ibft.IbftKeyName))
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to create also a libp2p address
	libp2pKey, err := network.ReadLibp2pKey(filepath.Join(dataDir, "libp2p"))
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	nodeId, err := peer.IDFromPrivateKey(libp2pKey)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Output(fmt.Sprintf("Public key: %s", crypto.PubKeyToAddress(&key.PublicKey)))
	p.UI.Output(fmt.Sprintf("Node ID: %s", nodeId.String()))
	p.UI.Output("Done!")
	return 0
}
