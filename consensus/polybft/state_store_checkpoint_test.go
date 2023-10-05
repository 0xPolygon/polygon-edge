package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	bolt "go.etcd.io/bbolt"
)

func TestState_Insert_And_Get_ExitEvents_PerEpoch(t *testing.T) {
	const (
		numOfEpochs         = 11
		numOfBlocksPerEpoch = 10
		numOfEventsPerBlock = 11
	)

	state := newTestState(t)
	insertTestExitEvents(t, state, numOfEpochs, numOfBlocksPerEpoch, numOfEventsPerBlock)

	t.Run("Get events for existing epoch", func(t *testing.T) {
		events, err := state.CheckpointStore.getExitEventsByEpoch(1)

		assert.NoError(t, err)
		assert.Len(t, events, numOfBlocksPerEpoch*numOfEventsPerBlock)
	})

	t.Run("Get events for non-existing epoch", func(t *testing.T) {
		events, err := state.CheckpointStore.getExitEventsByEpoch(12)

		assert.NoError(t, err)
		assert.Len(t, events, 0)
	})
}

func TestState_Insert_And_Get_ExitEvents_ForProof(t *testing.T) {
	const (
		numOfEpochs         = 11
		numOfBlocksPerEpoch = 10
		numOfEventsPerBlock = 10
	)

	state := newTestState(t)
	insertTestExitEvents(t, state, numOfEpochs, numOfBlocksPerEpoch, numOfEventsPerBlock)

	var cases = []struct {
		epoch                  uint64
		checkpointBlockNumber  uint64
		expectedNumberOfEvents int
	}{
		{1, 1, 10},
		{1, 2, 20},
		{1, 8, 80},
		{2, 12, 20},
		{2, 14, 40},
		{3, 26, 60},
		{4, 38, 80},
		{11, 105, 50},
	}

	for _, c := range cases {
		events, err := state.CheckpointStore.getExitEventsForProof(c.epoch, c.checkpointBlockNumber)

		assert.NoError(t, err)
		assert.Len(t, events, c.expectedNumberOfEvents)
	}
}

func TestState_Insert_And_Get_ExitEvents_ForProof_NoEvents(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	insertTestExitEvents(t, state, 1, 10, 1)

	events, err := state.CheckpointStore.getExitEventsForProof(2, 11)

	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestState_NoEpochForExitEventInLookup(t *testing.T) {
	t.Parallel()

	const (
		exitToTest         = uint64(11)
		epochToMatch       = uint64(2)
		blockNumberToMatch = uint64(12)
	)

	state := newTestState(t)
	insertTestExitEvents(t, state, 3, 10, 1)

	exitEventFromDB, err := state.CheckpointStore.getExitEvent(exitToTest)
	require.NoError(t, err)
	require.Equal(t, exitToTest, exitEventFromDB.ID.Uint64())
	require.Equal(t, epochToMatch, exitEventFromDB.EpochNumber)
	require.Equal(t, blockNumberToMatch, exitEventFromDB.BlockNumber)

	// simulate invalid case (for some reason lookup table doesn't have epoch for given exit)
	err = state.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(exitEventToEpochLookupBucket).Delete(common.EncodeUint64ToBytes(exitEventFromDB.ID.Uint64()))
	})

	require.NoError(t, err)

	_, err = state.CheckpointStore.getExitEvent(exitToTest)
	require.ErrorContains(t, err, "epoch was not found in lookup table")
}

func TestState_decodeExitEvent(t *testing.T) {
	t.Parallel()

	const (
		exitID      = 1
		epoch       = 1
		blockNumber = 10
	)

	state := newTestState(t)

	var exitEvent contractsapi.L2StateSyncedEvent

	topics := make([]ethgo.Hash, 4)
	topics[0] = exitEvent.Sig()
	topics[1] = ethgo.BytesToHash([]byte{exitID})
	topics[2] = ethgo.BytesToHash(ethgo.HexToAddress("0x1111").Bytes())
	topics[3] = ethgo.BytesToHash(ethgo.HexToAddress("0x2222").Bytes())
	personType := abi.MustNewType("tuple(string firstName, string lastName)")
	encodedData, err := personType.Encode(map[string]string{"firstName": "John", "lastName": "Doe"})
	require.NoError(t, err)

	log := &ethgo.Log{
		Address: ethgo.ZeroAddress,
		Topics:  topics,
		Data:    encodedData,
	}

	event, err := decodeExitEvent(log, epoch, blockNumber)
	require.NoError(t, err)
	require.Equal(t, uint64(exitID), event.ID.Uint64())
	require.Equal(t, uint64(epoch), event.EpochNumber)
	require.Equal(t, uint64(blockNumber), event.BlockNumber)

	require.NoError(t, state.CheckpointStore.insertExitEvent(event, nil))
}

func TestState_decodeExitEvent_NotAnExitEvent(t *testing.T) {
	t.Parallel()

	var stateSyncedEvent contractsapi.StateSyncedEvent

	topics := make([]ethgo.Hash, 4)
	topics[0] = stateSyncedEvent.Sig()

	log := &ethgo.Log{
		Address: ethgo.ZeroAddress,
		Topics:  topics,
	}

	event, err := decodeExitEvent(log, 1, 1)
	require.NoError(t, err)
	require.Nil(t, event)
}

func insertTestExitEvents(t *testing.T, state *State,
	numOfEpochs, numOfBlocksPerEpoch, numOfEventsPerBlock int) []*ExitEvent {
	t.Helper()

	var (
		index      = uint64(0)
		block      = uint64(1)
		exitEvents = make([]*ExitEvent, numOfEpochs*numOfBlocksPerEpoch*numOfEventsPerBlock)
	)

	for i := uint64(1); i <= uint64(numOfEpochs); i++ {
		for j := 1; j <= numOfBlocksPerEpoch; j++ {
			for k := 1; k <= numOfEventsPerBlock; k++ {
				exitEvents[index] =
					&ExitEvent{
						L2StateSyncedEvent: &contractsapi.L2StateSyncedEvent{
							ID:       new(big.Int).SetUint64(index),
							Sender:   types.ZeroAddress,
							Receiver: types.ZeroAddress,
							Data:     generateRandomBytes(t),
						},
						EpochNumber: i,
						BlockNumber: block,
					}
				index++
			}
			block++
		}
	}

	for _, ee := range exitEvents {
		require.NoError(t, state.CheckpointStore.insertExitEvent(ee, nil))
	}

	return exitEvents
}
