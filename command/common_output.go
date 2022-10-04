package command

import "sync"

type commonOutputFormatter struct {
	sync.Mutex
	errorOutput   error
	commandOutput CommandResult
}

func (c *commonOutputFormatter) SetError(err error) {
	c.Lock()
	c.errorOutput = err
	c.Unlock()
}

func (c *commonOutputFormatter) SetCommandResult(result CommandResult) {
	c.Lock()
	c.commandOutput = result
	c.Unlock()
}
