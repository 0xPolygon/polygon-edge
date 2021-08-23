package status

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/minimal/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// StatusCommand is the command to show the version of the agent
type StatusCommand struct {
	helper.Meta
}

// GetHelperText returns a simple description of the command
func (c *StatusCommand) GetHelperText() string {
	return "Returns the status of the Polygon SDK client"
}

func (c *StatusCommand) GetBaseCommand() string {
	return "status"
}

// Help implements the cli.Command interface
func (c *StatusCommand) Help() string {
	c.Meta.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *StatusCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *StatusCommand) Run(args []string) int {

	flags := c.FlagSet(c.GetBaseCommand())
	if err := flags.Parse(args); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	conn, err := c.Conn()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	clt := proto.NewSystemClient(conn)
	status, err := clt.GetStatus(context.Background(), &emptypb.Empty{})
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	output := "\n[CLIENT STATUS]\n"
	output += helper.FormatKV([]string{
		fmt.Sprintf("Network (Chain ID)|%d", status.Network),
		fmt.Sprintf("Current Block Number (base 10)|%d", status.Current.Number),
		fmt.Sprintf("Current Block Hash|%s", status.Current.Hash),
		fmt.Sprintf("Libp2p Address|%s", status.P2PAddr),
	})

	output += "\n"

	c.UI.Info(output)

	return 0
}
