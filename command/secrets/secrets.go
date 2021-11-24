package secrets

import "github.com/mitchellh/cli"

// SecretsCommand is the top level secret manager command
type SecretsCommand struct {
}

// Help implements the cli.Command interface
func (c *SecretsCommand) Help() string {
	return c.Synopsis()
}

func (c *SecretsCommand) GetBaseCommand() string {
	return "secrets"
}

// Synopsis implements the cli.Command interface
func (c *SecretsCommand) Synopsis() string {
	return "Top level SecretsManager command for interacting with secrets functionality. Only accepts subcommands"
}

// Run implements the cli.Command interface
func (c *SecretsCommand) Run(args []string) int {
	return cli.RunResultHelp
}
