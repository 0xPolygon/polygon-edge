package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	ibftOp "github.com/0xPolygon/minimal/consensus/ibft/proto"
)

// IbftSnapshot is the command to query the snapshot
type IbftSnapshot struct {
	Meta
}

// Help implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Help() string {
	return ""
}

// Synopsis implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Synopsis() string {
	return ""
}

// Run implements the cli.IbftSnapshot interface
func (p *IbftSnapshot) Run(args []string) int {
	flags := p.FlagSet("ibft snapshot")

	// query a specific snapshot
	number := flags.Uint64("number", 0, "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	req := &proto.SnapshotReq{
		Latest: number == nil,
	}
	if number != nil {
		req.Number = *number
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
	output = formatKV([]string{
		fmt.Sprintf("Block|%d", s.Number),
		fmt.Sprintf("Hash|%s", s.Hash),
	})

	votes := make([]string, len(s.Votes)+1)
	votes[0] = "Proposer|Address|Authorize"
	for i, d := range s.Votes {
		votes[i+1] = fmt.Sprintf("%s|%s|%v", d.Validator, d.Proposed, d.Auth)
	}

	output += "\nVotes\n"
	output += formatList(votes)

	validators := make([]string, len(s.Validators)+1)
	validators[0] = "Address"
	for i, d := range s.Validators {
		validators[i+1] = d.Address
	}

	output += "\nValidators\n"
	output += formatList(validators)

	return output
}
