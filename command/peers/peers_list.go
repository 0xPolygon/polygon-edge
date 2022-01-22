package peers

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// PeersList is the PeersList to start the sever
type PeersList struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *PeersList) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)
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
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.PeersList interface
func (p *PeersList) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersList interface
func (p *PeersList) Run(args []string) int {
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)
	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	clt := proto.NewSystemClient(conn)
	resp, err := clt.PeersList(context.Background(), &empty.Empty{})

	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	res := NewPeersListResult(resp)
	p.Formatter.OutputResult(res)

	return 0
}

type PeersListResult struct {
	Peers []string `json:"peers"`
}

func NewPeersListResult(resp *proto.PeersListResponse) *PeersListResult {
	peers := make([]string, len(resp.Peers))
	for i, p := range resp.Peers {
		peers[i] = p.Id
	}

	return &PeersListResult{
		Peers: peers,
	}
}

func (r *PeersListResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEERS LIST]\n")

	if len(r.Peers) == 0 {
		buffer.WriteString("No peers found")
	} else {
		buffer.WriteString(fmt.Sprintf("Number of peers: %d\n\n", len(r.Peers)))

		rows := make([]string, len(r.Peers))
		for i, p := range r.Peers {
			rows[i] = fmt.Sprintf("[%d]|%s", i, p)
		}
		buffer.WriteString(helper.FormatKV(rows))
	}

	buffer.WriteString("\n")

	return buffer.String()
}
