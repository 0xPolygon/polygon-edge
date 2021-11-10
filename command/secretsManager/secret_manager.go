package secretsManager

import "github.com/mitchellh/cli"

// SecretManagerCommand is the top level secret manager command
type SecretManagerCommand struct {
}

// Help implements the cli.Command interface
func (c *SecretManagerCommand) Help() string {
	return c.Synopsis()
}

func (c *SecretManagerCommand) GetBaseCommand() string {
	return "secrets-manager"
}

// Synopsis implements the cli.Command interface
func (c *SecretManagerCommand) Synopsis() string {
	return "Top level SecretsManager command for interacting with secrets-manager functionality. Only accepts subcommands"
}

// Run implements the cli.Command interface
func (c *SecretManagerCommand) Run(args []string) int {
	return cli.RunResultHelp
}
