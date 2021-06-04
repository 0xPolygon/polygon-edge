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

func (p *PeersStatus) DefineFlags() {
	if p.flagMap == nil {
		// Flag map not initialized
		p.flagMap = make(map[string]FlagDescriptor)
	}

	p.flagMap["libp2p-node-id"] = FlagDescriptor{
		description: fmt.Sprintf("A unique reference to a specific peer within p2p network"),
		arguments: []string{
			"PEER_ID",
		},
		argumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *PeersStatus) GetHelperText() string {
	return "Returns the status of the specified peer, using the libp2p ID of the peer"
}

// Help implements the cli.PeersStatus interface
func (p *PeersStatus) Help() string {
	p.DefineFlags()

	usage := "peers status --libp2p-node-id PEER_ID"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersStatus interface
func (p *PeersStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersStatus interface
func (p *PeersStatus) Run(args []string) int {
	flags := p.FlagSet("peers status")

	var nodeId string
	flags.StringVar(&nodeId, "libp2p-node-id", "", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if nodeId == "" {
		p.UI.Error("libp2p-node-id argument not provided")
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

	fmt.Println("-- PEER STATUS --")
	fmt.Println(resp)

	return 0
}
