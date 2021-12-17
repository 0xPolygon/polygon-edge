package ibft

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	ibftOp "github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// IbftCandidates is the command to query the snapshot
type IbftCandidates struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *IbftCandidates) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)
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
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftCandidates interface
func (p *IbftCandidates) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftCandidates interface
func (p *IbftCandidates) Run(args []string) int {
	flags := p.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)
	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	clt := ibftOp.NewIbftOperatorClient(conn)
	resp, err := clt.Candidates(context.Background(), &empty.Empty{})
	if err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	res := NewIBFTCandidatesResult(resp)
	p.Formatter.OutputResult(res)

	return 0
}

type IBFTCandidate struct {
	Address string `json:"address"`
	Vote    Vote   `json:"vote"`
}

type IBFTCandidatesResult struct {
	Candidates []IBFTCandidate `json:"candidates"`
}

func NewIBFTCandidatesResult(resp *ibftOp.CandidatesResp) *IBFTCandidatesResult {
	res := &IBFTCandidatesResult{
		Candidates: make([]IBFTCandidate, len(resp.Candidates)),
	}
	for i, c := range resp.Candidates {
		res.Candidates[i].Address = c.Address
		res.Candidates[i].Vote = voteToString(c.Auth)
	}
	return res
}

func (r *IBFTCandidatesResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[IBFT CANDIDATES]\n")
	if num := len(r.Candidates); num == 0 {
		buffer.WriteString("No candidates found")
	} else {
		buffer.WriteString(fmt.Sprintf("Number of candidates: %d\n\n", num))
		buffer.WriteString(formatCandidates(r.Candidates))
	}
	buffer.WriteString("\n")

	return buffer.String()
}

func formatCandidates(candidates []IBFTCandidate) string {
	generatedCandidates := make([]string, 0, len(candidates)+1)

	generatedCandidates = append(generatedCandidates, "Address|Vote")
	for _, c := range candidates {
		generatedCandidates = append(generatedCandidates, fmt.Sprintf("%s|%s", c.Address, c.Vote))
	}

	return helper.FormatKV(generatedCandidates)
}
