package polybft

import (
	"context"

	"github.com/0xPolygon/go-ibft/core"
)

// IBFTConsensusWrapper is a convenience wrapper for the go-ibft package
type IBFTConsensusWrapper struct {
	*core.IBFT

	cancelSequence context.CancelFunc
	sequenceDone   chan struct{}
}

func newIBFTConsensusWrapper(
	logger core.Logger,
	backend core.Backend,
	transport core.Transport,
) *IBFTConsensusWrapper {
	return &IBFTConsensusWrapper{
		IBFT: core.NewIBFT(logger, backend, transport),
	}
}

// runSequence starts the underlying consensus mechanism for the given height.
// It may be called by a single thread at any given time
func (c *IBFTConsensusWrapper) runSequence(height uint64) <-chan struct{} {
	var ctx context.Context

	c.sequenceDone = make(chan struct{})
	ctx, c.cancelSequence = context.WithCancel(context.Background())

	go func() {
		c.IBFT.RunSequence(ctx, height)
		c.cancelSequence()
		close(c.sequenceDone)
	}()

	return c.sequenceDone
}

// stopSequence terminates the running IBFT sequence gracefully and waits for it to return
func (c *IBFTConsensusWrapper) stopSequence() {
	c.cancelSequence()
	<-c.sequenceDone // waits until routine inside runSequence finishes
}
