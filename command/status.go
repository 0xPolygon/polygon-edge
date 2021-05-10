package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/minimal/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// StatusCommand is the command to show the version of the agent
type StatusCommand struct {
	Meta
}

// GetHelperText returns a simple description of the command
func (c *StatusCommand) GetHelperText() string {
	return "Returns the status of the Polygon SDK client"
}

// Help implements the cli.Command interface
func (c *StatusCommand) Help() string {
	usage := "status"

	return c.GenerateHelp(c.Synopsis(), usage)
}

// Synopsis implements the cli.Command interface
func (c *StatusCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *StatusCommand) Run(args []string) int {

	flags := c.FlagSet("status")
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

	fmt.Println(status)
	return 0
}
