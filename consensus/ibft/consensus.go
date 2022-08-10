package ibft

import (
	"context"
	"sync"

	"github.com/0xPolygon/go-ibft/core"
)

// IBFTConsensus is a convenience wrapper for the go-ibft package
type IBFTConsensus struct {
	*core.IBFT

	wg sync.WaitGroup

	cancelSequence context.CancelFunc
}

func newIBFT(
	logger core.Logger,
	backend core.Backend,
	transport core.Transport,
) *IBFTConsensus {
	return &IBFTConsensus{
		IBFT: core.NewIBFT(logger, backend, transport),
		wg:   sync.WaitGroup{},
	}
}

// runSequence starts the underlying consensus mechanism for the given height.
// It may be called by a single thread at any given time
func (c *IBFTConsensus) runSequence(height uint64) <-chan struct{} {
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	c.cancelSequence = cancel

	c.wg.Add(1)

	go func() {
		defer func() {
			cancel()
			c.wg.Done()
			close(done)
		}()

		c.RunSequence(ctx, height)
	}()

	return done
}

// stopSequence terminates the running IBFT sequence gracefully and waits for it to return
func (c *IBFTConsensus) stopSequence() {
	c.cancelSequence()
	c.wg.Wait()
}
