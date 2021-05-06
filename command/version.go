package command

import (
	"github.com/0xPolygon/minimal/version"
	"github.com/mitchellh/cli"
)

// VersionCommand is the command to show the version of the agent
type VersionCommand struct {
	UI cli.Ui
}

// Help implements the cli.Command interface
func (c *VersionCommand) Help() string {
	return "Returns the current Polygon SDK version"
}

// Synopsis implements the cli.Command interface
func (c *VersionCommand) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (c *VersionCommand) Run(args []string) int {
	c.UI.Output(version.GetVersion())

	return 0
}
