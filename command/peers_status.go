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

// Help implements the cli.PeersStatus interface
func (p *PeersStatus) Help() string {
	return ""
}

// Synopsis implements the cli.PeersStatus interface
func (p *PeersStatus) Synopsis() string {
	return ""
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
		p.UI.Error("peer id argument expected")
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

	fmt.Println("-- resp --")
	fmt.Println(resp)

	return 0
}
