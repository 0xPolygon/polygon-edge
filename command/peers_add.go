package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	Meta
}

func (p *PeersAdd) DefineFlags() {
	if p.flagMap == nil {
		// Flag map not initialized
		p.flagMap = make(map[string]FlagDescriptor)
	}

	p.flagMap["libp2p-address"] = FlagDescriptor{
		description: fmt.Sprintf("Peer's libp2p address in the multiaddr format"),
		arguments: []string{
			"PEER_ADDRESS",
		},
		argumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *PeersAdd) GetHelperText() string {
	return "Adds a new peer to the peer list, using the peer's libp2p address"
}

// Help implements the cli.PeersAdd interface
func (p *PeersAdd) Help() string {
	p.Meta.DefineFlags()
	p.DefineFlags()

	usage := "peers add --libp2p-address PEER_ADDRESS"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	flags := p.FlagSet("peers add")

	var peerAddress string
	flags.StringVar(&peerAddress, "libp2p-address", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if peerAddress == "" {
		p.UI.Error("libp2p-address argument expected")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	if _, err := clt.PeersAdd(context.Background(), &proto.PeersAddRequest{Id: peerAddress}); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Info("Peer added")
	return 0
}
