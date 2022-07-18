package ibft

import (
	"sync"

	"github.com/Trapesys/go-ibft/core"
)

//	Convenience wrapper for the go-ibft package
type consensus struct {
	*core.IBFT

	wg sync.WaitGroup
}

func newIBFT(
	logger core.Logger,
	backend core.Backend,
	transport core.Transport,
) *consensus {
	return &consensus{
		IBFT: core.NewIBFT(logger, backend, transport),
		wg:   sync.WaitGroup{},
	}
}

//	runSequence starts the underlying consensus mechanism for the given height.
//	It may be called by a single thread at any given time
func (c *consensus) runSequence(height uint64) <-chan struct{} {
	done := make(chan struct{})

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer close(done)

		c.RunSequence(height)
	}()

	return done
}

func (c *consensus) cancelSequence() {
	c.CancelSequence()
	c.wg.Wait()
}
