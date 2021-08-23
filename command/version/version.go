package version

import (
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/version"
	"github.com/mitchellh/cli"
)

// VersionCommand is the command to show the version of the agent
type VersionCommand struct {
	UI cli.Ui
	helper.Meta
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
	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
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
