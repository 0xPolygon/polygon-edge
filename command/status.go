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

// Help implements the cli.Command interface
func (c *StatusCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *StatusCommand) Synopsis() string {
	return ""
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
