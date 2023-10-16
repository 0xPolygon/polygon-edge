package polybft

import (
	"fmt"
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_insertAndGetValidatorSnapshot(t *testing.T) {
	t.Parallel()

	const (
		epoch            = uint64(1)
		epochEndingBlock = uint64(100)
	)

	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)

	require.NoError(t, err)

	snapshot := validator.AccountSet{
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	assert.NoError(t, state.EpochStore.insertValidatorSnapshot(
		&validatorSnapshot{epoch, epochEndingBlock, snapshot}, nil))

	snapshotFromDB, err := state.EpochStore.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Snapshot.Len())
	assert.Equal(t, epoch, snapshotFromDB.Epoch)
	assert.Equal(t, epochEndingBlock, snapshotFromDB.EpochEndingBlock)

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB.Snapshot[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB.Snapshot[i].BlsKey)
	}
}

func TestState_cleanValidatorSnapshotsFromDb(t *testing.T) {
	t.Parallel()

	fixedEpochSize := uint64(10)
	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	snapshot := validator.AccountSet{
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&validator.ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	var epoch uint64
	// add a couple of more snapshots above limit just to make sure we reached it
	for i := 1; i <= validatorSnapshotLimit+2; i++ {
		epoch = uint64(i)
		assert.NoError(t, state.EpochStore.insertValidatorSnapshot(
			&validatorSnapshot{epoch, epoch * fixedEpochSize, snapshot}, nil))
	}

	snapshotFromDB, err := state.EpochStore.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Snapshot.Len())
	assert.Equal(t, epoch, snapshotFromDB.Epoch)
	assert.Equal(t, epoch*fixedEpochSize, snapshotFromDB.EpochEndingBlock)

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB.Snapshot[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB.Snapshot[i].BlsKey)
	}

	assert.NoError(t, state.EpochStore.cleanValidatorSnapshotsFromDB(epoch, nil))

	// test that last (numberOfSnapshotsToLeaveInDb) of snapshots are left in db after cleanup
	validatorSnapshotsBucketStats, err := state.EpochStore.validatorSnapshotsDBStats()
	require.NoError(t, err)

	assert.Equal(t, numberOfSnapshotsToLeaveInDB, validatorSnapshotsBucketStats.KeyN)

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		snapshotFromDB, err = state.EpochStore.getValidatorSnapshot(epoch)
		assert.NoError(t, err)
		assert.NotNil(t, snapshotFromDB)
		epoch--
	}
}

func TestState_InsertVoteConcurrent(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	epoch := uint64(1)
	assert.NoError(t, state.EpochStore.insertEpoch(epoch, nil))

	hash := []byte{1, 2}

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			_, _ = state.StateSyncStore.insertMessageVote(epoch, hash, &MessageSignature{
				From:      fmt.Sprintf("NODE_%d", i),
				Signature: []byte{1, 2},
			}, nil)
		}(i)
	}

	wg.Wait()

	signatures, err := state.StateSyncStore.getMessageVotes(epoch, hash)
	assert.NoError(t, err)
	assert.Len(t, signatures, 100)
}

func TestState_Insert_And_Cleanup(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	hash1 := []byte{1, 2}

	for i := uint64(1); i <= 500; i++ {
		epoch := i
		err := state.EpochStore.insertEpoch(epoch, nil)

		assert.NoError(t, err)

		_, _ = state.StateSyncStore.insertMessageVote(epoch, hash1, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		}, nil)
	}

	stats, err := state.EpochStore.epochsDBStats()
	require.NoError(t, err)

	// BucketN returns number of all buckets inside root bucket (including nested buckets) + the root itself
	// Since we inserted 500 epochs we expect to have 1000 buckets inside epochs root bucket
	// (500 buckets for epochs + each epoch has 1 nested bucket for message votes)
	assert.Equal(t, 1000, stats.BucketN-1)

	assert.NoError(t, state.EpochStore.cleanEpochsFromDB(nil))

	stats, err = state.EpochStore.epochsDBStats()
	require.NoError(t, err)

	assert.Equal(t, 0, stats.BucketN-1)

	// there should be no votes for given epoch since we cleaned the db
	votes, _ := state.StateSyncStore.getMessageVotes(1, hash1)
	assert.Nil(t, votes)

	for i := uint64(501); i <= 1000; i++ {
		epoch := i
		err := state.EpochStore.insertEpoch(epoch, nil)
		assert.NoError(t, err)

		_, _ = state.StateSyncStore.insertMessageVote(epoch, hash1, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		}, nil)
	}

	stats, err = state.EpochStore.epochsDBStats()
	require.NoError(t, err)

	assert.Equal(t, 1000, stats.BucketN-1)

	votes, _ = state.StateSyncStore.getMessageVotes(1000, hash1)
	assert.Equal(t, 1, len(votes))
}

func TestState_getLastSnapshot(t *testing.T) {
	t.Parallel()

	const (
		lastEpoch          = uint64(10)
		fixedEpochSize     = uint64(10)
		numberOfValidators = 3
	)

	state := newTestState(t)

	for i := uint64(1); i <= lastEpoch; i++ {
		keys, err := bls.CreateRandomBlsKeys(numberOfValidators)

		require.NoError(t, err)

		var snapshot validator.AccountSet
		for j := 0; j < numberOfValidators; j++ {
			snapshot = append(snapshot, &validator.ValidatorMetadata{Address: types.BytesToAddress(generateRandomBytes(t)), BlsKey: keys[j].PublicKey()})
		}

		require.NoError(t, state.EpochStore.insertValidatorSnapshot(
			&validatorSnapshot{i, i * fixedEpochSize, snapshot}, nil))
	}

	snapshotFromDB, err := state.EpochStore.getLastSnapshot(nil)

	assert.NoError(t, err)
	assert.Equal(t, numberOfValidators, snapshotFromDB.Snapshot.Len())
	assert.Equal(t, lastEpoch, snapshotFromDB.Epoch)
	assert.Equal(t, lastEpoch*fixedEpochSize, snapshotFromDB.EpochEndingBlock)
}
