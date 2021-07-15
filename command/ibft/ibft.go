package ibft

import "github.com/mitchellh/cli"

// IbftCommand is the top level ibft command
type IbftCommand struct {
}

// Help implements the cli.Command interface
func (c *IbftCommand) Help() string {
	return c.Synopsis()
}

func (c *IbftCommand) GetBaseCommand() string {
	return "ibft"
}

// Synopsis implements the cli.Command interface
func (c *IbftCommand) Synopsis() string {
	return "Top level IBFT command for interacting with the IBFT consensus. Only accepts subcommands"
}

// Run implements the cli.Command interface
func (c *IbftCommand) Run(args []string) int {
	return cli.RunResultHelp
}
