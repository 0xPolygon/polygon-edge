package peers

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	helperFlags "github.com/0xPolygon/polygon-edge/helper/flags"
	"github.com/0xPolygon/polygon-edge/server/proto"
)

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

func (p *PeersAdd) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)

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
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	var passedInAddresses = make(helperFlags.ArrayFlags, 0)

	flags.Var(&passedInAddresses, "addr", "")

	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	if len(passedInAddresses) < 1 {
		p.Formatter.OutputError(errors.New("at least 1 peer address is required"))

		return 1
	}

	// Connect to the gRPC layer
	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	var (
		peersAdded    int
		addedPeers    = make([]string, 0)
		visibleErrors = make([]string, 0)
	)

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

	res := &PeersAddResult{
		NumRequested: len(passedInAddresses),
		NumAdded:     peersAdded,
		Peers:        addedPeers,
		Errors:       visibleErrors,
	}
	p.Formatter.OutputResult(res)

	return 0
}

type PeersAddResult struct {
	NumRequested int      `json:"num_requested"`
	NumAdded     int      `json:"num_added"`
	Peers        []string `json:"peers"`
	Errors       []string `json:"errors"`
}

func (r *PeersAddResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEERS ADDED]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Peers listed|%d", r.NumRequested), // The number of peers the user wanted to add
		fmt.Sprintf("Peers added|%d", r.NumAdded),      // The number of peers that have been added
	}))

	if len(r.Peers) > 0 {
		buffer.WriteString("\n\n[LIST OF ADDED PEERS]\n")
		buffer.WriteString(helper.FormatList(r.Peers))
	}

	if len(r.Errors) > 0 {
		buffer.WriteString("\n\n[ERRORS]\n")
		buffer.WriteString(helper.FormatList(r.Errors))
	}

	buffer.WriteString("\n")

	return buffer.String()
}
