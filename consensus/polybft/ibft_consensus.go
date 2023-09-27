package polybft

import (
	"context"

	"github.com/0xPolygon/go-ibft/core"
)

// IBFTConsensusWrapper is a convenience wrapper for the go-ibft package
type IBFTConsensusWrapper struct {
	*core.IBFT
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
// It returns channel which will be closed after c.IBFT.RunSequence is done
// and stopSequence function which can be used to halt c.IBFT.RunSequence routine from outside
func (c *IBFTConsensusWrapper) runSequence(height uint64) (<-chan struct{}, func()) {
	sequenceDone := make(chan struct{})
	ctx, cancelSequence := context.WithCancel(context.Background())

	go func() {
		c.IBFT.RunSequence(ctx, height)
		cancelSequence()
		close(sequenceDone)
	}()

	return sequenceDone, func() {
		// stopSequence terminates the running IBFT sequence gracefully and waits for it to return
		cancelSequence()
		<-sequenceDone // waits until c.IBFT.RunSequenc routine finishes
	}
}
