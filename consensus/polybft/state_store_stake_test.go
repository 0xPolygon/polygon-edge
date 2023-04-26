package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Insert_And_Get_TransferEvents_PerEpoch(t *testing.T) {
	t.Parallel()

	const (
		numOfEpochs         = 11
		numOfBlocksPerEpoch = 10
	)

	state := newTestState(t)
	insertTestTransferEvents(t, state, numOfEpochs, numOfBlocksPerEpoch)

	t.Run("Get events for existing epoch", func(t *testing.T) {
		t.Parallel()

		events, err := state.StakeStore.getTransferEvents(1)

		require.NoError(t, err)
		require.Len(t, events, numOfBlocksPerEpoch)
	})

	t.Run("Get events for non-existing epoch", func(t *testing.T) {
		t.Parallel()

		events, err := state.StakeStore.getTransferEvents(12)

		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})
}

func insertTestTransferEvents(t *testing.T, state *State,
	numOfEpochs, numOfEventsPerEpoch int) []*contractsapi.TransferEvent {
	t.Helper()

	var (
		index = uint64(0)
	)

	allEvents := make([]*contractsapi.TransferEvent, 0)

	for i := uint64(1); i <= uint64(numOfEpochs); i++ {
		transferEvents := make([]*contractsapi.TransferEvent, numOfEventsPerEpoch)
		for j := 1; j <= numOfEventsPerEpoch; j++ {
			transferEvents[j-1] =
				&contractsapi.TransferEvent{
					From:  types.ZeroAddress,
					To:    types.BytesToAddress([]byte{0, 1}),
					Value: big.NewInt(1),
				}
			index++
		}

		require.NoError(t, state.StakeStore.insertTransferEvents(i, transferEvents))

		allEvents = append(allEvents, transferEvents...)
	}

	return allEvents
}
