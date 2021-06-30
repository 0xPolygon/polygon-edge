package txpool

import "github.com/mitchellh/cli"

// TxPoolCommand is the top level ibft command
type TxPoolCommand struct {
}

// Help implements the cli.Command interface
func (c *TxPoolCommand) Help() string {
	return c.Synopsis()
}

func (c *TxPoolCommand) GetBaseCommand() string {
	return "txpool"
}

// Synopsis implements the cli.Command interface
func (c *TxPoolCommand) Synopsis() string {
	return "Top level command for interacting with the transaction pool. Only accepts subcommands"
}

// Run implements the cli.Command interface
func (c *TxPoolCommand) Run(args []string) int {
	return cli.RunResultHelp
}
