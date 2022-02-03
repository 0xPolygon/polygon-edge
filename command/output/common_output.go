package output

import (
	"github.com/spf13/cobra"
)

type commonOutputFormatter struct {
	baseCmd       *cobra.Command
	errorOutput   error
	commandOutput CommandResult
}

func (c *commonOutputFormatter) SetError(err error) {
	c.errorOutput = err
}

func (c *commonOutputFormatter) SetCommandResult(result CommandResult) {
	c.commandOutput = result
}
