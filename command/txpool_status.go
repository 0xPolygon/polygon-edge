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
	return "Returns the number of transactions in the pool"
}

// Synopsis implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Synopsis() string {
	return "txpool status"
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

	commandOutput := formatKV([]string{
		fmt.Sprintf("Number of txns in pool:|%d", resp.Length),
	})

	p.UI.Output(commandOutput)

	return 0
}
