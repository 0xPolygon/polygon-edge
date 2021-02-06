package command

import (
	"github.com/mitchellh/cli"
)

// DevCommand is the command to show the version of the agent
type DevCommand struct {
	UI cli.Ui
}

// Help implements the cli.Command interface
func (c *DevCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *DevCommand) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (c *DevCommand) Run(args []string) int {
	// TODO
	return 0
}
