package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	ibftOp "github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/types"
)

// IbftSnapshot is the command to query the snapshot
type IbftSnapshot struct {
	Meta
}

// DefineFlags defines the command flags
func (p *IbftSnapshot) DefineFlags() {
	if p.flagMap == nil {
		// Flag map not initialized
		p.flagMap = make(map[string]types.FlagDescriptor)
	}

	p.flagMap["number"] = MetaFlagDescriptor{
		description: "The block height (number) for the snapshot",
		arguments: []string{
			"BLOCK_NUMBER",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (p *IbftSnapshot) GetHelperText() string {
	return "Returns the IBFT snapshot at the latest block number, unless a block number is specified"
}

func (p *IbftSnapshot) GetBaseCommand() string {
	return "ibft-snapshot"
}

// Help implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Help() string {
	p.Meta.DefineFlags()
	p.DefineFlags()

	return types.GenerateHelp(p.Synopsis(), types.GenerateUsage(p.GetBaseCommand(), p.flagMap), p.flagMap)
}

// Synopsis implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

	// query a specific snapshot
	var number int64
	flags.Int64Var(&number, "number", -1, "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
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
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Output(printSnapshot(resp))
	return 0
}

func printSnapshot(s *proto.Snapshot) (output string) {
	output += "\n[IBFT SNAPSHOT]\n"
	output += formatKV([]string{
		fmt.Sprintf("Block|%d", s.Number),
		fmt.Sprintf("Hash|%s", s.Hash),
	})

	output += "\n"

	var votes []string = nil
	if len(s.Votes) == 0 {
		votes = make([]string, 1)
		votes[0] = "No votes found"
	} else {
		votes = make([]string, len(s.Votes)+1)
		votes[0] = "Proposer|Address|Authorize"
		for i, d := range s.Votes {
			votes[i+1] = fmt.Sprintf("%s|%s|%v", d.Validator, d.Proposed, d.Auth)
		}
	}

	output += "\n[VOTES]\n"
	output += formatList(votes)

	output += "\n"

	var validators []string = nil
	if len(s.Validators) == 0 {
		validators = make([]string, 1)
		validators[0] = "No validators found"
	} else {
		validators = make([]string, len(s.Validators)+1)
		validators[0] = "Address"
		for i, d := range s.Validators {
			validators[i+1] = d.Address
		}
	}

	output += "\n"

	output += "\n[VALIDATORS]\n"
	output += formatList(validators)

	return output
}
