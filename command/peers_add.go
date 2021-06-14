package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	Meta
}

// GetHelperText returns a simple description of the command
func (p *PeersAdd) GetHelperText() string {
	return "Adds new peers to the peer list, using the peer's libp2p address"
}

// Help implements the cli.PeersAdd interface
func (p *PeersAdd) Help() string {
	p.Meta.DefineFlags()

	usage := "peers-add PEER_ADDRESSES"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	flags := p.FlagSet("peers-add")
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	args = flags.Args()
	if len(args) < 1 {
		p.UI.Error("At least 1 peer address is required")
		return 1
	}

	// Connect to the gRPC layer
	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	var peersAdded int
	var addedPeers []string
	var visibleErrors []string

	// Adds all the peers and breaks if it hits an error
	clt := proto.NewSystemClient(conn)
	for _, address := range args {
		if _, err := clt.PeersAdd(context.Background(), &proto.PeersAddRequest{Id: address}); err != nil {
			visibleErrors = append(visibleErrors, err.Error())
			break
		}

		peersAdded++
		addedPeers = append(addedPeers, address)
	}

	var output = "\n[PEERS ADDED]\n"
	output += formatKV([]string{
		fmt.Sprintf("Peers listed|%d", len(args)), // The number of peers the user wanted to add
		fmt.Sprintf("Peers added|%d", peersAdded), // The number of peers that have been added
	})

	if len(addedPeers) > 0 {
		output += "\n\n[LIST OF ADDED PEERS]\n"
		output += formatList(addedPeers)
	}

	if len(visibleErrors) > 0 {
		output += "\n\n[ERRORS]\n"
		output += formatList(visibleErrors)
	}

	return 0
}
