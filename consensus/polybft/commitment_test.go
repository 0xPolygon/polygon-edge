package polybft

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
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

	commitmentMessage1 := NewCommitmentMessage(trie1.Hash(), 2, 8)
	commitmentMessage2 := NewCommitmentMessage(trie1.Hash(), 2, 8)
	commitmentMessage3 := NewCommitmentMessage(trie1.Hash(), 6, 10)
	commitmentMessage4 := NewCommitmentMessage(trie2.Hash(), 2, 8)

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
	_, commitmentMessage, _ := buildCommitmentAndStateSyncs(t, eventsCount, epoch, uint64(2))
	expectedSignedCommitmentMsg := &CommitmentMessageSigned{
		Message: commitmentMessage,
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
	commitment, commitmentMessage, stateSyncs := buildCommitmentAndStateSyncs(t, eventsCount, epoch, 0)
	require.Equal(t, uint64(10), commitmentMessage.ToIndex-commitment.FromIndex)

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

		err = commitmentMessage.VerifyStateSyncProof(executionStateSync)
		require.NoError(t, err)
	}
}

func TestCommitmentMessage_VerifyProof_NoStateSyncsInCommitment(t *testing.T) {
	t.Parallel()

	commitment := &CommitmentMessage{FromIndex: 0, ToIndex: 4}
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

	commitment := &CommitmentMessage{
		FromIndex:      fromIndex,
		ToIndex:        toIndex,
		MerkleRootHash: trie.Hash(),
	}

	assert.ErrorContains(t, commitment.VerifyStateSyncProof(stateSyncProof), "not a member of merkle tree")
}

func buildCommitmentAndStateSyncs(t *testing.T, stateSyncsCount int,
	epoch, startIdx uint64) (*Commitment, *CommitmentMessage, []*types.StateSyncEvent) {
	t.Helper()

	stateSyncEvents := generateStateSyncEvents(t, stateSyncsCount, startIdx)

	fromIndex := stateSyncEvents[0].ID
	toIndex := stateSyncEvents[len(stateSyncEvents)-1].ID
	commitment, err := NewCommitment(epoch, stateSyncEvents)

	require.NoError(t, err)

	commitmentMsg := NewCommitmentMessage(commitment.MerkleTree.Hash(),
		fromIndex,
		toIndex)

	require.NoError(t, err)

	return commitment, commitmentMsg, stateSyncEvents
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
