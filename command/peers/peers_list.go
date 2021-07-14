package peers

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

// PeersList is the PeersList to start the sever
type PeersList struct {
	helper.Meta
}

// GetHelperText returns a simple description of the command
func (p *PeersList) GetHelperText() string {
	return "Returns the list of connected peers, including the current node"
}

func (p *PeersList) GetBaseCommand() string {
	return "peers list"
}

// Help implements the cli.PeersList interface
func (p *PeersList) Help() string {
	p.Meta.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.PeersList interface
func (p *PeersList) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersList interface
func (p *PeersList) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())
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

	output := "\n[PEERS LIST]\n"

	if len(resp.Peers) == 0 {
		output += "No peers found"
	} else {
		output += fmt.Sprintf("Number of peers: %d\n\n", len(resp.Peers))

		output += formatPeers(resp.Peers)
	}

	output += "\n"

	p.UI.Output(output)

	return 0
}

func formatPeers(peers []*proto.Peer) string {
	var generatedRows []string
	for i := 0; i < len(peers); i++ {
		generatedRows = append(generatedRows, fmt.Sprintf("[%d]|%s", i, peers[i].Id))
	}

	return helper.FormatKV(generatedRows)
}
