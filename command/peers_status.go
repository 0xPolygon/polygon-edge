package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersStatus is the PeersStatus to start the sever
type PeersStatus struct {
	Meta
}

// GetHelperText returns a simple description of the command
func (p *PeersStatus) GetHelperText() string {
	return "Returns the status of the specified peer, using the libp2p ID of the peer"
}

// Help implements the cli.PeersStatus interface
func (p *PeersStatus) Help() string {
	usage := "peers status PEER_ID"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersStatus interface
func (p *PeersStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersStatus interface
func (p *PeersStatus) Run(args []string) int {
	flags := p.FlagSet("peers status")
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.UI.Error("peer id argument not provided")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	resp, err := clt.PeersStatus(context.Background(), &proto.PeersStatusRequest{Id: args[0]})
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	fmt.Println("-- PEER STATUS --")
	fmt.Println(resp)

	return 0
}
