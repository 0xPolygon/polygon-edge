package command

import (
	"context"

	ibftOp "github.com/0xPolygon/minimal/consensus/ibft2/proto"
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

	clt := ibftOp.NewOperatorClient(conn)
	clt.Candidates(context.Background(), &empty.Empty{})

	return 0
}
