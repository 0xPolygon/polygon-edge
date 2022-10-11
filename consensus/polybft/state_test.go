package polybft

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-memdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func newTestState(t *testing.T) *State {
	t.Helper()

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().Format(time.RFC3339))
	err := os.Mkdir(dir, 0777)

	if err != nil {
		t.Fatal(err)
	}

	state, err := newState(path.Join(dir, "my.db"))
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

	event := newStateSyncEvent(0, ethgo.Address{}, ethgo.Address{}, nil, nil)
	err := state.insertStateSyncEvent(event)
	assert.NoError(t, err)

	events, err := list[*StateSyncEvent](state, syncStateEventsBucket)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestState_Insert_And_Get_MessageVotes(t *testing.T) {
	state := newTestState(t)

	hash := "hash"
	_, err := state.insertMessageVote(1, hash, &MessageSignature{
		From:      "NODE_1",
		Signature: []byte{1, 2},
	})

	assert.NoError(t, err)

	votes, err := state.getMessageVotes(hash)
	assert.NoError(t, err)
	assert.Equal(t, hash, votes.Hash)
	assert.Equal(t, uint64(1), votes.Epoch)
	assert.Equal(t, 1, len(votes.Signatures))
	assert.Equal(t, pbft.NodeID("NODE_1"), votes.Signatures[0].From)
	assert.True(t, bytes.Equal([]byte{1, 2}, votes.Signatures[0].Signature))
}

func TestState_InsertAndGetVotesForDifferentCommitment(t *testing.T) {
	state := newTestState(t)

	commitmentMsg, err := createTestCommitmentMessage(1, 10)
	assert.NoError(t, err)

	hash := commitmentMsg.Message.Hash()

	assert.NoError(t, err)

	_, err = state.insertMessageVote(0, hash.String(), &MessageSignature{
		From:      "NODE_1",
		Signature: []byte{1, 2},
	})
	assert.NoError(t, err)

	commitmentMsg, err = createTestCommitmentMessage(11, 10)
	assert.NoError(t, err)

	hash = commitmentMsg.Message.Hash()

	assert.NoError(t, err)

	_, err = state.insertMessageVote(2, hash.String(), &MessageSignature{
		From:      "NODE_1",
		Signature: []byte{2, 3},
	})
	assert.NoError(t, err)
	_, err = state.insertMessageVote(2, hash.String(), &MessageSignature{
		From:      "NODE_2",
		Signature: []byte{4, 5},
	})
	assert.NoError(t, err)

	votes, err := state.getMessageVotes(hash.String())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(votes.Signatures))
}

func TestState_InsertVoteConcurrent(t *testing.T) {
	state := newTestState(t)

	epoch := uint64(1)
	hash := "hash"

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

	votes, err := state.getMessageVotes(hash)
	assert.NoError(t, err)
	assert.Len(t, votes.Signatures, 100)
}

func TestState_Insert_And_Cleanup(t *testing.T) {
	state := newTestState(t)

	for i := uint64(1); i < 1001; i++ {
		hash := fmt.Sprintf("hash_%v", i)
		_, _ = state.insertMessageVote(i, hash, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		})
	}

	// BucketN returns number of all buckets inside root bucket (including nested buckets) + the root itself
	// Since we inserted 1000 message votes for 1000 epochs we expect to have 1000 buckets
	assert.Equal(t, 1000, state.bucketStats(messageVotesBucket).KeyN)

	err := state.cleanPreviousEpochsDataFromDB(1001)
	assert.NoError(t, err)

	assert.Equal(t, 0, state.bucketStats(messageVotesBucket).BucketN-1)

	// there should be no votes for given epoch since we cleaned the db
	votes, err := state.getMessageVotes("hash_1000")
	assert.NoError(t, err)
	assert.Nil(t, votes)

	for i := uint64(1001); i < 2001; i++ {
		hash := fmt.Sprintf("hash_%v", i)
		_, _ = state.insertMessageVote(i, hash, &MessageSignature{
			From:      "NODE_1",
			Signature: []byte{1, 2},
		})
	}

	assert.Equal(t, 1000, state.bucketStats(messageVotesBucket).KeyN)

	votes, err = state.getMessageVotes("hash_2000")
	assert.NoError(t, err)
	assert.Len(t, votes.Signatures, 1)
}

