package txpool

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// TxPoolStatus is the command to query the snapshot
type TxPoolStatus struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *TxPoolStatus) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)
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
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.TxPoolStatus interface
func (p *TxPoolStatus) Run(args []string) int {
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	if err := flags.Parse(args); err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	clt := txpoolOp.NewTxnPoolOperatorClient(conn)

	resp, err := clt.Status(context.Background(), &empty.Empty{})
	if err != nil {
		p.Formatter.OutputError(err)

		return 1
	}

	res := &TxPoolStatusResult{
		Txs: resp.Length,
	}
	p.Formatter.OutputResult(res)

	return 0
}

type TxPoolStatusResult struct {
	Txs uint64 `json:"txs"`
}

func (r *TxPoolStatusResult) Output() string {
	var buffer bytes.Buffer

	// current number & hash
	buffer.WriteString("\n[TXPOOL STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Number of transactions in pool:|%d", r.Txs),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
