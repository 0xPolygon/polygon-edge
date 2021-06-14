package command

import (
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/version"
	"github.com/mitchellh/cli"
)

// VersionCommand is the command to show the version of the agent
type VersionCommand struct {
	UI cli.Ui
	Meta
}

// GetHelperText returns a simple description of the command
func (c *VersionCommand) GetHelperText() string {
	return "Returns the current Polygon SDK version"
}

func (c *VersionCommand) GetBaseCommand() string {
	return "version"
}

// Help implements the cli.Command interface
func (c *VersionCommand) Help() string {
	return types.GenerateHelp(c.Synopsis(), types.GenerateUsage(c.GetBaseCommand(), c.flagMap), c.flagMap)
}

// Synopsis implements the cli.Command interface
func (c *VersionCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *VersionCommand) Run(args []string) int {
	c.UI.Output(version.GetVersion())

	return 0
}
