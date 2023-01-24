package polybft

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func newTestCommitmentSigned(root types.Hash, startID, endID int64) *CommitmentMessageSigned {
	return &CommitmentMessageSigned{
		Message: &contractsapi.Commitment{
			StartID: big.NewInt(startID),
			EndID:   big.NewInt(endID),
			Root:    root,
		},
		AggSignature: Signature{},
		PublicKeys:   [][]byte{},
	}
}

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

	commitmentMessage1 := newTestCommitmentSigned(trie1.Hash(), 2, 8)
	commitmentMessage2 := newTestCommitmentSigned(trie1.Hash(), 2, 8)
	commitmentMessage3 := newTestCommitmentSigned(trie1.Hash(), 6, 10)
	commitmentMessage4 := newTestCommitmentSigned(trie2.Hash(), 2, 8)

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
		Message: pendingCommitment.Commitment,
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

	for i, stateSync := range stateSyncs {
		proof := commitment.MerkleTree.GenerateProof(uint64(i), 0)
		stateSyncsProof := &types.StateSyncProof{
			Proof:     proof,
			StateSync: stateSync,
		}

		inputData, err := stateSyncsProof.EncodeAbi()
		require.NoError(t, err)

		executionStateSync := &types.StateSyncProof{}
		require.NoError(t, executionStateSync.DecodeAbi(inputData))
		require.Equal(t, stateSyncsProof.StateSync, executionStateSync.StateSync)

		err = commitmentSigned.VerifyStateSyncProof(executionStateSync)
		require.NoError(t, err)
	}
}

func TestCommitmentMessage_VerifyProof_NoStateSyncsInCommitment(t *testing.T) {
	t.Parallel()

	commitment := &CommitmentMessageSigned{Message: &contractsapi.Commitment{StartID: big.NewInt(1), EndID: big.NewInt(10)}}
	err := commitment.VerifyStateSyncProof(&types.StateSyncProof{})
	assert.ErrorContains(t, err, "no state sync event")
}

func TestCommitmentMessage_VerifyProof_StateSyncHashNotEqualToProof(t *testing.T) {
	t.Parallel()

	const (
		fromIndex = 0
		toIndex   = 4
	)

	stateSyncs := generateStateSyncEvents(t, 5, 0)
	trie, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	proof := trie.GenerateProof(0, 0)

	stateSyncProof := &types.StateSyncProof{
		StateSync: stateSyncs[4],
		Proof:     proof,
	}

	commitment := &CommitmentMessageSigned{
		Message: &contractsapi.Commitment{
			StartID: big.NewInt(fromIndex),
			EndID:   big.NewInt(toIndex),
			Root:    trie.Hash(),
		},
	}

	assert.ErrorContains(t, commitment.VerifyStateSyncProof(stateSyncProof), "not a member of merkle tree")
}

func buildCommitmentAndStateSyncs(t *testing.T, stateSyncsCount int,
	epoch, startIdx uint64) (*PendingCommitment, *CommitmentMessageSigned, []*types.StateSyncEvent) {
	t.Helper()

	stateSyncEvents := generateStateSyncEvents(t, stateSyncsCount, startIdx)
	commitment, err := NewCommitment(epoch, stateSyncEvents)
	require.NoError(t, err)

	commitmentSigned := &CommitmentMessageSigned{
		Message: commitment.Commitment,
		AggSignature: Signature{
			AggregatedSignature: []byte{},
			Bitmap:              []byte{},
		},
		PublicKeys: [][]byte{},
	}

	return commitment, commitmentSigned, stateSyncEvents
}

func TestStateTransaction_Signature(t *testing.T) {
	t.Parallel()

	cases := []struct {
		m   *abi.Method
		sig string
	}{
		{
			commitEpochMethod,
			"410899c9",
		},
	}
	for _, c := range cases {
		sig := hex.EncodeToString(c.m.ID())
		require.Equal(t, c.sig, sig)
	}
}

func TestStateTransaction_Encoding(t *testing.T) {
	t.Parallel()

	cases := []StateTransactionInput{
		// empty commit epoch
		&CommitEpoch{
			Uptime: Uptime{
				UptimeData: []ValidatorUptime{},
			},
		},
	}

	for _, c := range cases {
		res, err := c.EncodeAbi()

		require.NoError(t, err)

		// use reflection to create another type and decode
		val := reflect.New(reflect.TypeOf(c).Elem()).Interface()
		obj, ok := val.(StateTransactionInput)
		assert.True(t, ok)

		err = obj.DecodeAbi(res)
		require.NoError(t, err)

		require.Equal(t, obj, c)
	}
}
