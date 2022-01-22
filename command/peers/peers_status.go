package peers

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
)

// PeersStatus is the PeersStatus to start the sever
type PeersStatus struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

func (p *PeersStatus) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)

	p.FlagMap["peer-id"] = helper.FlagDescriptor{
		Description: "Libp2p node ID of a specific peer within p2p network",
		Arguments: []string{
			"PEER_ID",
		},
		ArgumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *PeersStatus) GetHelperText() string {
	return "Returns the status of the specified peer, using the libp2p ID of the peer node"
}

func (p *PeersStatus) GetBaseCommand() string {
	return "peers status"
}

// Help implements the cli.PeersStatus interface
func (p *PeersStatus) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.PeersStatus interface
func (p *PeersStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersStatus interface
func (p *PeersStatus) Run(args []string) int {
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	var nodeID string

	flags.StringVar(&nodeID, "peer-id", "", "")

	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	if nodeID == "" {
		p.UI.Error("peer-id argument not provided")

		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	clt := proto.NewSystemClient(conn)
	resp, err := clt.PeersStatus(context.Background(), &proto.PeersStatusRequest{Id: nodeID})

	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	res := &PeersStatusResult{
		ID:        resp.Id,
		Protocols: resp.Protocols,
		Addresses: resp.Addrs,
	}
	p.Formatter.OutputResult(res)

	return 0
}

type PeersStatusResult struct {
	ID        string   `json:"id"`
	Protocols []string `json:"protocols"`
	Addresses []string `json:"addresses"`
}

func (r *PeersStatusResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEER STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("ID|%s", r.ID),
		fmt.Sprintf("Protocols|%s", r.Protocols),
		fmt.Sprintf("Addresses|%s", r.Addresses),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