func TestState_insertAndGetValidatorSnapshot(t *testing.T) {
	const (
		epoch           = uint64(1)
		numOfValidators = 3
	)

	state := newTestState(t)

	validators := newTestValidators(numOfValidators).getPublicIdentities()

	assert.NoError(t, state.insertValidatorSnapshot(epoch, validators))

	snapshotFromDB, err := state.getValidatorSnapshot(epoch)
	assert.NoError(t, err)
	assert.Equal(t, numOfValidators, snapshotFromDB.Len())

	for i, v := range validators {
		assert.Equal(t, v.Address, snapshotFromDB[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB[i].BlsKey)
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

	snapshotFromDB, err := state.getValidatorSnapshot(epoch)

	assert.NoError(t, err)
	assert.Equal(t, snapshot.Len(), snapshotFromDB.Len())

	for i, v := range snapshot {
		assert.Equal(t, v.Address, snapshotFromDB[i].Address)
		assert.Equal(t, v.BlsKey, snapshotFromDB[i].BlsKey)
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

func TestState_getStateSyncEventsForCommitment_NegativeCases(t *testing.T) {
	state := newTestState(t)

	for i := 0; i < stateSyncMainBundleSize; i++ {
		assert.NoError(t, state.insertStateSyncEvent(&StateSyncEvent{
			ID:   uint64(i),
			Data: []byte{1, 2},
		}))
	}

	_, err := state.getStateSyncEventsForCommitment(0, stateSyncMainBundleSize)
	assert.ErrorIs(t, err, errNotEnoughStateSyncs)

	t.Run("Get state sync events - not enough events", func(t *testing.T) {
		_, err := state.getStateSyncEventsForCommitment(0, stateSyncMainBundleSize+1)
		assert.ErrorIs(t, err, errNotEnoughStateSyncs)
	})

	t.Run("Get state sync events - there is a gap in events", func(t *testing.T) {
		assert.NoError(t, state.insertStateSyncEvent(&StateSyncEvent{
			ID:   uint64(stateSyncMainBundleSize + 2),
			Data: []byte{1, 2},
		}))
		_, err := state.getStateSyncEventsForCommitment(0, stateSyncMainBundleSize+2)
		assert.ErrorIs(t, err, errGapInStateSyncs)
	})
}

func TestState_getStateSyncEventsForCommitment(t *testing.T) {
	state := newTestState(t)

	for i := 1; i <= stateSyncMainBundleSize; i++ {
		assert.NoError(t, state.insertStateSyncEvent(&StateSyncEvent{
			ID:   uint64(i),
			Data: []byte{1, 2},
		}))
	}

	events, err := state.getStateSyncEventsForCommitment(1, stateSyncMainBundleSize)
	assert.NoError(t, err)
	assert.Equal(t, stateSyncMainBundleSize, len(events))
}

func TestState_insertCommitmentMessage(t *testing.T) {
	commitment, err := createTestCommitmentMessage(11, 10)
	require.NoError(t, err)

	commitmentToExecute := &CommitmentToExecute{
		SignedCommitment: commitment,
		ToIndex:          10,
	}

	state := newTestState(t)
	assert.NoError(t, state.insertCommitmentMessage(commitmentToExecute))

	commitmentToExecuteFromDB, err := list[*CommitmentToExecute](state, commitmentsBucket)

	assert.NoError(t, err)
	assert.NotNil(t, commitmentToExecuteFromDB)
	assert.Len(t, commitmentToExecuteFromDB, 1)
	assert.Equal(t, commitmentToExecute, commitmentToExecuteFromDB[0])
}

func TestState_getNonExecutedCommitments(t *testing.T) {
	const (
		numberOfCommitments    = 10
		lastExecutedCommitment = 80
		bundleSize             = 10
	)

	state := newTestState(t)

	for i := uint64(0); i < numberOfCommitments; i++ {
		commitment, err := createTestCommitmentMessage(i*bundleSize, bundleSize)
		require.NoError(t, err)

		commitmentToExecute := &CommitmentToExecute{
			SignedCommitment: commitment,
			ToIndex:          commitment.Message.ToIndex,
		}
		require.NoError(t, state.insertCommitmentMessage(commitmentToExecute))
	}

	t.Run("Get non executed commitments", func(t *testing.T) {
		commitmentsNotExecuted, err := state.getNonExecutedCommitments(lastExecutedCommitment)
		assert.NoError(t, err)
		assert.Len(t, commitmentsNotExecuted, 2)
	})

	t.Run("Get non executed commitments - nothing to execute", func(t *testing.T) {
		commitmentsNotExecuted, err := state.getNonExecutedCommitments(lastExecutedCommitment + lastExecutedCommitment)
		assert.NoError(t, err)
		assert.Len(t, commitmentsNotExecuted, 0)
	})
}

func TestState_cleanCommitments(t *testing.T) {
	const (
		numberOfStateSyncs = 10
		numberOfBundles    = 10
	)

	lastCommitmentToIndex := uint64(numberOfStateSyncs*stateSyncMainBundleSize - stateSyncMainBundleSize - 1)

	state := newTestState(t)
	insertTestCommitments(t, state, numberOfStateSyncs)

	assert.NoError(t, state.cleanCommitments(lastCommitmentToIndex))

	commitmentToExecute, err := getFromMemDB[*CommitmentToExecute](state.memdb, commitmentTable, "id", lastCommitmentToIndex)
	assert.NoError(t, err)
	assert.Equal(t, lastCommitmentToIndex, commitmentToExecute.ToIndex)
}

func insertTestCommitments(t *testing.T, state *State, numberOfCommitments uint64) {
	t.Helper()

	for i := uint64(1); i <= numberOfCommitments; i++ {
		commitment, err := createTestCommitmentMessage(i*stateSyncMainBundleSize, stateSyncMainBundleSize)
		require.NoError(t, err)

		commitmentToExecute := &CommitmentToExecute{
			SignedCommitment: commitment,
			ToIndex:          commitment.Message.ToIndex,
		}
		require.NoError(t, state.insertCommitmentMessage(commitmentToExecute))
	}
}

func createTestStateSyncs(numberOfEvents, startIndex uint64) []*StateSyncEvent {
	stateSyncEvents := make([]*StateSyncEvent, 0)
	for i := startIndex; i < numberOfEvents+startIndex; i++ {
		stateSyncEvents = append(stateSyncEvents, &StateSyncEvent{
			ID:     i,
			Sender: ethgo.ZeroAddress,
			Target: ethgo.ZeroAddress,
			Data:   []byte{0, 1},
		})
	}

	return stateSyncEvents
}

func createTestCommitmentMessage(fromIndex, bundleSize uint64) (*CommitmentMessageSigned, error) {
	// TODO - uncomment once changes from the integration with v3-contracts get moved to edge
	// tree, err := NewMerkleTree([][]byte{
	//      {0, 1},
	//      {2, 3},
	//      {4, 5},
	// })
	// if err != nil {
	//      return nil, err
	// }
	msg := &CommitmentMessage{
		// MerkleRootHash: tree.Hash(),
		MerkleRootHash: types.EmptyRootHash,
		FromIndex:      fromIndex,
		ToIndex:        fromIndex + bundleSize - 1,
		BundleSize:     bundleSize,
	}

	return &CommitmentMessageSigned{
		Message:      msg,
		AggSignature: Signature{},
	}, nil
}

func TestMemdb_InsertGetDeleteStateSyncEvents(t *testing.T) {
	const stateSyncCount = 10

	memdb, err := memdb.NewMemDB(memStateSchema)
	require.NoError(t, err)

	stateSyncs := generateStateSyncEvents(t, stateSyncCount, 0)

	for _, sse := range stateSyncs {
		require.NoError(t, insertToMemDB(memdb, stateSyncTable, sse))
	}

	t.Run("Get all state sync events", func(t *testing.T) {
		events, err := getFilteredFromMemDB[*StateSyncEvent](memdb, stateSyncTable, uint64(0), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, events, stateSyncCount)
	})

	t.Run("Get state sync events from beginning to middle", func(t *testing.T) {
		events, err := getFilteredFromMemDB[*StateSyncEvent](memdb, stateSyncTable, uint64(0), uint64(4))

		require.NoError(t, err)
		require.Len(t, events, 5)
	})

	t.Run("Get state sync events from middle to end", func(t *testing.T) {
		events, err := getFilteredFromMemDB[*StateSyncEvent](memdb, stateSyncTable, uint64(5), uint64(stateSyncCount)-1)

		require.NoError(t, err)
		require.Len(t, events, 5)
	})

	t.Run("Get state sync events from an non existing id", func(t *testing.T) {
		events, err := getFilteredFromMemDB[*StateSyncEvent](memdb, stateSyncTable, uint64(stateSyncCount+1), math.MaxUint64)

		require.NoError(t, err)
		require.Len(t, events, 0)
	})

	t.Run("Delete state sync events lower or equal to index 4", func(t *testing.T) {
		err := deleteFilteredFromMemDB(memdb, stateSyncTable, uint64(4))
		require.NoError(t, err)
		events, err := getFilteredFromMemDB[*StateSyncEvent](memdb, stateSyncTable, uint64(0), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, events, 5)
	})
}

func TestMemdb_InsertGetDeleteMessageVotes(t *testing.T) {
	const (
		epochsCount     = 5
		signaturesCount = 10
	)

	memdb, err := memdb.NewMemDB(memStateSchema)
	require.NoError(t, err)

	messageVotes := make([]*MessageVotes, epochsCount)

	for i := 0; i < epochsCount; i++ {
		signatures := make([]*MessageSignature, signaturesCount)
		for j := 0; j < signaturesCount; j++ {
			signatures[j] = &MessageSignature{
				From:      pbft.NodeID(fmt.Sprint(j)),
				Signature: []byte{0, 1},
			}
		}

		messageVotes[i] = &MessageVotes{
			Epoch:      uint64(i),
			Hash:       fmt.Sprint(i),
			Signatures: signatures,
		}
	}

	for _, vote := range messageVotes {
		require.NoError(t, insertToMemDB(memdb, messageVoteTable, vote))
	}

	t.Run("Get votes from all epochs", func(t *testing.T) {
		votes, err := getFilteredFromMemDB[*MessageVotes](memdb, messageVoteTable, uint64(0), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, votes, epochsCount)
	})

	t.Run("Get votes from first to middle epoch", func(t *testing.T) {
		votes, err := getFilteredFromMemDB[*MessageVotes](memdb, messageVoteTable, uint64(0), uint64(2))
		require.NoError(t, err)
		require.Len(t, votes, 3)
	})

	t.Run("Get vote based on hash", func(t *testing.T) {
		vote, err := getFromMemDB[*MessageVotes](memdb, messageVoteTable, "hash", "1")
		require.NoError(t, err)
		require.NotNil(t, vote)
		require.Equal(t, vote.Hash, "1")
	})

	t.Run("Get vote based on non-existing hash", func(t *testing.T) {
		vote, err := getFromMemDB[*MessageVotes](memdb, messageVoteTable, "hash", "1111")
		require.NoError(t, err)
		require.Nil(t, vote)
	})

	t.Run("Delete votes that are either from epoch 2 or previous epochs", func(t *testing.T) {
		err := deleteFilteredFromMemDB(memdb, messageVoteTable, uint64(2))
		require.NoError(t, err)
		votes, err := getFilteredFromMemDB[*MessageVotes](memdb, messageVoteTable, uint64(0), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, votes, 2)
	})
}

func TestMemdb_InsertGetDeleteValidatorSnapshots(t *testing.T) {
	const epochsCount = 5

	memdb, err := memdb.NewMemDB(memStateSchema)
	require.NoError(t, err)

	for i := 0; i < epochsCount; i++ {
		validators := newTestValidators(5)

		require.NoError(t, insertToMemDB(memdb, validatorSnapshotTable, &ValidatorSnapshot{
			Epoch:      uint64(i),
			AccountSet: validators.toValidatorSet().validators,
		}))
	}

	t.Run("Get all snapshots", func(t *testing.T) {
		snapshots, err := getFilteredFromMemDB[*ValidatorSnapshot](memdb, validatorSnapshotTable, uint64(0), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, snapshots, epochsCount)
	})

	t.Run("Get snapshots from beginning to middle", func(t *testing.T) {
		snapshots, err := getFilteredFromMemDB[*ValidatorSnapshot](memdb, validatorSnapshotTable, uint64(0), uint64(2))
		require.NoError(t, err)
		require.Len(t, snapshots, 3)
	})

	t.Run("Get snapshots from middle to end", func(t *testing.T) {
		snapshots, err := getFilteredFromMemDB[*ValidatorSnapshot](memdb, validatorSnapshotTable, uint64(3), uint64(epochsCount))
		require.NoError(t, err)
		require.Len(t, snapshots, 2)
	})

	t.Run("Get snapshots from a non existing epoch", func(t *testing.T) {
		snapshots, err := getFilteredFromMemDB[*ValidatorSnapshot](memdb, validatorSnapshotTable, uint64(epochsCount), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, snapshots, 0)
	})

	t.Run("Delete snapshots lower or equal to epoch 2", func(t *testing.T) {
		err := deleteFilteredFromMemDB(memdb, validatorSnapshotTable, uint64(2))
		require.NoError(t, err)
		snapshots, err := getFilteredFromMemDB[*ValidatorSnapshot](memdb, validatorSnapshotTable, uint64(0), math.MaxUint64)
		require.NoError(t, err)
		require.Len(t, snapshots, 2)
	})
}

func TestMemdb_UpdateRecordInMemdb(t *testing.T) {
	memdb, err := memdb.NewMemDB(memStateSchema)
	require.NoError(t, err)

	votes := &MessageVotes{
		Epoch: 1,
		Hash:  "hash",
		Signatures: []*MessageSignature{
			{
				From:      pbft.NodeID("0x1"),
				Signature: []byte{0, 1},
			},
		},
	}

	require.NoError(t, insertToMemDB(memdb, messageVoteTable, votes))

	votes, err = getFromMemDB[*MessageVotes](memdb, messageVoteTable, "id", votes.Epoch)
	require.NoError(t, err)
	require.Len(t, votes.Signatures, 1)

	votes.Signatures = append(votes.Signatures, &MessageSignature{
		From:      "0x2",
		Signature: []byte{2, 3},
	})

	require.NoError(t, insertToMemDB(memdb, messageVoteTable, votes))

	votes, err = getFromMemDB[*MessageVotes](memdb, messageVoteTable, "id", votes.Epoch)
	require.NoError(t, err)
	require.Len(t, votes.Signatures, 2)
}
