package polybft

import (
	"bytes"
	"fmt"
	"math/big"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func newTestState(t *testing.T) *State {
	t.Helper()

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0777)

	if err != nil {
		t.Fatal(err)
	}

	state, err := newState(path.Join(dir, "my.db"), hclog.NewNullLogger(), make(chan struct{}))
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
	t.Parallel()

	state := newTestState(t)
	event1 := &contractsapi.StateSyncedEvent{
		ID:       big.NewInt(0),
		Sender:   types.Address{},
		Receiver: types.Address{},
		Data:     []byte{},
	}

	err := state.insertStateSyncEvent(event1)
	assert.NoError(t, err)

	events, err := state.list()
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestState_Insert_And_Get_MessageVotes(t *testing.T) {
	t.Parallel()

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
	assert.Equal(t, "NODE_1", votes[0].From)
	assert.True(t, bytes.Equal([]byte{1, 2}, votes[0].Signature))
}

func TestState_InsertVoteConcurrent(t *testing.T) {
	t.Parallel()

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
				From:      fmt.Sprintf("NODE_%d", i),
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
	t.Parallel()

	state := newTestState(t)
	hash1 := []byte{1, 2}

	for i := uint64(1); i <= 500; i++ {
		epoch := i
		err := state.insertEpoch(epoch)

		assert.NoError(t, err)

		_, _ = state.insertMessageVote(epoch, hash1, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		})
	}

	// BucketN returns number of all buckets inside root bucket (including nested buckets) + the root itself
	// Since we inserted 500 epochs we expect to have 1000 buckets inside epochs root bucket
	// (500 buckets for epochs + each epoch has 1 nested bucket for message votes)
	assert.Equal(t, 1000, state.epochsDBStats().BucketN-1)

	err := state.cleanEpochsFromDB()
	assert.NoError(t, err)

	assert.Equal(t, 0, state.epochsDBStats().BucketN-1)

	// there should be no votes for given epoch since we cleaned the db
	votes, _ := state.getMessageVotes(1, hash1)
	assert.Nil(t, votes)

	for i := uint64(501); i <= 1000; i++ {
		epoch := i
		err := state.insertEpoch(epoch)
		assert.NoError(t, err)

		_, _ = state.insertMessageVote(epoch, hash1, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		})
	}

	assert.Equal(t, 1000, state.epochsDBStats().BucketN-1)

	votes, _ = state.getMessageVotes(1000, hash1)
	assert.Equal(t, 1, len(votes))
}

func TestState_insertAndGetValidatorSnapshot(t *testing.T) {
	t.Parallel()

	const (
		epoch            = uint64(1)
		epochEndingBlock = uint64(100)
	)

	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)

	require.NoError(t, err)

	snapshot := AccountSet{
		&ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	assert.NoError(t, state.insertValidatorSnapshot(&validatorSnapshot{epoch, epochEndingBlock, snapshot}))

	snapshotFromDB, err := state.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Snapshot.Len())
	assert.Equal(t, epoch, snapshotFromDB.Epoch)
	assert.Equal(t, epochEndingBlock, snapshotFromDB.EpochEndingBlock)

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB.Snapshot[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB.Snapshot[i].BlsKey)
	}
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

		var snapshot AccountSet
		for j := 0; j < numberOfValidators; j++ {
			snapshot = append(snapshot, &ValidatorMetadata{Address: types.BytesToAddress(generateRandomBytes(t)), BlsKey: keys[j].PublicKey()})
		}

		require.NoError(t, state.insertValidatorSnapshot(&validatorSnapshot{i, i * fixedEpochSize, snapshot}))
	}

	snapshotFromDB, err := state.getLastSnapshot()

	assert.NoError(t, err)
	assert.Equal(t, numberOfValidators, snapshotFromDB.Snapshot.Len())
	assert.Equal(t, lastEpoch, snapshotFromDB.Epoch)
	assert.Equal(t, lastEpoch*fixedEpochSize, snapshotFromDB.EpochEndingBlock)
}

func TestState_cleanValidatorSnapshotsFromDb(t *testing.T) {
	t.Parallel()

	fixedEpochSize := uint64(10)
	state := newTestState(t)
	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	snapshot := AccountSet{
		&ValidatorMetadata{Address: types.BytesToAddress([]byte{0x18}), BlsKey: keys[0].PublicKey()},
		&ValidatorMetadata{Address: types.BytesToAddress([]byte{0x23}), BlsKey: keys[1].PublicKey()},
		&ValidatorMetadata{Address: types.BytesToAddress([]byte{0x37}), BlsKey: keys[2].PublicKey()},
	}

	var epoch uint64
	// add a couple of more snapshots above limit just to make sure we reached it
	for i := 1; i <= validatorSnapshotLimit+2; i++ {
		epoch = uint64(i)
		assert.NoError(t, state.insertValidatorSnapshot(&validatorSnapshot{epoch, epoch * fixedEpochSize, snapshot}))
	}

	snapshotFromDB, err := state.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Snapshot.Len())
	assert.Equal(t, epoch, snapshotFromDB.Epoch)
	assert.Equal(t, epoch*fixedEpochSize, snapshotFromDB.EpochEndingBlock)

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB.Snapshot[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB.Snapshot[i].BlsKey)
	}

	assert.NoError(t, state.cleanValidatorSnapshotsFromDB(epoch))

	// test that last (numberOfSnapshotsToLeaveInDb) of snapshots are left in db after cleanup
	validatorSnapshotsBucketStats := state.validatorSnapshotsDBStats()

	assert.Equal(t, numberOfSnapshotsToLeaveInDB, validatorSnapshotsBucketStats.KeyN)

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		snapshotFromDB, err = state.getValidatorSnapshot(epoch)
		assert.NoError(t, err)
		assert.NotNil(t, snapshotFromDB)
		epoch--
	}
}

