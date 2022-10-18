package command

type commonOutputFormatter struct {
	errorOutput   error
	commandOutput CommandResult
}

func (c *commonOutputFormatter) SetError(err error) {
	c.errorOutput = err
}

func (c *commonOutputFormatter) SetCommandResult(result CommandResult) {
	c.commandOutput = result
}
