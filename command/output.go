package command

import (
	"github.com/spf13/cobra"
)

// OutputFormatter is the standardized interface all output formatters
// should use
type OutputFormatter interface {
	// getErrorOutput returns the CLI command error
	getErrorOutput() string

	// getCommandOutput returns the CLI command output
	getCommandOutput() string

	// SetError sets the encountered error
	SetError(err error)

	// SetCommandResult sets the result of the command execution
	SetCommandResult(result CommandResult)

	// WriteOutput writes the result / error output
	WriteOutput()
}

type CommandResult interface {
	GetOutput() string
}

func shouldOutputJSON(baseCmd *cobra.Command) bool {
	return baseCmd.Flag(JSONOutputFlag).Changed
}

func InitializeOutputter(cmd *cobra.Command) OutputFormatter {
	if shouldOutputJSON(cmd) {
		return newJSONOutput()
	}

	return newCLIOutput()
}
