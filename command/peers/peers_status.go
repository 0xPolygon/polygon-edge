package peers

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersStatus is the PeersStatus to start the sever
type PeersStatus struct {
	helper.Meta
}

func (p *PeersStatus) DefineFlags() {
	if p.FlagMap == nil {
		// Flag map not initialized
		p.FlagMap = make(map[string]helper.FlagDescriptor)
	}

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
	p.Meta.DefineFlags()
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.PeersStatus interface
func (p *PeersStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersStatus interface
func (p *PeersStatus) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

	var nodeId string
	flags.StringVar(&nodeId, "peer-id", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if nodeId == "" {
		p.UI.Error("peer-id argument not provided")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	resp, err := clt.PeersStatus(context.Background(), &proto.PeersStatusRequest{Id: nodeId})
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	var output = "\n[PEER STATUS]\n"
	output += formatPeerStatus(resp)

	output += "\n"

	p.UI.Info(output)

	return 0
}

// formatPeerStatus formats the peer status response for a single peer
func formatPeerStatus(peer *proto.Peer) string {
	return helper.FormatKV([]string{
		fmt.Sprintf("ID|%s", peer.Id),
		fmt.Sprintf("Protocols|%s", peer.Protocols),
		fmt.Sprintf("Addresses|%s", peer.Addrs),
	})
}
