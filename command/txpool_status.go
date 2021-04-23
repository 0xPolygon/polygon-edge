package command

import (
	"context"
	"fmt"

	txpoolOp "github.com/0xPolygon/minimal/txpool/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

// TxPoolStatus is the command to query the snapshot
type TxPoolStatus struct {
	Meta
}

// Help implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Help() string {
	return ""
}

// Synopsis implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Synopsis() string {
	return ""
}

// Run implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Run(args []string) int {
	flags := p.FlagSet("txpool status")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := txpoolOp.NewTxnPoolOperatorClient(conn)
	fmt.Println(clt)

	resp, err := clt.Status(context.Background(), &empty.Empty{})
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	p.UI.Output(fmt.Sprintf("%d", resp.Length))
	return 0
}
