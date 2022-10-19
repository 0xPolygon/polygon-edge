package polybft

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) *State {
	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().Format(time.RFC3339))
	err := os.Mkdir(dir, 0777)
	if err != nil {
		t.Fatal(err)
	}

	state, err := newState(path.Join(dir, "my.db"), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	})
	return state
}

func TestState_InsertEvent(t *testing.T) {
	state := newTestState(t)

	evnt1 := newStateSyncEvent(0, types.Address{}, types.Address{}, nil, nil)
	err := state.insertStateSyncEvent(evnt1)
	assert.NoError(t, err)

	events, err := state.list()
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestState_Insert_And_Get_MessageVotes(t *testing.T) {
	state := newTestState(t)
	epoch := uint64(1)
	assert.NoError(t, state.insertEpoch(epoch))

	hash := []byte{1, 2}
	_, err := state.insertMessageVote(1, hash, &MessageSignature{
		From:      "NODE_1",
		Signature: []byte{1, 2},
	})

	assert.NoError(t, err)

	votes, err := state.getMessageVotes(epoch, hash)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(votes))
	assert.Equal(t, pbft.NodeID("NODE_1"), votes[0].From)
	assert.True(t, bytes.Equal([]byte{1, 2}, votes[0].Signature))
}

func TestState_InsertVoteConcurrent(t *testing.T) {
	state := newTestState(t)

	epoch := uint64(1)
	assert.NoError(t, state.insertEpoch(epoch))
	hash := []byte{1, 2}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			_, _ = state.insertMessageVote(epoch, hash, &MessageSignature{
				From:      pbft.NodeID(fmt.Sprintf("NODE_%d", i)),
				Signature: []byte{1, 2},
			})
		}(i)
	}

	wg.Wait()

	signatures, err := state.getMessageVotes(epoch, hash)
	assert.NoError(t, err)
	assert.Len(t, signatures, 100)
}

func TestState_Insert_And_Cleanup(t *testing.T) {
	state := newTestState(t)
	hash1 := []byte{1, 2}
	for i := uint64(1); i < 1001; i++ {
		epoch := i
		err := state.insertEpoch(epoch)
		assert.NoError(t, err)
		_, _ = state.insertMessageVote(epoch, hash1, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		})
	}

	// BucketN returns number of all buckets inside root bucket (including nested buckets) + the root itself
	// Since we inserted 1000 epochs we expect to have 2000 buckets inside epochs root bucket
	// (1000 buckets for epochs + each epoch has 1 nested bucket for message votes)
	assert.Equal(t, 2000, state.epochsDBStats().BucketN-1)

	err := state.cleanEpochsFromDB()
	assert.NoError(t, err)

	assert.Equal(t, 0, state.epochsDBStats().BucketN-1)

	// there should be no votes for given epoch since we cleaned the db
	votes, _ := state.getMessageVotes(1, hash1)
	assert.Nil(t, votes)

	for i := uint64(1001); i < 2001; i++ {
		epoch := i
		err := state.insertEpoch(epoch)
		assert.NoError(t, err)

		_, _ = state.insertMessageVote(epoch, hash1, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		})
	}

	assert.Equal(t, 2000, state.epochsDBStats().BucketN-1)

	votes, _ = state.getMessageVotes(2000, hash1)
	assert.Equal(t, 1, len(votes))
}

func TestState_insertAndGetValidatorSnapshot(t *testing.T) {
	epoch := uint64(1)
	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)
	snapshot := AccountSet{
		&ValidatorAccount{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&ValidatorAccount{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&ValidatorAccount{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	assert.NoError(t, state.insertValidatorSnapshot(epoch, snapshot))

	snapshotFromDb, err := state.getValidatorSnapshot(epoch)
	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDb.Len())

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDb[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDb[i].BlsKey)
	}
}

func TestState_cleanValidatorSnapshotsFromDb(t *testing.T) {
	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)
	snapshot := AccountSet{
		&ValidatorAccount{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&ValidatorAccount{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&ValidatorAccount{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	var epoch uint64
	// add a couple of more snapshots above limit just to make sure we reached it
	for i := 1; i <= validatorSnapshotLimit+2; i++ {
		epoch = uint64(i)
		assert.NoError(t, state.insertValidatorSnapshot(epoch, snapshot))
	}

	snapshotFromDb, err := state.getValidatorSnapshot(epoch)
	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDb.Len())
	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDb[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDb[i].BlsKey)
	}

	assert.NoError(t, state.cleanValidatorSnapshotsFromDB(epoch))

	// test that last (numberOfSnapshotsToLeaveInDb) of snapshots are left in db after cleanup
	validatorSnapshotsBucketStats := state.validatorSnapshotsDBStats()
	assert.Equal(t, numberOfSnapshotsToLeaveInDB, validatorSnapshotsBucketStats.KeyN)
	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		snapshotFromDb, err = state.getValidatorSnapshot(epoch)
		assert.NoError(t, err)
		assert.NotNil(t, snapshotFromDb)
		epoch--
	}
}

func TestState_getStateSyncEventsForCommitment_NotEnoughEvents(t *testing.T) {
	state := newTestState(t)

	for i := 0; i < stateSyncMainBundleSize-2; i++ {
		assert.NoError(t, state.insertStateSyncEvent(&StateSyncEvent{
			ID:   uint64(i),
			Data: []byte{1, 2},
		}))
	}

	_, err := state.getStateSyncEventsForCommitment(0, stateSyncMainBundleSize-1)
	assert.ErrorIs(t, err, ErrNotEnoughStateSyncs)
}

func TestState_getStateSyncEventsForCommitment(t *testing.T) {
	state := newTestState(t)

	for i := 0; i < stateSyncMainBundleSize; i++ {
		assert.NoError(t, state.insertStateSyncEvent(&StateSyncEvent{
			ID:   uint64(i),
			Data: []byte{1, 2},
		}))
	}

	events, err := state.getStateSyncEventsForCommitment(0, stateSyncMainBundleSize-1)
	assert.NoError(t, err)
	assert.Equal(t, stateSyncMainBundleSize, len(events))
}

func TestState_insertCommitmentMessage(t *testing.T) {
	commitment, err := createTestCommitmentMessage(11)
	require.NoError(t, err)

	state := newTestState(t)
	assert.NoError(t, state.insertCommitmentMessage(commitment))

	commitmentFromDb, err := state.getCommitmentMessage(commitment.Message.ToIndex)
	assert.NoError(t, err)
	assert.NotNil(t, commitmentFromDb)
	assert.Equal(t, commitment, commitmentFromDb)
}

func TestState_getNonExecutedCommitments(t *testing.T) {
	const (
		numberOfCommitments    = 10
		lastExecutedCommitment = 80
	)

	state := newTestState(t)

	for i := uint64(0); i < numberOfCommitments; i++ {
		commitment, err := createTestCommitmentMessage(i * stateSyncMainBundleSize)
		require.NoError(t, err)
		require.NoError(t, state.insertCommitmentMessage(commitment))
	}

	commitmentsNotExecuted, err := state.getNonExecutedCommitments(lastExecutedCommitment)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(commitmentsNotExecuted))
}

