package txpool

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/command/helper"
	txpoolOp "github.com/0xPolygon/minimal/txpool/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// TxPoolStatus is the command to query the snapshot
type TxPoolStatus struct {
	helper.Meta
}

// GetHelperText returns a simple description of the command
func (p *TxPoolStatus) GetHelperText() string {
	return "Returns the number of transactions in the pool"
}

func (p *TxPoolStatus) GetBaseCommand() string {
	return "txpool status"
}

// Help implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Help() string {
	p.Meta.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

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

	output := "\n[TXPOOL STATUS]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Number of transactions in pool:|%d", resp.Length),
	})

	output += "\n"

	p.UI.Output(output)

	return 0
}
