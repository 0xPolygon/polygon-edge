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

// GetHelperText returns a simple description of the command
func (p *PeersList) GetHelperText() string {
	return "Returns the list of connected peers, including the current node"
}

// Help implements the cli.PeersList interface
func (p *PeersList) Help() string {
	p.Meta.DefineFlags()

	usage := "peers-list"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersList interface
func (p *PeersList) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersList interface
func (p *PeersList) Run(args []string) int {
	flags := p.FlagSet("peers-list")
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
	output += formatPeers(resp.Peers)

	output += "\n"

	p.UI.Output(output)

	return 0
}

func formatPeers(peers []*proto.Peer) string {
	if len(peers) == 0 {
		return "No peers found"
	}

	rows := make([]string, len(peers)+1)
	rows[0] = "ID"
	for i, d := range peers {
		rows[i+1] = fmt.Sprintf("%s", d.Id)
	}

	var generatedRows []string
	for i := 0; i < len(peers); i++ {
		generatedRows = append(generatedRows, fmt.Sprintf("[%d]|%s", i, peers[i].Id))
	}

	return formatKV(generatedRows)
}
