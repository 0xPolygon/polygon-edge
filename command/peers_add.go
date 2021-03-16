package command

import (
	"context"

	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	Meta
}

// Help implements the cli.PeersAdd interface
func (p *PeersAdd) Help() string {
	return ""
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return ""
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	flags := p.FlagSet("peers add")
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.UI.Error("peer id argument expected")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	if _, err := clt.PeersAdd(context.Background(), &proto.PeersAddRequest{Id: args[0]}); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Info("Peer added")
	return 0
}
