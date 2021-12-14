package ibft

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
	ibftOp "github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
)

// IbftSnapshot is the command to query the snapshot
type IbftSnapshot struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *IbftSnapshot) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)

	p.FlagMap["number"] = helper.FlagDescriptor{
		Description: "The block height (number) for the snapshot",
		Arguments: []string{
			"BLOCK_NUMBER",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftSnapshot) GetHelperText() string {
	return "Returns the IBFT snapshot at the latest block number, unless a block number is specified"
}

func (p *IbftSnapshot) GetBaseCommand() string {
	return "ibft snapshot"
}

// Help implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Run(args []string) int {
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	// query a specific snapshot
	var number int64
	flags.Int64Var(&number, "number", -1, "")

	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	req := &proto.SnapshotReq{}
	if number >= 0 {
		req.Number = uint64(number)
	} else {
		req.Latest = true
	}

	clt := ibftOp.NewIbftOperatorClient(conn)
	resp, err := clt.GetSnapshot(context.Background(), req)
	if err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	res := NewIBFTSnapshotResult(resp)
	p.Formatter.OutputResult(res)

	return 0
}

type IBFTSnapshotVote struct {
	Proposer string `json:"proposer"`
	Address  string `json:"address"`
	Vote     Vote   `json:"vote"`
}

type IBFTSnapshotResult struct {
	Number     uint64             `json:"number"`
	Hash       string             `json:"hash"`
	Votes      []IBFTSnapshotVote `json:"votes"`
	Validators []string           `json:"validators"`
}

func NewIBFTSnapshotResult(resp *ibftOp.Snapshot) *IBFTSnapshotResult {
	res := &IBFTSnapshotResult{
		Number:     resp.Number,
		Hash:       resp.Hash,
		Votes:      make([]IBFTSnapshotVote, len(resp.Votes)),
		Validators: make([]string, len(resp.Validators)),
	}
	for i, v := range resp.Votes {
		res.Votes[i].Proposer = v.Validator
		res.Votes[i].Address = v.Proposed
		res.Votes[i].Vote = voteToString(v.Auth)
	}
	for i, v := range resp.Validators {
		res.Validators[i] = v.Address
	}
	return res
}

func (r *IBFTSnapshotResult) Output() string {
	var buffer bytes.Buffer

	// current number & hash
	buffer.WriteString("\n[IBFT SNAPSHOT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Block|%d", r.Number),
		fmt.Sprintf("Hash|%s", r.Hash),
	}))
	buffer.WriteString("\n")

	// votes
	numVotes := len(r.Votes)
	votes := make([]string, numVotes+1)
	if numVotes == 0 {
		votes[0] = "No votes found"
	} else {
		votes[0] = "PROPOSER|ADDRESS|VOTE TO ADD"
		for i, d := range r.Votes {
			votes[i+1] = fmt.Sprintf("%s|%s|%v", d.Proposer, d.Address, d.Vote == VoteAdd)
		}
	}
	buffer.WriteString("\n[VOTES]\n")
	buffer.WriteString(helper.FormatList(votes))
	buffer.WriteString("\n")

	// validators
	numValidators := len(r.Validators)
	validators := make([]string, numValidators+1)
	if numValidators == 0 {
		validators[0] = "No validators found"
	} else {
		validators[0] = "ADDRESS"
		for i, d := range r.Validators {
			validators[i+1] = d
		}
	}
	buffer.WriteString("\n[VALIDATORS]\n")
	buffer.WriteString(helper.FormatList(validators))
	buffer.WriteString("\n")

	return buffer.String()
}
