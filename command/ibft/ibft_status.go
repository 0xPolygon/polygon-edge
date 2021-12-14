package ibft

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	ibftOp "github.com/0xPolygon/polygon-sdk/consensus/ibft/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// IbftStatus is the command to query the snapshot
type IbftStatus struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *IbftStatus) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)
}

// GetHelperText returns a simple description of the command
func (p *IbftStatus) GetHelperText() string {
	return "Returns the current validator key of the IBFT client"
}

func (p *IbftStatus) GetBaseCommand() string {
	return "ibft status"
}

// Help implements the cli.IbftStatus interface
func (p *IbftStatus) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.IbftStatus interface
func (p *IbftStatus) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.IbftStatus interface
func (p *IbftStatus) Run(args []string) int {
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

	clt := ibftOp.NewIbftOperatorClient(conn)
	resp, err := clt.Status(context.Background(), &empty.Empty{})
	if err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	res := &IBFTStatusResult{
		ValidatorKey: resp.Key,
	}
	p.Formatter.OutputResult(res)

	return 0
}

type IBFTStatusResult struct {
	ValidatorKey string `json:"validator_key"`
}

func (r *IBFTStatusResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Validator key|%s", r.ValidatorKey),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