func TestState_getStateSyncEventsForCommitment_NotEnoughEvents(t *testing.T) {
	t.Parallel()

	state := newTestState(t)

	for i := 0; i < maxCommitmentSize-2; i++ {
		assert.NoError(t, state.insertStateSyncEvent(&contractsapi.StateSyncedEvent{
			ID:   big.NewInt(int64(i)),
			Data: []byte{1, 2},
		}))
	}

	_, err := state.getStateSyncEventsForCommitment(0, maxCommitmentSize-1)
	assert.ErrorIs(t, err, errNotEnoughStateSyncs)
}

func TestState_getStateSyncEventsForCommitment(t *testing.T) {
	t.Parallel()

	state := newTestState(t)

	for i := 0; i < maxCommitmentSize; i++ {
		assert.NoError(t, state.insertStateSyncEvent(&contractsapi.StateSyncedEvent{
			ID:   big.NewInt(int64(i)),
			Data: []byte{1, 2},
		}))
	}

	t.Run("Return all - forced. Enough events", func(t *testing.T) {
		t.Parallel()

		events, err := state.getStateSyncEventsForCommitment(0, maxCommitmentSize-1)
		require.NoError(t, err)
		require.Equal(t, maxCommitmentSize, len(events))
	})

	t.Run("Return all - forced. Not enough events", func(t *testing.T) {
		t.Parallel()

		_, err := state.getStateSyncEventsForCommitment(0, maxCommitmentSize+1)
		require.ErrorIs(t, err, errNotEnoughStateSyncs)
	})

	t.Run("Return all you can. Enough events", func(t *testing.T) {
		t.Parallel()

		events, err := state.getStateSyncEventsForCommitment(0, maxCommitmentSize-1)
		assert.NoError(t, err)
		assert.Equal(t, maxCommitmentSize, len(events))
	})

	t.Run("Return all you can. Not enough events", func(t *testing.T) {
		t.Parallel()

		events, err := state.getStateSyncEventsForCommitment(0, maxCommitmentSize+1)
		assert.ErrorIs(t, err, errNotEnoughStateSyncs)
		assert.Equal(t, maxCommitmentSize, len(events))
	})
}

func TestState_insertCommitmentMessage(t *testing.T) {
	t.Parallel()

	commitment := createTestCommitmentMessage(t, 11)

	state := newTestState(t)
	assert.NoError(t, state.insertCommitmentMessage(commitment))

	commitmentFromDB, err := state.getCommitmentMessage(commitment.Message.EndID.Uint64())

	assert.NoError(t, err)
	assert.NotNil(t, commitmentFromDB)
	assert.Equal(t, commitment, commitmentFromDB)
}

func TestState_StateSync_insertAndGetStateSyncProof(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	commitment := createTestCommitmentMessage(t, 0)
	require.NoError(t, state.insertCommitmentMessage(commitment))

	insertTestStateSyncProofs(t, state, 10)

	proofFromDB, err := state.getStateSyncProof(1)

	assert.NoError(t, err)
	assert.Equal(t, uint64(1), proofFromDB.StateSync.ID.Uint64())
	assert.NotNil(t, proofFromDB.Proof)
}

