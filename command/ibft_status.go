package command

import (
	"context"

	ibftOp "github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

// IbftStatus is the command to query the snapshot
type IbftStatus struct {
	Meta
}

// Help implements the cli.IbftStatus interface
func (p *IbftStatus) Help() string {
	return ""
}

// Synopsis implements the cli.IbftStatus interface
func (p *IbftStatus) Synopsis() string {
	return ""
}

// Run implements the cli.IbftStatus interface
func (p *IbftStatus) Run(args []string) int {
	flags := p.FlagSet("ibft propose")

	var add, del bool
	flags.BoolVar(&add, "add", false, "add")
	flags.BoolVar(&del, "del", false, "del")

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
	resp, err := clt.Status(context.Background(), &empty.Empty{})
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Output(resp.Key)
	return 0
}
