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
	p.Meta.DefineFlags()

	usage := "peers-status PEER_ID"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersStatus interface
func (p *PeersStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersStatus interface
func (p *PeersStatus) Run(args []string) int {
	flags := p.FlagSet("peers-status")

	var peerId string
	flags.StringVar(&peerId, "id", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if peerId == "" {
		p.UI.Error("The PEER_ID argument is missing")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	resp, err := clt.PeersStatus(context.Background(), &proto.PeersStatusRequest{Id: peerId})
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
	return formatKV([]string{
		fmt.Sprintf("ID|%s", peer.Id),
		fmt.Sprintf("Protocols|%s", peer.Protocols),
		fmt.Sprintf("Addresses|%s", peer.Addrs),
	})
}
