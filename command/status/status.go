package status

import (
	"bytes"
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// StatusCommand is the command to show the version of the agent
type StatusCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (c *StatusCommand) DefineFlags() {
	c.Base.DefineFlags(c.Formatter, c.GRPC)
}

// GetHelperText returns a simple description of the command
func (c *StatusCommand) GetHelperText() string {
	return "Returns the status of the Polygon Edge client"
}

func (c *StatusCommand) GetBaseCommand() string {
	return "status"
}

// Help implements the cli.Command interface
func (c *StatusCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *StatusCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *StatusCommand) Run(args []string) int {
	flags := c.Base.NewFlagSet(c.GetBaseCommand(), c.Formatter, c.GRPC)
	if err := flags.Parse(args); err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	conn, err := c.GRPC.Conn()
	if err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	clt := proto.NewSystemClient(conn)
	status, err := clt.GetStatus(context.Background(), &emptypb.Empty{})

	if err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	res := &StatusResult{
		ChainID:            status.Network,
		CurrentBlockNumber: status.Current.Number,
		CurrentBlockHash:   status.Current.Hash,
		LibP2PAddress:      status.P2PAddr,
	}

	c.Formatter.OutputResult(res)

	return 0
}

type StatusResult struct {
	ChainID            int64  `json:"chain_id"`
	CurrentBlockNumber int64  `json:"current_block_number"`
	CurrentBlockHash   string `json:"current_block_hash"`
	LibP2PAddress      string `json:"libp2p_address"`
}

func (r *StatusResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[CLIENT STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Network (Chain ID)|%d", r.ChainID),
		fmt.Sprintf("Current Block Number (base 10)|%d", r.CurrentBlockNumber),
		fmt.Sprintf("Current Block Hash|%s", r.CurrentBlockHash),
		fmt.Sprintf("Libp2p Address|%s", r.LibP2PAddress),
	}))

	return buffer.String()
}
