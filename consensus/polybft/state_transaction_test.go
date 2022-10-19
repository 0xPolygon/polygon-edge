package polybft

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func TestCommitmentMessage_Hash(t *testing.T) {
	const (
		bundleSize  = uint64(2)
		eventsCount = 10
	)

	stateSyncEvents := generateStateSyncEvents(t, eventsCount, 0)

	trie1, err := createMerkleTree(stateSyncEvents, bundleSize)
	require.NoError(t, err)
	trie2, err := createMerkleTree(stateSyncEvents[0:len(stateSyncEvents)-1], bundleSize)
	require.NoError(t, err)
	commitmentMessage1 := NewCommitmentMessage(trie1.Hash(), 2, 8, bundleSize)
	commitmentMessage2 := NewCommitmentMessage(trie1.Hash(), 2, 8, bundleSize)
	commitmentMessage3 := NewCommitmentMessage(trie1.Hash(), 6, 10, bundleSize)
	commitmentMessage4 := NewCommitmentMessage(trie2.Hash(), 2, 8, bundleSize)

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
	const epoch, bundleSize, eventsCount = uint64(100), uint64(3), 11
	_, commitmentMessage, _ := buildCommitmentAndStateSyncs(t, eventsCount, epoch, bundleSize, uint64(2))
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
	const epoch, bundleSize, eventsCount = uint64(100), uint64(3), 11
	commitment, commitmentMessage, stateSyncs := buildCommitmentAndStateSyncs(t, eventsCount, epoch, bundleSize, 0)
	require.Equal(t, uint64(4), commitmentMessage.BundlesCount())

	for i := uint64(0); i < commitmentMessage.BundlesCount(); i++ {
		until := (i + 1) * commitmentMessage.BundleSize
		if until > uint64(len(stateSyncs)) {
			until = uint64(len(stateSyncs))
		}
		proof := commitment.MerkleTree.GenerateProof(i, 0)
		bundleProof := &BundleProof{
			Proof:      proof,
			StateSyncs: stateSyncs[i*commitmentMessage.BundleSize : until],
		}
		inputData, err := bundleProof.EncodeAbi()
		require.NoError(t, err)

		executionBundle := &BundleProof{}
		require.NoError(t, executionBundle.DecodeAbi(inputData))
		require.Equal(t, bundleProof.StateSyncs, executionBundle.StateSyncs)

		err = commitmentMessage.VerifyProof(executionBundle)
		require.NoError(t, err)
	}
}

func TestCommitmentMessage_GetBundleIdxFromStateSyncEventIdx(t *testing.T) {
	cases := []struct {
		eventsCount int
		bundleSize  uint64
	}{
		{5, 2},
		{8, 3},
		{16, 4},
		{24, 7},
	}
	for _, c := range cases {
		_, commitmentMessage, stateSyncEvents := buildCommitmentAndStateSyncs(t, c.eventsCount, 10, c.bundleSize, uint64(2))
		for i, x := range stateSyncEvents {
			bundleIdx := commitmentMessage.GetBundleIdxFromStateSyncEventIdx(x.ID)
			require.Equal(t, uint64(i)/c.bundleSize, bundleIdx)
		}
	}
}

func TestCommitmentMessage_GetFirstStateSyncIndexFromBundleIndex(t *testing.T) {
	cases := []struct {
		bundleSize    uint64
		fromIndex     uint64
		bundleIndex   uint64
		expectedIndex uint64
	}{
		{5, 0, 0, 0},
		{5, 5, 1, 10},
		{10, 100, 3, 130},
		{25, 275, 1, 300},
		{3, 9, 0, 9},
	}

	for _, c := range cases {
		commitment := &CommitmentMessage{
			FromIndex:  c.fromIndex,
			BundleSize: c.bundleSize,
		}

		assert.Equal(t, c.expectedIndex, commitment.GetFirstStateSyncIndexFromBundleIndex(c.bundleIndex))
	}
}

func TestCommitmentMessage_VerifyProof_NoStateSyncsInBundle(t *testing.T) {
	commitment := &CommitmentMessage{FromIndex: 0, ToIndex: 4, BundleSize: 5}

	err := commitment.VerifyProof(&BundleProof{})

	assert.ErrorContains(t, err, "no state sync events")
}

func TestCommitmentMessage_VerifyProof_StateSyncHashNotEqualToProof(t *testing.T) {
	const (
		bundleIndex = 0
		bundleSize  = 5
		fromIndex   = 0
		toIndex     = 4
	)

	stateSyncs := generateStateSyncEvents(t, 5, 0)
	trie, err := createMerkleTree(stateSyncs, bundleSize)
	require.NoError(t, err)

	proof := trie.GenerateProof(bundleIndex, 0)

	bundleProof := &BundleProof{
		StateSyncs: stateSyncs[1:],
		Proof:      proof,
	}

	commitment := &CommitmentMessage{
		FromIndex:      fromIndex,
		ToIndex:        toIndex,
		BundleSize:     bundleSize,
		MerkleRootHash: trie.Hash(),
	}

	assert.ErrorContains(t, commitment.VerifyProof(bundleProof), "not a member of merkle tree")
}

func buildCommitmentAndStateSyncs(t *testing.T, stateSyncsCount int,
	epoch, bundleSize, startIdx uint64) (*Commitment, *CommitmentMessage, []*StateSyncEvent) {
	stateSyncEvents := generateStateSyncEvents(t, stateSyncsCount, startIdx)

	fromIndex := stateSyncEvents[0].ID
	toIndex := stateSyncEvents[len(stateSyncEvents)-1].ID
	commitment, err := NewCommitment(epoch, fromIndex, toIndex, bundleSize, stateSyncEvents)
	require.NoError(t, err)
	commitmentMsg := NewCommitmentMessage(commitment.MerkleTree.Hash(),
		fromIndex,
		toIndex,
		bundleSize)
	require.NoError(t, err)

	return commitment, commitmentMsg, stateSyncEvents
}

func TestStateTransaction_Signature(t *testing.T) {
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
		obj := val.(StateTransactionInput)

		err = obj.DecodeAbi(res)
		require.NoError(t, err)

		require.Equal(t, obj, c)
	}
}
