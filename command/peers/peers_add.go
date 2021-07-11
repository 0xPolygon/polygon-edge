package peers

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/command/helper"
	helperFlags "github.com/0xPolygon/minimal/helper/flags"
	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	helper.Meta
}

func (p *PeersAdd) DefineFlags() {
	if p.FlagMap == nil {
		// Flag map not initialized
		p.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	p.FlagMap["addr"] = helper.FlagDescriptor{
		Description: "Peer's libp2p address in the multiaddr format",
		Arguments: []string{
			"PEER_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}
}

// GetHelperText returns a simple description of the command
func (p *PeersAdd) GetHelperText() string {
	return "Adds new peers to the peer list, using the peer's libp2p address"
}

func (p *PeersAdd) GetBaseCommand() string {
	return "peers add"
}

// Help implements the cli.PeersAdd interface
func (p *PeersAdd) Help() string {
	p.Meta.DefineFlags()
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

	var passedInAddresses = make(helperFlags.ArrayFlags, 0)
	flags.Var(&passedInAddresses, "addr", "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if len(passedInAddresses) < 1 {
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
	for _, address := range passedInAddresses {
		if _, err := clt.PeersAdd(context.Background(), &proto.PeersAddRequest{Id: address}); err != nil {
			visibleErrors = append(visibleErrors, err.Error())
			break
		}

		peersAdded++
		addedPeers = append(addedPeers, address)
	}

	var output = "\n[PEERS ADDED]\n"
	output += helper.FormatKV([]string{
		fmt.Sprintf("Peers listed|%d", len(passedInAddresses)), // The number of peers the user wanted to add
		fmt.Sprintf("Peers added|%d", peersAdded),              // The number of peers that have been added
	})

	if len(addedPeers) > 0 {
		output += "\n\n[LIST OF ADDED PEERS]\n"
		output += helper.FormatList(addedPeers)
	}

	if len(visibleErrors) > 0 {
		output += "\n\n[ERRORS]\n"
		output += helper.FormatList(visibleErrors)
	}

	output += "\n"

	p.UI.Info(output)

	return 0
}
