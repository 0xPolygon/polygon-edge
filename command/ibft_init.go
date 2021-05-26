package command

import (
	"fmt"
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

// GetHelperText returns a simple description of the command
func (p *IbftInit) GetHelperText() string {
	return "Initializes IBFT for the Polygon SDK, in the specified directory"
}

// Help implements the cli.IbftInit interface
func (p *IbftInit) Help() string {
	usage := "ibft init DATA_DIRECTORY"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.IbftInit interface
func (p *IbftInit) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftInit interface
func (p *IbftInit) Run(args []string) int {
	flags := p.FlagSet("ibft init")
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.UI.Error("required argument (data directory) not passed in")
		return 1
	}

	pathName := args[0]
	if err := minimal.SetupDataDir(pathName, []string{"consensus", "libp2p"}); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to write the ibft private key
	key, err := crypto.ReadPrivKey(filepath.Join(pathName, "consensus", ibft.IbftKeyName))
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to create also a libp2p address
	if _, err := network.ReadLibp2pKey(filepath.Join(pathName, "libp2p")); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Output(fmt.Sprintf("Public key: %s", crypto.PubKeyToAddress(&key.PublicKey)))
	p.UI.Output("Done!")
	return 0
}
