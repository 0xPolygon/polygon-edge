package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsensusRuntime_GetVotes(t *testing.T) {
	t.Parallel()

	const (
		epoch           = uint64(1)
		validatorsCount = 7
		stateSyncsCount = 15
	)

	validatorIds := []string{"A", "B", "C", "D", "E", "F", "G"}
	validatorAccounts := newTestValidatorsWithAliases(validatorIds)
	state := newTestState(t)
	runtime := &consensusRuntime{
		state: state,
		epoch: &epochMetadata{
			Number:     epoch,
			Validators: validatorAccounts.getPublicIdentities(),
		},
	}

	commitment, _, _ := buildCommitmentAndStateSyncs(t, stateSyncsCount, epoch, 0)

	quorumSize := validatorAccounts.toValidatorSetWithError(t).getQuorumSize()

	require.NoError(t, state.insertEpoch(epoch))

	votesCount := quorumSize + 1
	hash, err := commitment.Hash()
	require.NoError(t, err)

	for i := 0; i < int(votesCount); i++ {
		validator := validatorAccounts.getValidator(validatorIds[i])
		signature, err := validator.mustSign(hash.Bytes()).Marshal()
		require.NoError(t, err)

		_, err = state.insertMessageVote(epoch, hash.Bytes(),
			&MessageSignature{
				From:      validator.Key().String(),
				Signature: signature,
			})
		require.NoError(t, err)
	}

	votes, err := runtime.state.getMessageVotes(runtime.epoch.Number, hash.Bytes())
	require.NoError(t, err)
	require.Len(t, votes, int(votesCount))
}

func TestConsensusRuntime_GetVotesError(t *testing.T) {
	t.Parallel()

	const (
		epoch           = uint64(1)
		stateSyncsCount = 30
		startIndex      = 0
	)

	state := newTestState(t)
	runtime := &consensusRuntime{state: state}
	commitment, _, _ := buildCommitmentAndStateSyncs(t, 5, epoch, startIndex)
	hash, err := commitment.Hash()
	require.NoError(t, err)
	_, err = runtime.state.getMessageVotes(epoch, hash.Bytes())
	assert.ErrorContains(t, err, "could not find")
}

func newTestStateSyncManager(t *testing.T, key *testValidator) *StateSyncManager {
	t.Helper()

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(0))

	s, err := NewStateSyncManager(hclog.NewNullLogger(), key.Key(), state, types.Address{}, "", "", nil)
	require.NoError(t, err)

	return s
}

func TestStateSyncManager_BuildCommitment(t *testing.T) {
	vals := newTestValidators(5)
	s := newTestStateSyncManager(t, vals.getValidator("0"))

	// there are no state syncs
	commitment, err := s.buildCommitment(0, 0)
	require.NoError(t, err)
	require.Nil(t, commitment)

	stateSyncs10 := generateStateSyncEvents(t, 20, 0)

	// add 5 state syncs starting in index 0, it will not generate a commitment
	for i := 0; i < 5; i++ {
		require.NoError(t, s.state.insertStateSyncEvent(stateSyncs10[i]))
	}

	commitment, err = s.buildCommitment(0, 0)
	require.NoError(t, err)
	require.Nil(t, commitment)

	// add the next 5 state syncs, at that point
	for i := 5; i < 10; i++ {
		require.NoError(t, s.state.insertStateSyncEvent(stateSyncs10[i]))
	}

	commitment, err = s.buildCommitment(0, 0)
	require.NoError(t, err)

	require.Equal(t, commitment.Epoch, uint64(0))
	require.Equal(t, commitment.FromIndex, uint64(0))
	require.Equal(t, commitment.ToIndex, uint64(9))
}