func TestState_Insert_And_Get_ExitEvents_PerEpoch(t *testing.T) {
	const (
		numOfEpochs         = 11
		numOfBlocksPerEpoch = 10
		numOfEventsPerBlock = 11
	)

	state := newTestState(t)
	insertTestExitEvents(t, state, numOfEpochs, numOfBlocksPerEpoch, numOfEventsPerBlock)

	t.Run("Get events for existing epoch", func(t *testing.T) {
		events, err := state.getExitEventsByEpoch(1)

		assert.NoError(t, err)
		assert.Len(t, events, numOfBlocksPerEpoch*numOfEventsPerBlock)
	})

	t.Run("Get events for non-existing epoch", func(t *testing.T) {
		events, err := state.getExitEventsByEpoch(12)

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
		events, err := state.getExitEventsForProof(c.epoch, c.checkpointBlockNumber)

		assert.NoError(t, err)
		assert.Len(t, events, c.expectedNumberOfEvents)
	}
}

func TestState_Insert_And_Get_ExitEvents_ForProof_NoEvents(t *testing.T) {
	state := newTestState(t)
	insertTestExitEvents(t, state, 1, 10, 1)

	events, err := state.getExitEventsForProof(2, 11)

	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestState_decodeExitEvent(t *testing.T) {
	t.Parallel()

	const (
		exitID      = 1
		epoch       = 1
		blockNumber = 10
	)

	state := newTestState(t)

	topics := make([]ethgo.Hash, 4)
	topics[0] = exitEventABI.ID()
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
	require.Equal(t, uint64(exitID), event.ID)
	require.Equal(t, uint64(epoch), event.EpochNumber)
	require.Equal(t, uint64(blockNumber), event.BlockNumber)

	require.NoError(t, state.insertExitEvent(event))
}

func TestState_decodeExitEvent_NotAnExitEvent(t *testing.T) {
	t.Parallel()

	topics := make([]ethgo.Hash, 4)
	topics[0] = stateTransferEventABI.ID()

	log := &ethgo.Log{
		Address: ethgo.ZeroAddress,
		Topics:  topics,
	}

	event, err := decodeExitEvent(log, 1, 1)
	require.NoError(t, err)
	require.Nil(t, event)
}

func TestState_getProposerSnapshot_writeProposerSnapshot(t *testing.T) {
	t.Parallel()

	const (
		height = uint64(100)
		round  = uint64(5)
	)

	state := newTestState(t)

	snap, err := state.getProposerSnapshot()
	require.NoError(t, err)
	require.Nil(t, snap)

	newSnapshot := &ProposerSnapshot{Height: height, Round: round}
	require.NoError(t, state.writeProposerSnapshot(newSnapshot))

	snap, err = state.getProposerSnapshot()
	require.NoError(t, err)
	require.Equal(t, newSnapshot, snap)
}

func insertTestExitEvents(t *testing.T, state *State,
	numOfEpochs, numOfBlocksPerEpoch, numOfEventsPerBlock int) {
	t.Helper()

	index, block := uint64(1), uint64(1)

	for i := uint64(1); i <= uint64(numOfEpochs); i++ {
		for j := 1; j <= numOfBlocksPerEpoch; j++ {
			for k := 1; k <= numOfEventsPerBlock; k++ {
				event := &ExitEvent{index, ethgo.HexToAddress("0x101"), ethgo.HexToAddress("0x102"), []byte{11, 22}, i, block}
				assert.NoError(t, state.insertExitEvent(event))

				index++
			}
			block++
		}
	}
}

func TestState_getCommitmentForStateSync(t *testing.T) {
	const (
		numOfCommitments = 10
	)

	state := newTestState(t)

	insertTestCommitments(t, state, numOfCommitments)

	var cases = []struct {
		stateSyncID   uint64
		hasCommitment bool
	}{
		{1, true},
		{10, true},
		{11, true},
		{7, true},
		{999, false},
		{121, false},
		{99, true},
		{101, true},
		{111, false},
		{75, true},
		{5, true},
		{102, true},
		{211, false},
		{21, true},
		{30, true},
		{81, true},
		{90, true},
	}

	for _, c := range cases {
		commitment, err := state.getCommitmentForStateSync(c.stateSyncID)

		if c.hasCommitment {
			require.NoError(t, err, fmt.Sprintf("state sync %v", c.stateSyncID))
			require.Equal(t, c.hasCommitment, commitment.ContainsStateSync(c.stateSyncID))
		} else {
			require.ErrorIs(t, errNoCommitmentForStateSync, err)
		}
	}
}

func insertTestCommitments(t *testing.T, state *State, numberOfCommitments uint64) {
	t.Helper()

	for i := uint64(0); i <= numberOfCommitments; i++ {
		commitment := createTestCommitmentMessage(t, i*maxCommitmentSize+1)
		require.NoError(t, state.insertCommitmentMessage(commitment))
	}
}

func insertTestStateSyncProofs(t *testing.T, state *State, numberOfProofs int64) {
	t.Helper()

	ssProofs := make([]*StateSyncProof, numberOfProofs)

	for i := int64(0); i < numberOfProofs; i++ {
		proofs := &StateSyncProof{
			Proof:     []types.Hash{types.BytesToHash(generateRandomBytes(t))},
			StateSync: createTestStateSync(i),
		}
		ssProofs[i] = proofs
	}

	require.NoError(t, state.insertStateSyncProofs(ssProofs))
}

func createTestStateSync(index int64) *contractsapi.StateSyncedEvent {
	return &contractsapi.StateSyncedEvent{
		ID:       big.NewInt(index),
		Sender:   types.ZeroAddress,
		Receiver: types.ZeroAddress,
		Data:     []byte{0, 1},
	}
}

func createTestCommitmentMessage(t *testing.T, fromIndex uint64) *CommitmentMessageSigned {
	t.Helper()

	tree, err := NewMerkleTree([][]byte{
		{0, 1},
		{2, 3},
		{4, 5},
	})

	require.NoError(t, err)

	msg := &contractsapi.StateSyncCommitment{
		Root:    tree.Hash(),
		StartID: big.NewInt(int64(fromIndex)),
		EndID:   big.NewInt(int64(fromIndex + maxCommitmentSize - 1)),
	}

	return &CommitmentMessageSigned{
		Message:      msg,
		AggSignature: Signature{},
	}
}
