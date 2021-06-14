package command

import (
	"context"
	"fmt"

	helperFlags "github.com/0xPolygon/minimal/helper/flags"
	"github.com/0xPolygon/minimal/minimal/proto"
)

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	Meta
}

// DefineFlags defines the command flags
func (p *PeersAdd) DefineFlags() {
	if p.flagMap == nil {
		// Flag map not initialized
		p.flagMap = make(map[string]FlagDescriptor)
	}

	p.flagMap["a"] = FlagDescriptor{
		description: "Specifies the libp2p address of the peer in the format /ip4/<ip_address>/tcp/<port>/p2p/<node_id>",
		arguments: []string{
			"PEER_ADDRESS",
		},
		argumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *PeersAdd) GetHelperText() string {
	return "Adds new peers to the peer list, using the peer's libp2p address"
}

// Help implements the cli.PeersAdd interface
func (p *PeersAdd) Help() string {
	p.Meta.DefineFlags()
	p.DefineFlags()

	usage := "peers-add -a PEER_ADDRESS [-a PEER_ADDRESS ...]"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	flags := p.FlagSet("peers-add")

	var passedInAddresses = make(helperFlags.ArrayFlags, 0)
	flags.Var(&passedInAddresses, "a", "")

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
	output += formatKV([]string{
		fmt.Sprintf("Peers listed|%d", len(passedInAddresses)), // The number of peers the user wanted to add
		fmt.Sprintf("Peers added|%d", peersAdded),              // The number of peers that have been added
	})

	if len(addedPeers) > 0 {
		output += "\n\n[LIST OF DIALED PEERS]\n"
		output += formatList(addedPeers)
	}

	if len(visibleErrors) > 0 {
		output += "\n\n[ERRORS]\n"
		output += formatList(visibleErrors)
	}

	output += "\n"

	p.UI.Info(output)

	return 0
}