func TestState_cleanCommitments(t *testing.T) {
	const (
		numberOfCommitments = 10
		numberOfBundles     = 10
	)

	lastCommitmentToIndex := uint64(numberOfCommitments*stateSyncMainBundleSize - stateSyncMainBundleSize - 1)

	state := newTestState(t)
	insertTestCommitments(t, state, 1, numberOfCommitments)
	insertTestBundles(t, state, numberOfBundles)

	assert.NoError(t, state.cleanCommitments(lastCommitmentToIndex))

	commitment, err := state.getCommitmentMessage(lastCommitmentToIndex)
	assert.NoError(t, err)
	assert.Equal(t, lastCommitmentToIndex, commitment.Message.ToIndex)

	for i := uint64(1); i < numberOfCommitments; i++ {
		c, err := state.getCommitmentMessage(i*stateSyncMainBundleSize + lastCommitmentToIndex - 1)
		assert.NoError(t, err)
		assert.Nil(t, c)
	}

	bundles, err := state.getBundles(0, maxBundlesPerSprint)
	assert.NoError(t, err)
	assert.Nil(t, bundles)
}

func TestState_insertAndGetBundles(t *testing.T) {
	const numberOfBundles = 10

	state := newTestState(t)

	commitment, err := createTestCommitmentMessage(0)
	require.NoError(t, err)
	require.NoError(t, state.insertCommitmentMessage(commitment))

	insertTestBundles(t, state, numberOfBundles)

	bundlesFromDb, err := state.getBundles(0, maxBundlesPerSprint)
	assert.NoError(t, err)
	assert.Equal(t, numberOfBundles, len(bundlesFromDb))
	assert.Equal(t, uint64(0), bundlesFromDb[0].ID())
	assert.Equal(t, stateSyncBundleSize, len(bundlesFromDb[0].StateSyncs))
	assert.NotNil(t, bundlesFromDb[0].Proof)
}

func insertTestCommitments(t *testing.T, state *State, epoch, numberOfCommitments uint64) {
	for i := uint64(0); i <= numberOfCommitments; i++ {
		commitment, err := createTestCommitmentMessage(i * stateSyncMainBundleSize)
		require.NoError(t, err)
		require.NoError(t, state.insertCommitmentMessage(commitment))
	}
}

func insertTestBundles(t *testing.T, state *State, numberOfBundles uint64) {
	bundles := make([]*BundleProof, numberOfBundles)
	for i := uint64(0); i < numberOfBundles; i++ {
		bundle := &BundleProof{
			Proof:      []types.Hash{types.BytesToHash(generateRandomBytes(t))},
			StateSyncs: createTestStateSyncs(stateSyncBundleSize, i*stateSyncBundleSize),
		}
		bundles[i] = bundle
	}

	require.NoError(t, state.insertBundles(bundles))
}

func createTestStateSyncs(numberOfEvents, startIndex uint64) []*StateSyncEvent {
	stateSyncEvents := make([]*StateSyncEvent, 0)
	for i := startIndex; i < numberOfEvents+startIndex; i++ {
		stateSyncEvents = append(stateSyncEvents, &StateSyncEvent{
			ID:       i,
			Sender:   types.ZeroAddress,
			Receiver: types.ZeroAddress,
			Data:     []byte{0, 1},
		})
	}
	return stateSyncEvents
}

func createTestCommitmentMessage(fromIndex uint64) (*CommitmentMessageSigned, error) {
	tree, err := NewMerkleTree([][]byte{
		{0, 1},
		{2, 3},
		{4, 5},
	})
	if err != nil {
		return nil, err
	}

	msg := &CommitmentMessage{
		MerkleRootHash: tree.Hash(),
		FromIndex:      fromIndex,
		ToIndex:        fromIndex + stateSyncMainBundleSize - 1,
		BundleSize:     2,
	}

	return &CommitmentMessageSigned{
		Message:      msg,
		AggSignature: Signature{},
	}, nil
}
