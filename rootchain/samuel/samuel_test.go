package samuel

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/proto"
	"github.com/stretchr/testify/assert"
)

func TestSAMUEL_Start(t *testing.T) {
	var (
		hasSubscribed        = false
		hasRegistered        = false
		startedBlock  uint64 = 0
		eventTracker         = mockEventTracker{
			subscribeFn: func() <-chan rootchain.Event {
				hasSubscribed = true

				ch := make(chan rootchain.Event)
				close(ch)

				return ch
			},
			startFn: func(blockNum uint64) error {
				startedBlock = blockNum
				return nil
			},
		}
		transport = mockTransport{
			subscribeFn: func(f func(sam *proto.SAM)) error {
				hasRegistered = true

				return nil
			},
		}
		storage = mockStorage{}
	)

	// Create the SAMUEL instance
	s := &SAMUEL{
		transport:    transport,
		storage:      storage,
		eventTracker: eventTracker,
	}

	// Make sure there were no errors in starting
	assert.NoError(t, s.Start())

	// Make sure the event subscription is active
	assert.True(t, hasSubscribed)

	// Make sure the gossip handler is registered
	assert.True(t, hasRegistered)

	// Make sure the start block is the latest block
	assert.Equal(t, rootchain.LatestRootchainBlockNumber, startedBlock)
}

func TestSAMUEL_GetStartBlockNumber_Predefined(t *testing.T) {
	var (
		storedBlockNumber uint64 = 100
		storedEventIndex  uint64 = 1
		storage                  = mockStorage{
			readFn: func(_ string) (string, bool) {
				return fmt.Sprintf(
					"%d:%d",
					storedEventIndex,
					storedBlockNumber,
				), true
			},
		}
	)

	s := &SAMUEL{
		storage: storage,
	}

	// Get the start block number
	startBlock, err := s.getStartBlockNumber()

	assert.NoError(t, err)
	assert.Equal(t, storedBlockNumber, startBlock)
}

func TestSAMUEL_GetLatestStartBlock(t *testing.T) {
	s := &SAMUEL{
		storage: mockStorage{},
	}

	// Get the start block number
	startBlock, err := s.getStartBlockNumber()

	assert.NoError(t, err)
	assert.Equal(t, rootchain.LatestRootchainBlockNumber, startBlock)
}
