package peers

import "github.com/mitchellh/cli"

// PeersCommand is the top level ibft command
type PeersCommand struct {
}

// Help implements the cli.Command interface
func (c *PeersCommand) Help() string {
	return c.Synopsis()
}

func (c *PeersCommand) GetBaseCommand() string {
	return "peers"
}

// Synopsis implements the cli.Command interface
func (c *PeersCommand) Synopsis() string {
	return "Top level command for interacting with the network peers. Only accepts subcommands"
}

// Run implements the cli.Command interface
func (c *PeersCommand) Run(args []string) int {
	return cli.RunResultHelp
}