/*
func TestConsensusRuntime_deliverMessage_MessageWhenEpochNotStarted(t *testing.T) {
	t.Parallel()

	const epoch = uint64(5)

	validatorIds := []string{"A", "B", "C", "D", "E", "F", "G"}
	state := newTestState(t)
	validators := newTestValidatorsWithAliases(validatorIds)
	localValidator := validators.getValidator("A")
	runtime := &consensusRuntime{
		logger:              hclog.NewNullLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config:              &runtimeConfig{Key: localValidator.Key()},
		epoch: &epochMetadata{
			Number:     epoch,
			Validators: validators.getPublicIdentities(),
		},
		lastBuiltBlock: &types.Header{},
	}

	// dummy hash
	hash := crypto.Keccak256Hash(generateRandomBytes(t)).Bytes()

	// insert dummy epoch to the state
	require.NoError(t, state.insertEpoch(epoch))

	// insert dummy message vote
	_, err := runtime.state.insertMessageVote(epoch, hash,
		createTestMessageVote(t, hash, localValidator))
	require.NoError(t, err)

	// prevent node sender is the local node
	senderID := ""
	for senderID == "" || senderID == localValidator.alias {
		senderID = validatorIds[rand.Intn(len(validatorIds))]
	}
	// deliverMessage should not fail, although epochMetadata is not initialized
	// message vote should be added to the consensus runtime state.
	err = runtime.deliverMessage(createTestTransportMessage(t, hash, epoch, validators.getValidator(senderID).Key()))
	require.NoError(t, err)

	// assert that no additional message signatures aren't inserted into the consensus runtime state
	// (other than the one we have previously inserted by ourselves)
	signatures, err := runtime.state.getMessageVotes(epoch, hash)
	require.NoError(t, err)
	require.Len(t, signatures, 2)
}

func TestConsensusRuntime_AddLog(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		state:  state,
		config: &runtimeConfig{Key: createTestKey(t)},
	}
	topics := make([]ethgo.Hash, 4)
	topics[0] = stateTransferEventABI.ID()
	topics[1] = ethgo.BytesToHash([]byte{0x1})
	topics[2] = ethgo.BytesToHash(runtime.config.Key.Address().Bytes())
	topics[3] = ethgo.BytesToHash(contracts.NativeTokenContract[:])
	personType := abi.MustNewType("tuple(string firstName, string lastName)")
	encodedData, err := personType.Encode(map[string]string{"firstName": "John", "lastName": "Doe"})
	require.NoError(t, err)

	log := &ethgo.Log{
		LogIndex:        uint64(0),
		BlockNumber:     uint64(0),
		TransactionHash: ethgo.BytesToHash(generateRandomBytes(t)),
		BlockHash:       ethgo.BytesToHash(generateRandomBytes(t)),
		Address:         ethgo.ZeroAddress,
		Topics:          topics,
		Data:            encodedData,
	}
	event, err := decodeStateSyncEvent(log)
	require.NoError(t, err)
	runtime.AddLog(log)

	stateSyncs, err := runtime.state.getStateSyncEventsForCommitment(1, 1)
	require.NoError(t, err)
	require.Len(t, stateSyncs, 1)
	require.Equal(t, event.ID, stateSyncs[0].ID)
}

func TestConsensusRuntime_deliverMessage_EpochNotStarted(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	err := state.insertEpoch(1)
	assert.NoError(t, err)

	// random node not among validator set
	account := newTestValidator("A", 1)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		state:  state,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize: 1,
			},
			Key: account.Key(),
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: newTestValidators(5).getPublicIdentities(),
		},
		lastBuiltBlock: &types.Header{},
	}

	msg := createTestTransportMessage(t, generateRandomBytes(t), 1, account.Key())
	err = runtime.deliverMessage(msg)
	assert.ErrorContains(t, err, "not among the active validator set")

	votes, err := state.getMessageVotes(1, msg.Hash)
	assert.NoError(t, err)
	assert.Empty(t, votes)
}

func TestConsensusRuntime_deliverMessage_ForExistingEpochAndCommitmentMessage(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	err := state.insertEpoch(1)
	require.NoError(t, err)

	validators := newTestValidatorsWithAliases([]string{"SENDER", "RECEIVER"})
	validatorSet := validators.getPublicIdentities()
	sender := validators.getValidator("SENDER").Key()

	runtime := &consensusRuntime{
		logger:              hclog.NewNullLogger(),
		state:               state,
		activeValidatorFlag: 1,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize: 1,
			},
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: validatorSet,
		},
		lastBuiltBlock: &types.Header{},
	}

	msg := createTestTransportMessage(t, generateRandomBytes(t), 1, sender)
	err = runtime.deliverMessage(msg)
	assert.NoError(t, err)

	votes, err := state.getMessageVotes(1, msg.Hash)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(votes))
	assert.True(t, bytes.Equal(msg.Signature, votes[0].Signature))
}

func TestConsensusRuntime_deliverMessage_SenderMessageNotInCurrentValidatorset(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	err := state.insertEpoch(1)
	require.NoError(t, err)

	validators := newTestValidators(6)

	runtime := &consensusRuntime{
		state:               state,
		activeValidatorFlag: 1,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize: 1,
			},
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.getPublicIdentities(),
		},
		lastBuiltBlock: &types.Header{},
	}

	msg := createTestTransportMessage(t, generateRandomBytes(t), 1, createTestKey(t))
	err = runtime.deliverMessage(msg)
	assert.Error(t, err)
	assert.ErrorContains(t, err,
		fmt.Sprintf("message is received from sender %s, which is not in current validator set", msg.NodeID))
}
*/
