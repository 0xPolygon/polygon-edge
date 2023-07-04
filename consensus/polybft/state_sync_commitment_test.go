package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitmentMessage_Hash(t *testing.T) {
	t.Parallel()

	const (
		eventsCount = 10
	)

	stateSyncEvents := generateStateSyncEvents(t, eventsCount, 0)

	trie1, err := createMerkleTree(stateSyncEvents)
	require.NoError(t, err)

	trie2, err := createMerkleTree(stateSyncEvents[0 : len(stateSyncEvents)-1])
	require.NoError(t, err)

	commitmentMessage1 := newTestCommitmentSigned(t, trie1.Hash(), 2, 8)
	commitmentMessage2 := newTestCommitmentSigned(t, trie1.Hash(), 2, 8)
	commitmentMessage3 := newTestCommitmentSigned(t, trie1.Hash(), 6, 10)
	commitmentMessage4 := newTestCommitmentSigned(t, trie2.Hash(), 2, 8)

	hash1, err := commitmentMessage1.Hash()
	require.NoError(t, err)
	hash2, err := commitmentMessage2.Hash()
	require.NoError(t, err)
	hash3, err := commitmentMessage3.Hash()
	require.NoError(t, err)
	hash4, err := commitmentMessage4.Hash()
	require.NoError(t, err)

	require.Equal(t, hash1, hash2)
	require.NotEqual(t, hash1, hash3)
	require.NotEqual(t, hash1, hash4)
	require.NotEqual(t, hash3, hash4)
}

func TestCommitmentMessage_ToRegisterCommitmentInputData(t *testing.T) {
	t.Parallel()

	const epoch, eventsCount = uint64(100), 11
	pendingCommitment, _, _ := buildCommitmentAndStateSyncs(t, eventsCount, epoch, uint64(2))
	expectedSignedCommitmentMsg := &CommitmentMessageSigned{
		Message: pendingCommitment.StateSyncCommitment,
		AggSignature: Signature{
			Bitmap:              []byte{5, 1},
			AggregatedSignature: []byte{1, 1},
		},
		PublicKeys: [][]byte{{0, 1}, {2, 3}, {4, 5}},
	}
	inputData, err := expectedSignedCommitmentMsg.EncodeAbi()
	require.NoError(t, err)
	require.NotEmpty(t, inputData)

	var actualSignedCommitmentMsg CommitmentMessageSigned

	require.NoError(t, actualSignedCommitmentMsg.DecodeAbi(inputData))
	require.NoError(t, err)
	require.Equal(t, *expectedSignedCommitmentMsg.Message, *actualSignedCommitmentMsg.Message)
	require.Equal(t, expectedSignedCommitmentMsg.AggSignature, actualSignedCommitmentMsg.AggSignature)
}

func TestCommitmentMessage_VerifyProof(t *testing.T) {
	t.Parallel()

	const epoch, eventsCount = uint64(100), 11
	commitment, commitmentSigned, stateSyncs := buildCommitmentAndStateSyncs(t, eventsCount, epoch, 0)
	require.Equal(t, uint64(10), commitment.EndID.Sub(commitment.EndID, commitment.StartID).Uint64())

	for _, stateSync := range stateSyncs {
		leaf, err := stateSync.EncodeAbi()
		require.NoError(t, err)

		proof, err := commitment.MerkleTree.GenerateProof(leaf)
		require.NoError(t, err)

		execute := &contractsapi.ExecuteStateReceiverFn{
			Proof: proof,
			Obj:   (*contractsapi.StateSync)(stateSync),
		}

		inputData, err := execute.EncodeAbi()
		require.NoError(t, err)

		executionStateSync := &contractsapi.ExecuteStateReceiverFn{}
		require.NoError(t, executionStateSync.DecodeAbi(inputData))
		require.Equal(t, stateSync.ID.Uint64(), executionStateSync.Obj.ID.Uint64())
		require.Equal(t, stateSync.Sender, executionStateSync.Obj.Sender)
		require.Equal(t, stateSync.Receiver, executionStateSync.Obj.Receiver)
		require.Equal(t, stateSync.Data, executionStateSync.Obj.Data)
		require.Equal(t, proof, executionStateSync.Proof)

		err = commitmentSigned.VerifyStateSyncProof(executionStateSync.Proof,
			(*contractsapi.StateSyncedEvent)(executionStateSync.Obj))
		require.NoError(t, err)
	}
}

func TestCommitmentMessage_VerifyProof_NoStateSyncsInCommitment(t *testing.T) {
	t.Parallel()

	commitment := &CommitmentMessageSigned{Message: &contractsapi.StateSyncCommitment{StartID: big.NewInt(1), EndID: big.NewInt(10)}}
	err := commitment.VerifyStateSyncProof([]types.Hash{}, nil)
	assert.ErrorContains(t, err, "no state sync event")
}

func TestCommitmentMessage_VerifyProof_StateSyncHashNotEqualToProof(t *testing.T) {
	t.Parallel()

	const (
		fromIndex = 0
		toIndex   = 4
	)

	stateSyncs := generateStateSyncEvents(t, 5, 0)
	tree, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	leaf, err := stateSyncs[0].EncodeAbi()
	require.NoError(t, err)

	proof, err := tree.GenerateProof(leaf)
	require.NoError(t, err)

	commitment := &CommitmentMessageSigned{
		Message: &contractsapi.StateSyncCommitment{
			StartID: big.NewInt(fromIndex),
			EndID:   big.NewInt(toIndex),
			Root:    tree.Hash(),
		},
	}

	assert.ErrorContains(t, commitment.VerifyStateSyncProof(proof, stateSyncs[4]), "not a member of merkle tree")
}

func newTestCommitmentSigned(t *testing.T, root types.Hash, startID, endID int64) *CommitmentMessageSigned {
	t.Helper()

	return &CommitmentMessageSigned{
		Message: &contractsapi.StateSyncCommitment{
			StartID: big.NewInt(startID),
			EndID:   big.NewInt(endID),
			Root:    root,
		},
		AggSignature: Signature{},
		PublicKeys:   [][]byte{},
	}
}

func buildCommitmentAndStateSyncs(t *testing.T, stateSyncsCount int,
	epoch, startIdx uint64) (*PendingCommitment, *CommitmentMessageSigned, []*contractsapi.StateSyncedEvent) {
	t.Helper()

	stateSyncEvents := generateStateSyncEvents(t, stateSyncsCount, startIdx)
	commitment, err := NewPendingCommitment(epoch, stateSyncEvents)
	require.NoError(t, err)

	commitmentSigned := &CommitmentMessageSigned{
		Message: commitment.StateSyncCommitment,
		AggSignature: Signature{
			AggregatedSignature: []byte{},
			Bitmap:              []byte{},
		},
		PublicKeys: [][]byte{},
	}

	return commitment, commitmentSigned, stateSyncEvents
}
