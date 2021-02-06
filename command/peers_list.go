package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

// PeersList is the PeersList to start the sever
type PeersList struct {
	Meta
}

// Help implements the cli.PeersList interface
func (p *PeersList) Help() string {
	return ""
}

// Synopsis implements the cli.PeersList interface
func (p *PeersList) Synopsis() string {
	return ""
}

// Run implements the cli.PeersList interface
func (p *PeersList) Run(args []string) int {
	flags := p.FlagSet("peers list")
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	resp, err := clt.PeersList(context.Background(), &empty.Empty{})
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if len(resp.Peers) == 0 {
		p.UI.Info("No peers")
	} else {
		for _, peer := range resp.Peers {
			fmt.Println(peer.Id)
		}
	}
	return 0
}
