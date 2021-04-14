package command

import (
	"context"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	ibftOp "github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
)

// IbftPropose is the command to query the snapshot
type IbftPropose struct {
	Meta
}

// Help implements the cli.IbftPropose interface
func (p *IbftPropose) Help() string {
	return ""
}

// Synopsis implements the cli.IbftPropose interface
func (p *IbftPropose) Synopsis() string {
	return ""
}

// Run implements the cli.IbftPropose interface
func (p *IbftPropose) Run(args []string) int {
	flags := p.FlagSet("ibft propose")

	var add, del bool
	flags.BoolVar(&add, "add", false, "add")
	flags.BoolVar(&del, "del", false, "del")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	if add == del {
		// either (add=true, del=true) or (add=false, del=false)
		p.UI.Error("only one of add and del needs to be set")
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.UI.Error("number expected")
		return 1
	}

	var addr types.Address
	if err := addr.UnmarshalText([]byte(args[0])); err != nil {
		p.UI.Error("failed to decode addr")
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := ibftOp.NewIbftOperatorClient(conn)
	req := &proto.Candidate{
		Address: addr.String(),
		Auth:    add,
	}
	_, err = clt.Propose(context.Background(), req)
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}
	return 0
}
