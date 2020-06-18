package core

import "github.com/0xPolygon/minimal/types"

func (c *core) handleFinalCommitted() error {
	logger := c.logger.New("state", c.state)
	logger.Trace("Received a final committed proposal")
	c.startNewRound(types.Big0)
	return nil
}
