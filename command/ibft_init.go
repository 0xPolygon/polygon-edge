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

// Help implements the cli.IbftInit interface
func (p *IbftInit) Help() string {
	return ""
}

// Synopsis implements the cli.IbftInit interface
func (p *IbftInit) Synopsis() string {
	return ""
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
		p.UI.Error("number expected")
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
