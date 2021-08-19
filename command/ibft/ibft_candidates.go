package ibft

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	ibftOp "github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// IbftCandidates is the command to query the snapshot
type IbftCandidates struct {
	helper.Meta
}

// GetHelperText returns a simple description of the command
func (p *IbftCandidates) GetHelperText() string {
	return "Queries the current set of proposed candidates, as well as candidates that have not been included yet"
}

func (p *IbftCandidates) GetBaseCommand() string {
	return "ibft candidates"
}

// Help implements the cli.IbftCandidates interface
func (p *IbftCandidates) Help() string {
	p.Meta.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftCandidates interface
func (p *IbftCandidates) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftCandidates interface
func (p *IbftCandidates) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := ibftOp.NewIbftOperatorClient(conn)
	resp, err := clt.Candidates(context.Background(), &empty.Empty{})
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	output := "\n[IBFT CANDIDATES]\n"

	if len(resp.Candidates) == 0 {
		output += "No candidates found"
	} else {
		output += fmt.Sprintf("Number of candidates: %d\n\n", len(resp.Candidates))

		output += formatCandidates(resp.Candidates)
	}

	output += "\n"

	p.UI.Output(output)

	return 0
}

func formatCandidates(candidates []*ibftOp.Candidate) string {
	var generatedCandidates []string

	generatedCandidates = append(generatedCandidates, "Address|Vote")

	for _, c := range candidates {
		generatedCandidates = append(generatedCandidates, fmt.Sprintf("%s|%s", c.Address, voteToString(c.Auth)))
	}

	return helper.FormatKV(generatedCandidates)
}

func voteToString(vote bool) string {
	if vote {
		return "ADD"
	}

	return "REMOVE"
}
