package command

import (
	"github.com/spf13/cobra"
)

// OutputFormatter is the standardized interface all output formatters
// should use
type OutputFormatter interface {
	// SetError sets the encountered error
	SetError(err error)

	// SetCommandResult sets the result of the command execution
	SetCommandResult(result CommandResult)

	// WriteOutput writes the previously set result / error output
	WriteOutput()

	// WriteCommandResult immediately writes the given command result without waiting for WriteOutput func call.
	WriteCommandResult(result CommandResult)

	// Write extends io.Writer interface
	Write(p []byte) (n int, err error)
}

type CommandResult interface {
	GetOutput() string
}

func InitializeOutputter(cmd *cobra.Command) OutputFormatter {
	if shouldOutputJSON(cmd) {
		return newJSONOutput()
	}

	return newCLIOutput()
}

func shouldOutputJSON(baseCmd *cobra.Command) bool {
	jsonOutputFlag := baseCmd.Flag(JSONOutputFlag)
	if jsonOutputFlag == nil {
		return false
	}

	return jsonOutputFlag.Changed
}
