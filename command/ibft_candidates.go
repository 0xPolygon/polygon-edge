package command

import (
	"context"
	"fmt"

	ibftOp "github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

// IbftCandidates is the command to query the snapshot
type IbftCandidates struct {
	Meta
}

// Help implements the cli.IbftCandidates interface
func (p *IbftCandidates) Help() string {
	return ""
}

// Synopsis implements the cli.IbftCandidates interface
func (p *IbftCandidates) Synopsis() string {
	return ""
}

// Run implements the cli.IbftCandidates interface
func (p *IbftCandidates) Run(args []string) int {
	flags := p.FlagSet("ibft candidates")
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

	if len(resp.Candidates) == 0 {
		p.UI.Output("No candidates")
		return 0
	}

	for _, c := range resp.Candidates {
		p.UI.Output(fmt.Sprintf("%s %v", c.Address, c.Auth))
	}
	return 0
}
