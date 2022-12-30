package polybft

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func TestConsensusRuntime_GetVotes(t *testing.T) {
	t.Parallel()

	const (
		epoch           = uint64(1)
		validatorsCount = 7
		stateSyncsCount = 15
	)

	validatorIds := []string{"A", "B", "C", "D", "E", "F", "G"}
	validators := newTestValidatorsWithAliases(validatorIds)
	validatorAccs := validators.getPublicIdentities()
	state := newTestState(t)
	runtime := &consensusRuntime{
		state: state,
		epoch: &epochMetadata{
			Number:     epoch,
			Validators: validatorAccs,
		},
	}

	commitment, _, _ := buildCommitmentAndStateSyncs(t, stateSyncsCount, epoch, 0)

	require.NoError(t, state.insertEpoch(epoch))

	votesCount := int(getQuorumSize(validatorAccs.GetTotalVotingPower()).Int64())
	hash, err := commitment.Hash()
	require.NoError(t, err)

	for i := 0; i < votesCount; i++ {
		validator := validators.getValidator(validatorIds[i])
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
	require.Len(t, votes, votesCount)
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

func TestConsensusRuntime_isFixedSizeOfEpochMet_NotReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		epochSize, firstBlockInEpoch, parentBlockNumber uint64
	}{
		{4, 1, 2},
		{5, 1, 3},
		{6, 0, 6},
		{7, 0, 4},
		{8, 0, 5},
		{9, 4, 9},
		{10, 7, 10},
		{10, 1, 1},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{},
		},
		lastBuiltBlock: &types.Header{},
		epoch:          &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.PolyBFTConfig.EpochSize = c.epochSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.False(
			t,
			runtime.isFixedSizeOfEpochMet(c.parentBlockNumber+1, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of epoch for epoch size=%v and parent block number=%v",
				c.epochSize,
				c.parentBlockNumber),
		)
	}
}

func TestConsensusRuntime_isFixedSizeOfEpochMet_ReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		epochSize, firstBlockInEpoch, blockNumber uint64
	}{
		{4, 1, 4},
		{5, 1, 5},
		{6, 0, 5},
		{7, 0, 6},
		{8, 0, 7},
		{9, 4, 12},
		{10, 7, 16},
		{10, 1, 10},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{},
		},
		epoch: &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.PolyBFTConfig.EpochSize = c.epochSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.True(
			t,
			runtime.isFixedSizeOfEpochMet(c.blockNumber, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of epoch for epoch size=%v and parent block number=%v",
				c.epochSize,
				c.blockNumber),
		)
	}
}

func TestConsensusRuntime_isFixedSizeOfSprintMet_NotReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		sprintSize, firstBlockInEpoch, blockNumber uint64
	}{
		{4, 1, 2},
		{5, 1, 3},
		{6, 0, 6},
		{7, 0, 4},
		{8, 0, 5},
		{9, 4, 9},
		{10, 7, 10},
		{10, 1, 1},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{},
		},
		epoch: &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.PolyBFTConfig.SprintSize = c.sprintSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.False(t,
			runtime.isFixedSizeOfSprintMet(c.blockNumber, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of sprint for sprint size=%v and parent block number=%v",
				c.sprintSize,
				c.blockNumber),
		)
	}
}

func TestConsensusRuntime_isFixedSizeOfSprintMet_ReachedEnd(t *testing.T) {
	t.Parallel()

	// because of slashing, we can assume some epochs started at random numbers
	var cases = []struct {
		sprintSize, firstBlockInEpoch, blockNumber uint64
	}{
		{4, 1, 4},
		{5, 1, 5},
		{6, 0, 5},
		{7, 0, 6},
		{8, 0, 7},
		{9, 4, 12},
		{10, 7, 16},
		{10, 1, 10},
		{5, 1, 10},
		{3, 3, 5},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{},
		},
		epoch: &epochMetadata{},
	}

	for _, c := range cases {
		runtime.config.PolyBFTConfig.SprintSize = c.sprintSize
		runtime.epoch.FirstBlockInEpoch = c.firstBlockInEpoch
		assert.True(t,
			runtime.isFixedSizeOfSprintMet(c.blockNumber, runtime.epoch),
			fmt.Sprintf(
				"Not expected end of sprint for sprint size=%v and parent block number=%v",
				c.sprintSize,
				c.blockNumber),
		)
	}
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

func TestConsensusRuntime_OnBlockInserted_EndOfEpoch(t *testing.T) {
	t.Parallel()

	const (
		epochSize       = uint64(10)
		validatorsCount = 7
	)

	currentEpochNumber := getEpochNumber(t, epochSize, epochSize)
	validatorSet := newTestValidators(validatorsCount).getPublicIdentities()
	header, headerMap := createTestBlocks(t, epochSize, epochSize, validatorSet)
	builtBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: header,
	})

	newEpochNumber := currentEpochNumber + 1
	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(newEpochNumber).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorSet).Times(3)

	txPool := new(txPoolMock)
	txPool.On("ResetWithHeaders", mock.Anything).Once()

	snapshot := NewProposerSnapshot(epochSize-1, validatorSet)
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize: epochSize,
		},
		blockchain:     blockchainMock,
		polybftBackend: polybftBackendMock,
		txPool:         txPool,
		State:          newTestState(t),
	}
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		state:              config.State,
		config:             config,
		epoch: &epochMetadata{
			Number:            currentEpochNumber,
			FirstBlockInEpoch: header.Number - epochSize + 1,
		},
		lastBuiltBlock: &types.Header{Number: header.Number - 1},
	}
	runtime.OnBlockInserted(builtBlock)

	require.True(t, runtime.state.isEpochInserted(currentEpochNumber+1))
	require.Equal(t, newEpochNumber, runtime.epoch.Number)

	blockchainMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
}

func TestConsensusRuntime_OnBlockInserted_MiddleOfEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch             = 2
		epochSize         = uint64(10)
		firstBlockInEpoch = epochSize + 1
		blockNumber       = epochSize + 2
	)

	header := &types.Header{Number: blockNumber}
	builtBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: header,
	})

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(builtBlock.Header, true).Once()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(nil).Once()

	txPool := new(txPoolMock)
	txPool.On("ResetWithHeaders", mock.Anything).Once()

	snapshot := NewProposerSnapshot(blockNumber, []*ValidatorMetadata{})
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{EpochSize: epochSize},
		blockchain:    blockchainMock,
		txPool:        txPool,
	}

	runtime := &consensusRuntime{
		lastBuiltBlock: header,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{EpochSize: epochSize},
			blockchain:    blockchainMock,
			txPool:        txPool,
		},
		epoch: &epochMetadata{
			Number:            epoch,
			FirstBlockInEpoch: firstBlockInEpoch,
		},
		logger:             hclog.NewNullLogger(),
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}
	runtime.OnBlockInserted(builtBlock)

	require.Equal(t, header.Number, runtime.lastBuiltBlock.Number)
}

func TestConsensusRuntime_FSM_NotInValidatorSet(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D"})

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize: 1,
		},
		Key: createTestKey(t),
	}
	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		activeValidatorFlag: 1,
		config:              config,
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.getPublicIdentities(),
		},
		lastBuiltBlock: &types.Header{},
	}

	err := runtime.FSM()
	assert.ErrorIs(t, err, errNotAValidator)
}

func TestConsensusRuntime_FSM_NotEndOfEpoch_NotEndOfSprint(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{},
	}
	lastBlock := &types.Header{
		Number:    1,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}

	validators := newTestValidators(3)
	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  10,
			SprintSize: 5,
		},
		Key:        wallet.NewKey(validators.getPrivateIdentities()[0]),
		blockchain: blockchainMock,
	}
	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:              hclog.NewNullLogger(),
		activeValidatorFlag: 1,
		config:              config,
		epoch: &epochMetadata{
			Number:            1,
			Validators:        validators.getPublicIdentities(),
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock: lastBlock,
		state:          newTestState(t),
	}

	err := runtime.FSM()
	require.NoError(t, err)

	assert.True(t, runtime.isActiveValidator())
	assert.False(t, runtime.fsm.isEndOfEpoch)
	assert.False(t, runtime.fsm.isEndOfSprint)
	assert.Equal(t, lastBlock.Number, runtime.fsm.parent.Number)

	address := types.Address(runtime.config.Key.Address())
	assert.True(t, runtime.fsm.ValidatorSet().Includes(address))

	assert.NotNil(t, runtime.fsm.blockBuilder)
	assert.NotNil(t, runtime.fsm.backend)

	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_FSM_EndOfEpoch_BuildRegisterCommitment_And_Uptime(t *testing.T) {
	t.Parallel()

	const (
		epoch               = 1
		epochSize           = uint64(10)
		firstBlockInEpoch   = uint64(1)
		sprintSize          = uint64(3)
		commitmentSize      = 10
		beginStateSyncIndex = uint64(0)
		fromIndex           = uint64(0)
		toIndex             = uint64(9)
	)

	validatorAccounts := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccounts.getPublicIdentities()
	accounts := validatorAccounts.getPrivateIdentities()

	lastBuiltBlock, headerMap := createTestBlocks(t, 9, epochSize, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(epoch))

	stateSyncs := generateStateSyncEvents(t, commitmentSize, 0)
	for _, event := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(event))
	}

	trie, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	commitment := &Commitment{MerkleTree: trie, Epoch: epoch}

	hash, err := commitment.Hash()
	require.NoError(t, err)

	for _, a := range accounts {
		signature, err := a.Bls.Sign(hash.Bytes())
		require.NoError(t, err)
		signatureRaw, err := signature.Marshal()
		require.NoError(t, err)
		_, err = state.insertMessageVote(epoch, hash.Bytes(), &MessageSignature{
			From:      a.Ecdsa.Address().String(),
			Signature: signatureRaw,
		})
		require.NoError(t, err)
	}

	metadata := &epochMetadata{
		Validators:        validators,
		Number:            epoch,
		FirstBlockInEpoch: firstBlockInEpoch,
		Commitment:        commitment,
	}

	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  epochSize,
			SprintSize: sprintSize,
			Bridge:     &BridgeConfig{},
		},
		Key:        validatorAccounts.getValidator("A").Key(),
		blockchain: blockchainMock,
	}

	snapshot := NewProposerSnapshot(1, nil)
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		state:              state,
		epoch:              metadata,
		config:             config,
		lastBuiltBlock:     lastBuiltBlock,
	}

	err = runtime.FSM()
	fsm := runtime.fsm

	assert.NoError(t, err)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.uptimeCounter)
	assert.NotEmpty(t, fsm.uptimeCounter)
	assert.NotNil(t, fsm.proposerCommitmentToRegister)
	assert.Equal(t, fromIndex, fsm.proposerCommitmentToRegister.Message.FromIndex)
	assert.Equal(t, toIndex, fsm.proposerCommitmentToRegister.Message.ToIndex)
	assert.Equal(t, trie.Hash(), fsm.proposerCommitmentToRegister.Message.MerkleRootHash)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_FSM_EndOfEpoch_RegisterCommitmentNotFound(t *testing.T) {
	t.Parallel()

	const (
		epochSize           = uint64(10)
		sprintSize          = uint64(5)
		beginStateSyncIndex = uint64(5)
	)

	validatorAccs := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccs.getPublicIdentities()
	lastBuiltBlock, headerMap := createTestBlocks(t, 9, epochSize, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(new(blockBuilderMock), nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	epoch := getEpochNumber(t, lastBuiltBlock.Number, epochSize)
	metadata := &epochMetadata{
		Validators:        validators,
		Number:            epoch,
		FirstBlockInEpoch: epoch*epochSize - epochSize + 1,
	}

	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  epochSize,
			SprintSize: sprintSize,
			Bridge:     &BridgeConfig{},
		},
		Key:        validatorAccs.getValidator("A").Key(),
		blockchain: blockchainMock,
	}

	snapshot := NewProposerSnapshot(1, nil)
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		epoch:              metadata,
		config:             config,
		lastBuiltBlock:     lastBuiltBlock,
		state:              newTestState(t),
	}

	err := runtime.FSM()
	fsm := runtime.fsm

	require.NoError(t, err)
	require.NotNil(t, fsm)
	require.Nil(t, fsm.proposerCommitmentToRegister)
	require.True(t, fsm.isEndOfEpoch)
	require.NotNil(t, fsm.uptimeCounter)
	require.NotEmpty(t, fsm.uptimeCounter)
}

func TestConsensusRuntime_FSM_EndOfEpoch_BuildRegisterCommitment_QuorumNotReached(t *testing.T) {
	t.Parallel()

	const (
		epoch               = 1
		epochSize           = uint64(10)
		firstBlockInEpoch   = uint64(1)
		sprintSize          = uint64(5)
		beginStateSyncIndex = uint64(0)
	)

	validatorAccs := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccs.getPublicIdentities()
	lastBuiltBlock, headerMap := createTestBlocks(t, 9, epochSize, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(epoch))

	stateSyncs := generateStateSyncEvents(t, 10, 0)
	for _, event := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(event))
	}

	trie, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	commitment := &Commitment{MerkleTree: trie, Epoch: epoch}

	hash, err := commitment.Hash()
	require.NoError(t, err)

	validatorKey := validatorAccs.getValidator("C").Key()
	signature, err := validatorKey.Sign(hash.Bytes())
	require.NoError(t, err)
	_, err = state.insertMessageVote(epoch, hash.Bytes(), &MessageSignature{
		From:      validators[0].Address.String(),
		Signature: signature,
	})
	require.NoError(t, err)

	metadata := &epochMetadata{
		Validators:        validators,
		Number:            epoch,
		FirstBlockInEpoch: firstBlockInEpoch,
		Commitment:        commitment,
	}

	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  epochSize,
			SprintSize: sprintSize,
			Bridge:     &BridgeConfig{},
		},
		Key:        validatorKey,
		blockchain: blockchainMock,
	}

	snapshot := NewProposerSnapshot(1, nil)
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		state:              state,
		epoch:              metadata,
		config:             config,
		lastBuiltBlock:     lastBuiltBlock,
	}

	err = runtime.FSM()
	fsm := runtime.fsm

	assert.NoError(t, err)
	assert.Nil(t, fsm.proposerCommitmentToRegister)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.uptimeCounter)
	assert.NotEmpty(t, fsm.uptimeCounter)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func Test_NewConsensusRuntime(t *testing.T) {
	t.Parallel()

	_, err := os.Create("/tmp/consensusState.db")
	require.NoError(t, err)

	polyBftConfig := &PolyBFTConfig{
		Bridge: &BridgeConfig{
			BridgeAddr:      types.Address{0x13},
			CheckpointAddr:  types.Address{0x10},
			JSONRPCEndpoint: "testEndpoint",
		},
		ValidatorSetAddr: types.Address{0x11},
		EpochSize:        10,
		SprintSize:       10,
		BlockTime:        2 * time.Second,
	}

	validators := newTestValidators(3).getPublicIdentities()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(uint64(1)).Once()
	systemStateMock.On("GetNextCommittedIndex").Return(uint64(1)).Once()

	blockchainMock := &blockchainMock{}
	blockchainMock.On("CurrentHeader").Return(&types.Header{Number: 1, ExtraData: createTestExtraForAccounts(t, 1, validators, nil)})
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()
	blockchainMock.On("GetHeaderByNumber", uint64(0)).Return(&types.Header{Number: 0, ExtraData: createTestExtraForAccounts(t, 0, validators, nil)})

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators).Twice()

	tmpDir := t.TempDir()
	config := &runtimeConfig{
		polybftBackend: polybftBackendMock,
		State:          newTestState(t),
		PolyBFTConfig:  polyBftConfig,
		DataDir:        tmpDir,
		Key:            createTestKey(t),
		blockchain:     blockchainMock,
	}
	runtime, err := newConsensusRuntime(hclog.NewNullLogger(), config)
	require.NoError(t, err)

	assert.False(t, runtime.isActiveValidator())
	assert.Equal(t, runtime.config.DataDir, tmpDir)
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.SprintSize)
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.EpochSize)
	assert.Equal(t, "0x1100000000000000000000000000000000000000", runtime.config.PolyBFTConfig.ValidatorSetAddr.String())
	assert.Equal(t, "0x1300000000000000000000000000000000000000", runtime.config.PolyBFTConfig.Bridge.BridgeAddr.String())
	assert.Equal(t, "0x1000000000000000000000000000000000000000", runtime.config.PolyBFTConfig.Bridge.CheckpointAddr.String())
	assert.True(t, runtime.IsBridgeEnabled())
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_FSM_EndOfSprint_HasCommitmentsToRegister(t *testing.T) {
	t.Parallel()

	const (
		epochNumber        = uint64(2)
		firstBlockInEpoch  = uint64(21)
		fromIndex          = uint64(5)
		nextCommittedIndex = uint64(3)
		stateSyncsCount    = 30
		commitmentSize     = stateSyncsCount / 3
	)

	state := newTestState(t)
	err := state.insertEpoch(epochNumber)
	require.NoError(t, err)

	stateSyncs := insertTestStateSyncEvents(t, stateSyncsCount, fromIndex, state)

	commitment, err := NewCommitment(epochNumber, stateSyncs)
	require.NoError(t, err)

	commitmentMsg := NewCommitmentMessage(commitment.MerkleTree.Hash(), fromIndex, fromIndex+commitmentSize)
	signedCommitmentMsg := &CommitmentMessageSigned{
		Message:      commitmentMsg,
		AggSignature: Signature{},
	}
	require.NoError(t, state.insertCommitmentMessage(signedCommitmentMsg))

	validatorAccs := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F", "G"})
	validatorSet := validatorAccs.getPublicIdentities()

	extra := &Extra{
		Checkpoint: &CheckpointData{},
	}

	lastBlock := &types.Header{
		Number:    24,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(nextCommittedIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder").Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	for i := 0; i < int(lastBlock.Number); i++ {
		blockchainMock.On("GetHeaderByNumber", uint64(i)).Return(&types.Header{
			Number:    uint64(i),
			ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
		}, true).Maybe()
	}

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  10,
			SprintSize: 5,
			Bridge: &BridgeConfig{
				BridgeAddr: types.BytesToAddress(big.NewInt(23).Bytes()),
			},
		},
		State:      state,
		Key:        validatorAccs.getValidator("A").Key(),
		blockchain: blockchainMock,
	}

	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:              hclog.NewNullLogger(),
		activeValidatorFlag: 1,
		state:               config.State,
		config:              config,
		epoch: &epochMetadata{
			Number:            epochNumber,
			FirstBlockInEpoch: firstBlockInEpoch,
			Validators:        validatorSet,
			Commitment:        commitment,
		},
		lastBuiltBlock: lastBlock,
	}

	require.NoError(t, runtime.buildProofs(runtime.getEpoch().Commitment, commitmentMsg))

	err = runtime.FSM()
	fsm := runtime.fsm

	require.NoError(t, err)

	// check if it is end of sprint
	require.True(t, fsm.isEndOfSprint)

	// check if commitment message to execute is attached to fsm
	require.Nil(t, fsm.proposerCommitmentToRegister)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_SameEpochNumberAsTheLastOne(t *testing.T) {
	t.Parallel()

	const originalBlockNumber = uint64(5)

	newCurrentHeader := &types.Header{Number: originalBlockNumber + 1}
	validatorSet := newTestValidators(3).getPublicIdentities()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(uint64(1), nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		blockchain: blockchainMock,
	}
	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		activeValidatorFlag: 1,
		config:              config,
		epoch: &epochMetadata{
			Number:            1,
			Validators:        validatorSet,
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock: &types.Header{
			Number: originalBlockNumber,
		},
	}

	epoch, err := runtime.restartEpoch(newCurrentHeader)

	require.NoError(t, err)

	for _, a := range validatorSet.GetAddresses() {
		assert.True(t, epoch.Validators.ContainsAddress(a))
	}

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_FirstRestart_NoStateSyncEvents(t *testing.T) {
	t.Parallel()

	newCurrentHeader := &types.Header{Number: 0}
	state := newTestState(t)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(uint64(1), nil).Once()

	validators := newTestValidators(3)
	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.getPublicIdentities()).Once()

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		blockchain:     blockchainMock,
		polybftBackend: polybftBackendMock,
		PolyBFTConfig:  &PolyBFTConfig{},
	}
	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:              hclog.NewNullLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config:              config,
	}

	epoch, err := runtime.restartEpoch(newCurrentHeader)

	require.NoError(t, err)
	require.Equal(t, uint64(1), epoch.Number)
	require.Equal(t, 3, len(epoch.Validators))
	require.True(t, runtime.isActiveValidator())
	require.True(t, state.isEpochInserted(1))

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_FirstRestart_BuildsCommitment(t *testing.T) {
	t.Parallel()

	const (
		newEpoch           = uint64(2)
		epochSize          = uint64(10)
		nextCommittedIndex = uint64(10)
		stateSyncsCount    = 20
	)

	state := newTestState(t)
	stateSyncs := insertTestStateSyncEvents(t, stateSyncsCount, 0, state)
	validatorIds := []string{"A", "B", "C", "D", "E", "F"}
	validatorAccs := newTestValidatorsWithAliases(validatorIds)
	validators := validatorAccs.getPublicIdentities()

	header, _ := createTestBlocks(t, epochSize, epochSize, validators)

	transportMock := new(transportMock)
	transportMock.On("Multicast", mock.Anything).Once()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(nextCommittedIndex, nil).Once()
	systemStateMock.On("GetEpoch").Return(newEpoch, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators).Once()

	localValidatorID := validatorIds[rand.Intn(len(validatorIds))]
	localValidator := validatorAccs.getValidator(localValidatorID)

	snapshot := NewProposerSnapshot(header.Number, validators)
	config := &runtimeConfig{
		blockchain:      blockchainMock,
		polybftBackend:  polybftBackendMock,
		BridgeTransport: transportMock,
		Key:             localValidator.Key(),
		PolyBFTConfig: &PolyBFTConfig{
			Bridge: &BridgeConfig{},
		},
		State: state,
	}
	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:              hclog.NewNullLogger(),
		activeValidatorFlag: 1,
		state:               config.State,
		config:              config,
	}

	epochData, err := runtime.restartEpoch(header)

	require.NoError(t, err)
	require.Equal(t, newEpoch, epochData.Number)
	require.Equal(t, len(validatorIds), len(epochData.Validators))
	require.True(t, runtime.isActiveValidator())
	require.True(t, state.isEpochInserted(newEpoch))

	commitment := epochData.Commitment
	require.NotNil(t, commitment)
	require.Equal(t, newEpoch, commitment.Epoch)

	commitmentHash, err := commitment.Hash()
	require.NoError(t, err)
	stateSyncsTrie, err := createMerkleTree(stateSyncs[nextCommittedIndex : nextCommittedIndex+stateSyncCommitmentSize])
	require.NoError(t, err)
	require.Equal(t, stateSyncsTrie.Hash(), commitment.MerkleTree.Hash())

	votes, err := state.getMessageVotes(newEpoch, commitmentHash.Bytes())
	require.NoError(t, err)
	require.Equal(t, 1, len(votes))
	require.Equal(t, localValidator.Key().String(), votes[0].From)

	signature, err := localValidator.mustSign(commitmentHash.Bytes()).Marshal()
	require.NoError(t, err)
	require.Equal(t, signature, votes[0].Signature)

	for _, validator := range validatorAccs.validators {
		if localValidator.Key().String() == validator.Key().String() {
			continue
		}

		signature, err := validator.mustSign(commitmentHash.Bytes()).Marshal()
		require.NoError(t, err)

		_, err = state.insertMessageVote(epochData.Number, commitmentHash.Bytes(),
			&MessageSignature{
				From:      validator.Key().String(),
				Signature: signature,
			})
		require.NoError(t, err)
	}

	commitmentMsgSigned, err := runtime.getCommitmentToRegister(epochData, nextCommittedIndex)
	require.NoError(t, err)
	require.NotNil(t, commitmentMsgSigned)
	require.Equal(t, nextCommittedIndex, commitmentMsgSigned.Message.FromIndex)

	transportMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_NewEpochToRun_BuildCommitment(t *testing.T) {
	t.Parallel()

	const (
		blockNumber        = 11
		epochSize          = 10
		nextCommittedIndex = uint64(10)
		oldEpoch           = uint64(1)
		newEpoch           = oldEpoch + 1
		validatorsCount    = 6
	)

	// create originalValidators
	originalValidatorIds := []string{"A", "B", "C", "D", "E", "F"}
	originalValidators := newTestValidatorsWithAliases(originalValidatorIds)
	oldValidatorSet := originalValidators.getPublicIdentities()

	// remove first validator and add a new one to the end
	newValidatorSet := make(AccountSet, validatorsCount)
	for i := 1; i < len(oldValidatorSet); i++ {
		newValidatorSet[i-1] = oldValidatorSet[i].Copy()
	}

	newValidatorSet[validatorsCount-1] = newTestValidator("G", 1).ValidatorMetadata()

	header, headerMap := createTestBlocks(t, blockNumber, epochSize, oldValidatorSet)

	state := newTestState(t)
	allStateSyncs := insertTestStateSyncEvents(t, 4*stateSyncCommitmentSize, 0, state)

	transportMock := &transportMock{}
	transportMock.On("Multicast", mock.Anything).Once()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(nextCommittedIndex, nil).Once()
	systemStateMock.On("GetEpoch").Return(newEpoch, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(newValidatorSet).Once()

	snapshot := NewProposerSnapshot(header.Number, nil)
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  10,
			SprintSize: 5,
			Bridge: &BridgeConfig{
				BridgeAddr: types.BytesToAddress(big.NewInt(23).Bytes()),
			},
		},
		BridgeTransport: transportMock,
		Key:             originalValidators.getValidator("A").Key(),
		blockchain:      blockchainMock,
		polybftBackend:  polybftBackendMock,
	}
	runtime := &consensusRuntime{
		proposerCalculator:  NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:              hclog.NewNullLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config:              config,
		epoch: &epochMetadata{
			Number:            oldEpoch,
			Validators:        originalValidators.getPublicIdentities(),
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock: header,
	}

	epoch, err := runtime.restartEpoch(header)

	require.NoError(t, err)

	// check new epoch number
	assert.Equal(t, newEpoch, epoch.Number)

	// check new epoch number
	assert.Equal(t, header.Number, runtime.lastBuiltBlock.Number)

	// check if it is validator
	assert.True(t, runtime.isActiveValidator())

	// check if new epoch is inserted
	assert.True(t, state.isEpochInserted(newEpoch))

	// check if new epoch is created
	assert.NotNil(t, runtime.epoch)

	// check new validators
	assert.Equal(t, len(originalValidatorIds), len(epoch.Validators))

	for _, a := range newValidatorSet.GetAddresses() {
		assert.True(t, epoch.Validators.ContainsAddress(a))
	}

	commitment := epoch.Commitment
	require.NotNil(t, commitment)
	require.Equal(t, newEpoch, commitment.Epoch)

	commitmentHash, err := commitment.Hash()
	require.NoError(t, err)

	for _, validatorID := range originalValidatorIds {
		validator := originalValidators.getValidator(validatorID)
		signature, err := validator.mustSign(commitmentHash.Bytes()).Marshal()
		require.NoError(t, err)
		_, err = state.insertMessageVote(epoch.Number, commitmentHash.Bytes(),
			&MessageSignature{
				From:      validator.Key().String(),
				Signature: signature,
			})
		require.NoError(t, err)
	}

	stateSyncTrie, err := createMerkleTree(allStateSyncs[nextCommittedIndex : nextCommittedIndex+stateSyncCommitmentSize])
	require.NoError(t, err)
	require.NotNil(t, stateSyncTrie)

	commitmentMsgSigned, err := runtime.getCommitmentToRegister(epoch, nextCommittedIndex)
	require.NoError(t, err)
	require.NotNil(t, commitmentMsgSigned)
	require.Equal(t, stateSyncTrie.Hash(), commitmentMsgSigned.Message.MerkleRootHash)
	require.Equal(t, nextCommittedIndex, commitmentMsgSigned.Message.FromIndex)
	require.Equal(t, nextCommittedIndex+stateSyncCommitmentSize-1, commitmentMsgSigned.Message.ToIndex)

	transportMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_calculateUptime_SecondEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch           = 2
		epochSize       = 10
		epochStartBlock = 11
		epochEndBlock   = 20
		sprintSize      = 5
	)

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	polybftConfig := &PolyBFTConfig{
		ValidatorSetAddr: contracts.ValidatorSetContract,
		EpochSize:        epochSize,
		SprintSize:       sprintSize,
	}

	lastBuiltBlock, headerMap := createTestBlocks(t, 19, epochSize, validators.getPublicIdentities())

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.getPublicIdentities()).Twice()

	config := &runtimeConfig{
		PolyBFTConfig:  polybftConfig,
		blockchain:     blockchainMock,
		polybftBackend: polybftBackendMock,
		Key:            validators.getValidator("A").Key(),
	}

	consensusRuntime := &consensusRuntime{
		config: config,
		epoch: &epochMetadata{
			Number:            epoch,
			Validators:        validators.getPublicIdentities(),
			FirstBlockInEpoch: epochStartBlock,
		},
		lastBuiltBlock: lastBuiltBlock,
	}

	uptime, err := consensusRuntime.calculateUptime(lastBuiltBlock, consensusRuntime.epoch)
	assert.NoError(t, err)
	assert.NotEmpty(t, uptime)
	assert.Equal(t, uint64(epoch), uptime.EpochID)
	assert.Equal(t, uint64(epochStartBlock), uptime.Epoch.StartBlock)
	assert.Equal(t, uint64(epochEndBlock), uptime.Epoch.EndBlock)

	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_validateVote_VoteSentFromUnknownValidator(t *testing.T) {
	t.Parallel()

	epoch := &epochMetadata{Validators: newTestValidators(5).getPublicIdentities()}
	nonValidatorAccount := createTestKey(t)
	hash := crypto.Keccak256Hash(generateRandomBytes(t)).Bytes()
	// Sign content by non validator account
	signature, err := nonValidatorAccount.Sign(hash)
	require.NoError(t, err)

	vote := &MessageSignature{
		From:      nonValidatorAccount.String(),
		Signature: signature}
	assert.ErrorContains(t, validateVote(vote, epoch),
		fmt.Sprintf("message is received from sender %s, which is not in current validator set", vote.From))
}

func TestConsensusRuntime_buildProofs_NoCommitment(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		State: state,
	}

	commitmentMsg := NewCommitmentMessage(types.Hash{}, 0, 4)
	runtime := &consensusRuntime{
		logger:             hclog.NewNullLogger(),
		state:              config.State,
		epoch:              &epochMetadata{Number: 0},
		lastBuiltBlock:     &types.Header{},
		config:             config,
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}

	guardedData, err := runtime.getGuardedData()
	require.NoError(t, err)
	assert.NoError(t, runtime.buildProofs(guardedData.epoch.Commitment, commitmentMsg))

	proof, err := state.getStateSyncProof(0)

	assert.NoError(t, err)
	assert.Nil(t, proof)
}

func TestConsensusRuntime_buildProofs(t *testing.T) {
	t.Parallel()

	const (
		epoch                 = 1
		bundleSize            = 5
		fromIndex             = 0
		toIndex               = 4
		expectedBundlesNumber = 1
	)

	state := newTestState(t)
	stateSyncs := insertTestStateSyncEvents(t, bundleSize, 0, state)
	trie, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	commitmentMsg := NewCommitmentMessage(trie.Hash(), fromIndex, toIndex)
	commitmentMsgSigned := &CommitmentMessageSigned{
		Message: commitmentMsg,
		AggSignature: Signature{
			Bitmap:              []byte{5, 1},
			AggregatedSignature: []byte{1, 1},
		},
	}
	require.NoError(t, state.insertCommitmentMessage(commitmentMsgSigned))

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		State: state,
	}

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		state:  config.State,
		epoch: &epochMetadata{
			Number: epoch,
			Commitment: &Commitment{
				MerkleTree: trie,
				Epoch:      epoch,
			},
		},
		lastBuiltBlock:     &types.Header{},
		config:             config,
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}

	guardedData, err := runtime.getGuardedData()
	require.NoError(t, err)
	assert.NoError(t, runtime.buildProofs(guardedData.epoch.Commitment, commitmentMsg))

	for i := fromIndex; i <= toIndex; i++ {
		proof, err := state.getStateSyncProof(uint64(i))
		assert.NoError(t, err)
		assert.NotNil(t, proof)
		assert.NotEmpty(t, proof.Proof)
		assert.Equal(t, uint64(i), proof.StateSync.ID)
	}
}

func TestConsensusRuntime_FSM_EndOfEpoch_OnBlockInserted(t *testing.T) {
	t.Parallel()

	const (
		epoch               = 1
		epochSize           = uint64(10)
		firstBlockInEpoch   = uint64(1)
		sprintSize          = uint64(5)
		beginStateSyncIndex = uint64(0)
		fromIndex           = uint64(0)
		toIndex             = uint64(9)
		numOfBlock          = uint64(9)
	)

	validatorAccounts := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	signingAccounts := validatorAccounts.getPrivateIdentities()
	validators := validatorAccounts.getPublicIdentities()
	lastBuiltBlock, headerMap := createTestBlocks(t, 9, epochSize, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(epoch)
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil)
	// systemStateMock.On("GetNextExecutionIndex").Return(beginStateSyncIndex, nil)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock))
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators).Once()

	txPool := new(txPoolMock)

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(epoch))

	stateSyncs := generateStateSyncEvents(t, stateSyncCommitmentSize, 0)
	for _, event := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(event))
	}

	trie, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	commitment := &Commitment{MerkleTree: trie, Epoch: epoch}
	hash, err := commitment.Hash()
	require.NoError(t, err)

	for _, a := range signingAccounts {
		signature, err := a.Bls.Sign(hash.Bytes())
		require.NoError(t, err)
		signatureRaw, err := signature.Marshal()
		require.NoError(t, err)
		_, err = state.insertMessageVote(epoch, hash.Bytes(), &MessageSignature{
			From:      a.Ecdsa.Address().String(),
			Signature: signatureRaw,
		})
		require.NoError(t, err)
	}

	metadata := &epochMetadata{
		Validators:        validators,
		Number:            epoch,
		FirstBlockInEpoch: firstBlockInEpoch,
		Commitment:        commitment,
	}

	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  epochSize,
			SprintSize: sprintSize,
			Bridge:     &BridgeConfig{},
		},
		polybftBackend: polybftBackendMock,
		Key:            validatorAccounts.getValidator("A").Key(),
		blockchain:     blockchainMock,
		txPool:         txPool,
		State:          state,
	}

	signer := validatorAccounts.getValidator("A").Key()
	snapshot := NewProposerSnapshot(numOfBlock, nil)
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		state:              config.State,
		epoch:              metadata,
		config:             config,
		lastBuiltBlock:     lastBuiltBlock,
		checkpointManager:  newCheckpointManager(wallet.NewEcdsaSigner(signer), 5, types.ZeroAddress, nil, nil, nil, hclog.NewNullLogger()),
	}

	err = runtime.FSM()
	fsm := runtime.fsm

	assert.NoError(t, err)
	assert.NotNil(t, fsm.proposerCommitmentToRegister)
	assert.Equal(t, fromIndex, fsm.proposerCommitmentToRegister.Message.FromIndex)
	assert.Equal(t, toIndex, fsm.proposerCommitmentToRegister.Message.ToIndex)
	assert.Equal(t, trie.Hash(), fsm.proposerCommitmentToRegister.Message.MerkleRootHash)
	assert.NotNil(t, fsm.proposerCommitmentToRegister.AggSignature)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.uptimeCounter)
	assert.NotEmpty(t, fsm.uptimeCounter)

	inputData, err := fsm.proposerCommitmentToRegister.EncodeAbi()
	assert.NoError(t, err)

	tx := createStateTransactionWithData(fsm.config.StateReceiverAddr, inputData)

	block := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{Number: lastBuiltBlock.Number + 1},
		Txns: []*types.Transaction{
			tx,
		},
	})

	txPool.On("ResetWithHeaders", mock.MatchedBy(func(i interface{}) bool {
		ph, ok := i.([]*types.Header)
		require.True(t, ok)
		require.Len(t, ph, 1)

		return ph[0].Number == block.Header.Number
	})).Once()

	runtime.OnBlockInserted(block)

	commitmentMsgFromDB, err := state.getCommitmentMessage(toIndex)
	assert.NoError(t, err)
	assert.Equal(t, fromIndex, commitmentMsgFromDB.Message.FromIndex)
	assert.Equal(t, toIndex, commitmentMsgFromDB.Message.ToIndex)
	assert.Equal(t, trie.Hash(), commitmentMsgFromDB.Message.MerkleRootHash)
	assert.NotNil(t, commitmentMsgFromDB.AggSignature)

	proof, err := state.getStateSyncProof(fromIndex)
	assert.NoError(t, err)
	assert.Equal(t, fromIndex, proof.StateSync.ID)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_getExitEventRootHash(t *testing.T) {
	t.Parallel()

	const (
		numOfBlocks         = 10
		numOfEventsPerBlock = 2
	)

	state := newTestState(t)
	runtime := &consensusRuntime{
		state: state,
	}

	encodedEvents := setupExitEventsForProofVerification(t, state, numOfBlocks, numOfEventsPerBlock)

	t.Run("Get exit event root hash", func(t *testing.T) {
		t.Parallel()

		tree, err := NewMerkleTree(encodedEvents)
		require.NoError(t, err)

		hash, err := runtime.BuildEventRoot(1)
		require.NoError(t, err)
		require.Equal(t, tree.Hash(), hash)
	})

	t.Run("Get exit event root hash - no events", func(t *testing.T) {
		t.Parallel()

		hash, err := runtime.BuildEventRoot(2)
		require.NoError(t, err)
		require.Equal(t, types.Hash{}, hash)
	})
}

func TestConsensusRuntime_GenerateExitProof(t *testing.T) {
	t.Parallel()

	const (
		numOfBlocks         = 10
		numOfEventsPerBlock = 2
	)

	state := newTestState(t)
	runtime := &consensusRuntime{
		state: state,
	}

	encodedEvents := setupExitEventsForProofVerification(t, state, numOfBlocks, numOfEventsPerBlock)
	checkpointEvents := encodedEvents[:numOfEventsPerBlock]

	// manually create merkle tree for a desired checkpoint to verify the generated proof
	tree, err := NewMerkleTree(checkpointEvents)
	require.NoError(t, err)

	proof, err := runtime.GenerateExitProof(1, 1, 1)
	require.NoError(t, err)
	require.NotNil(t, proof)

	t.Run("Generate and validate exit proof", func(t *testing.T) {
		t.Parallel()
		// verify generated proof on desired tree
		require.NoError(t, VerifyProof(1, encodedEvents[1], proof.Proof, tree.Hash()))
	})

	t.Run("Generate and validate exit proof - invalid proof", func(t *testing.T) {
		t.Parallel()

		// copy and make proof invalid
		invalidProof := make([]types.Hash, len(proof.Proof))
		copy(invalidProof, proof.Proof)
		invalidProof[0][0]++

		// verify generated proof on desired tree
		require.ErrorContains(t, VerifyProof(1, encodedEvents[1], invalidProof, tree.Hash()), "not a member of merkle tree")
	})

	t.Run("Generate exit proof - no event", func(t *testing.T) {
		t.Parallel()

		_, err := runtime.GenerateExitProof(21, 1, 1)
		require.ErrorContains(t, err, "could not find any exit event that has an id")
	})
}

func TestConsensusRuntime_IsValidSender(t *testing.T) {
	t.Parallel()

	validatorAccounts := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})

	extra := &Extra{}
	lastBuildBlock := &types.Header{
		Number:    0,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()

	state := newTestState(t)
	snapshot := NewProposerSnapshot(0, nil)
	config := &runtimeConfig{
		Key:           validatorAccounts.getValidator("B").Key(),
		blockchain:    blockchainMock,
		PolyBFTConfig: &PolyBFTConfig{EpochSize: 10, SprintSize: 5},
	}
	runtime := &consensusRuntime{
		state:          state,
		config:         config,
		lastBuiltBlock: lastBuildBlock,
		epoch: &epochMetadata{
			Number:     1,
			Validators: validatorAccounts.getPublicIdentities()[:len(validatorAccounts.validators)-1],
		},
		logger:             hclog.NewNullLogger(),
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}

	require.NoError(t, runtime.FSM())

	sender := validatorAccounts.getValidator("A")
	msg, err := sender.Key().SignEcdsaMessage(&proto.Message{
		From: sender.Address().Bytes(),
	})

	require.NoError(t, err)

	assert.True(t, runtime.IsValidSender(msg))
	blockchainMock.AssertExpectations(t)

	// sender not in current epoch validators
	sender = validatorAccounts.getValidator("F")
	msg, err = sender.Key().SignEcdsaMessage(&proto.Message{
		From: sender.Address().Bytes(),
	})

	require.NoError(t, err)

	assert.False(t, runtime.IsValidSender(msg))
	blockchainMock.AssertExpectations(t)

	// signature does not come from sender
	sender = validatorAccounts.getValidator("A")
	msg, err = sender.Key().SignEcdsaMessage(&proto.Message{
		From: validatorAccounts.getValidator("B").Address().Bytes(),
	})

	require.NoError(t, err)

	assert.False(t, runtime.IsValidSender(msg))
	blockchainMock.AssertExpectations(t)

	// invalid signature
	sender = validatorAccounts.getValidator("A")
	msg = &proto.Message{
		From:      sender.Address().Bytes(),
		Signature: []byte{1, 2},
	}

	assert.False(t, runtime.IsValidSender(msg))
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_IsValidProposalHash(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber: 1,
			BlockRound:  1,
		},
	}
	block := &types.Block{
		Header: &types.Header{
			Number:    10,
			ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
		},
	}
	block.Header.ComputeHash()

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	require.True(t, runtime.IsValidProposalHash(block.MarshalRLP(), proposalHash.Bytes()))
}

func TestConsensusRuntime_IsValidProposalHash_InvalidProposalHash(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber: 1,
			BlockRound:  1,
		},
	}

	block := &types.Block{
		Header: &types.Header{
			Number:    10,
			ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
		},
	}

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	extra.Checkpoint.BlockRound = 2 // change it so it is not the same as in proposal hash
	block.Header.ExtraData = append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...)
	block.Header.ComputeHash()

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	require.False(t, runtime.IsValidProposalHash(block.MarshalRLP(), proposalHash.Bytes()))
}

func TestConsensusRuntime_IsValidProposalHash_InvalidExtra(t *testing.T) {
	t.Parallel()

	extra := &Extra{
		Checkpoint: &CheckpointData{
			EpochNumber: 1,
			BlockRound:  1,
		},
	}

	block := &types.Block{
		Header: &types.Header{
			Number:    10,
			ExtraData: []byte{1, 2, 3}, // invalid extra in block
		},
	}
	block.Header.ComputeHash()

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	require.False(t, runtime.IsValidProposalHash(block.MarshalRLP(), proposalHash.Bytes()))
}

func TestConsensusRuntime_BuildProposal_InvalidParent(t *testing.T) {
	config := &runtimeConfig{}
	snapshot := NewProposerSnapshot(1, nil)
	runtime := &consensusRuntime{
		logger:             hclog.NewNullLogger(),
		lastBuiltBlock:     &types.Header{Number: 2},
		epoch:              &epochMetadata{Number: 1},
		config:             config,
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}

	require.Nil(t, runtime.BuildProposal(&proto.View{Height: 5, Round: 1}))
}

func TestConsensusRuntime_ID(t *testing.T) {
	t.Parallel()

	key1, key2 := createTestKey(t), createTestKey(t)
	runtime := &consensusRuntime{
		config: &runtimeConfig{Key: key1},
	}

	require.Equal(t, runtime.ID(), key1.Address().Bytes())
	require.NotEqual(t, runtime.ID(), key2.Address().Bytes())
}

func TestConsensusRuntime_HasQuorum(t *testing.T) {
	t.Parallel()

	const round = 5

	validatorAccounts := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})

	extra := &Extra{
		Checkpoint: &CheckpointData{},
	}

	lastBuildBlock := &types.Header{
		Number:    1,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(&types.Header{
		Number: 0,
	}, true).Once()

	state := newTestState(t)
	snapshot := NewProposerSnapshot(lastBuildBlock.Number+1, validatorAccounts.getPublicIdentities())
	config := &runtimeConfig{
		Key:           validatorAccounts.getValidator("B").Key(),
		blockchain:    blockchainMock,
		PolyBFTConfig: &PolyBFTConfig{EpochSize: 10, SprintSize: 5},
	}
	runtime := &consensusRuntime{
		state:          state,
		config:         config,
		lastBuiltBlock: lastBuildBlock,
		epoch: &epochMetadata{
			Number:     1,
			Validators: validatorAccounts.getPublicIdentities()[:len(validatorAccounts.validators)-1],
		},
		logger:             hclog.NewNullLogger(),
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
	}

	require.NoError(t, runtime.FSM())

	proposer, err := snapshot.CalcProposer(round, lastBuildBlock.Number+1)
	require.NoError(t, err)

	runtime.fsm.proposerSnapshot = snapshot

	messages := make([]*proto.Message, 0, len(validatorAccounts.validators))

	// Unknown message type
	assert.False(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages, -1))

	// invalid block number
	for _, msgType := range []proto.MessageType{proto.MessageType_PREPREPARE, proto.MessageType_PREPARE,
		proto.MessageType_ROUND_CHANGE, proto.MessageType_COMMIT} {
		assert.False(t, runtime.HasQuorum(lastBuildBlock.Number, nil, msgType))
	}

	// MessageType_PREPREPARE - only one message is enough
	messages = append(messages, &proto.Message{
		From: proposer.Bytes(),
		Type: proto.MessageType_PREPREPARE,
		View: &proto.View{Height: 1, Round: round},
	})

	assert.False(t, runtime.HasQuorum(lastBuildBlock.Number+1, nil, proto.MessageType_PREPREPARE))
	assert.True(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages, proto.MessageType_PREPREPARE))

	// MessageType_PREPARE
	messages = make([]*proto.Message, 0, len(validatorAccounts.validators))

	for _, x := range validatorAccounts.validators {
		address := x.Address()

		// proposer must not be included in prepare messages
		if address != proposer {
			messages = append(messages, &proto.Message{
				From: address[:],
				Type: proto.MessageType_PREPARE,
				View: &proto.View{Height: lastBuildBlock.Number + 1, Round: round},
			})
		}
	}

	// enough quorum
	assert.True(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages, proto.MessageType_PREPARE))

	// not enough quorum
	assert.False(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages[:1], proto.MessageType_PREPARE))

	// include proposer which is not allowed
	messages = append(messages, &proto.Message{
		From: proposer[:],
		Type: proto.MessageType_PREPARE,
		View: &proto.View{Height: lastBuildBlock.Number + 1, Round: round},
	})
	assert.False(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages, proto.MessageType_PREPARE))

	// last message is MessageType_PREPREPARE - this should be allowed
	messages[len(messages)-1].Type = proto.MessageType_PREPREPARE
	assert.True(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages, proto.MessageType_PREPARE))

	//proto.MessageType_ROUND_CHANGE, proto.MessageType_COMMIT
	for _, msgType := range []proto.MessageType{proto.MessageType_ROUND_CHANGE, proto.MessageType_COMMIT} {
		messages = make([]*proto.Message, 0, len(validatorAccounts.validators))

		for _, x := range validatorAccounts.validators {
			messages = append(messages, &proto.Message{
				From: x.Address().Bytes(),
				Type: msgType,
				View: &proto.View{Height: lastBuildBlock.Number + 1, Round: round},
			})
		}

		assert.True(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages, msgType))
		assert.False(t, runtime.HasQuorum(lastBuildBlock.Number+1, messages[:1], msgType))
	}
}

func TestConsensusRuntime_BuildRoundChangeMessage(t *testing.T) {
	t.Parallel()

	key := createTestKey(t)
	view, proposal, certificate := &proto.View{}, []byte{1}, &proto.PreparedCertificate{}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{RoundChangeData: &proto.RoundChangeMessage{
			LastPreparedProposedBlock: proposal,
			LatestPreparedCertificate: certificate,
		}},
	}

	signedMsg, err := key.SignEcdsaMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildRoundChangeMessage(proposal, certificate, view))
}

func TestConsensusRuntime_BuildCommitMessage(t *testing.T) {
	t.Parallel()

	key := createTestKey(t)
	view, proposalHash := &proto.View{}, []byte{1, 2, 4}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	committedSeal, err := key.Sign(proposalHash)
	require.NoError(t, err)

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}

	signedMsg, err := key.SignEcdsaMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildCommitMessage(proposalHash, view))
}

func TestConsensusRuntime_BuildPrePrepareMessage_EmptyProposal(t *testing.T) {
	t.Parallel()

	runtime := &consensusRuntime{logger: hclog.NewNullLogger()}

	assert.Nil(t, runtime.BuildPrePrepareMessage(nil, &proto.RoundChangeCertificate{}, &proto.View{Height: 1, Round: 0}))
}

func TestConsensusRuntime_IsValidProposalHash_EmptyProposal(t *testing.T) {
	t.Parallel()

	runtime := &consensusRuntime{logger: hclog.NewNullLogger()}

	assert.False(t, runtime.IsValidProposalHash(nil, []byte("hash")))
}

func TestConsensusRuntime_BuildPrepareMessage(t *testing.T) {
	t.Parallel()

	key := createTestKey(t)
	view, proposalHash := &proto.View{}, []byte{1, 2, 4}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}

	signedMsg, err := key.SignEcdsaMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildPrepareMessage(proposalHash, view))
}

func createTestTransportMessage(t *testing.T, hash []byte, epochNumber uint64, key *wallet.Key) *TransportMessage {
	t.Helper()

	signature, _ := key.Sign(hash)

	return &TransportMessage{
		Hash:        hash,
		Signature:   signature,
		NodeID:      key.String(),
		EpochNumber: epochNumber,
	}
}

func createTestMessageVote(t *testing.T, hash []byte, validator *testValidator) *MessageSignature {
	t.Helper()

	signature, err := validator.mustSign(hash).Marshal()
	require.NoError(t, err)

	return &MessageSignature{
		From:      validator.Key().String(),
		Signature: signature,
	}
}

func createTestBlocks(t *testing.T, numberOfBlocks, defaultEpochSize uint64,
	validatorSet AccountSet) (*types.Header, *testHeadersMap) {
	t.Helper()

	headerMap := &testHeadersMap{}
	bitmaps := createTestBitmaps(t, validatorSet, numberOfBlocks)

	extra := &Extra{
		Checkpoint: &CheckpointData{EpochNumber: 0},
	}

	genesisBlock := &types.Header{
		Number:    0,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}
	parentHash := types.BytesToHash(big.NewInt(0).Bytes())

	headerMap.addHeader(genesisBlock)

	var hash types.Hash

	var blockHeader *types.Header

	for i := uint64(1); i <= numberOfBlocks; i++ {
		big := big.NewInt(int64(i))
		hash = types.BytesToHash(big.Bytes())

		header := &types.Header{
			Number:     i,
			ParentHash: parentHash,
			ExtraData:  createTestExtraForAccounts(t, getEpochNumber(t, i, defaultEpochSize), validatorSet, bitmaps[i]),
			GasLimit:   types.StateTransactionGasLimit,
		}

		headerMap.addHeader(header)

		parentHash = hash
		blockHeader = header
	}

	return blockHeader, headerMap
}

func createTestBitmaps(t *testing.T, validators AccountSet, numberOfBlocks uint64) map[uint64]bitmap.Bitmap {
	t.Helper()

	bitmaps := make(map[uint64]bitmap.Bitmap, numberOfBlocks)

	rand.Seed(time.Now().Unix())

	for i := numberOfBlocks; i > 1; i-- {
		bitmap := bitmap.Bitmap{}
		j := 0

		for j != 3 {
			validator := validators[rand.Intn(validators.Len())]
			index := uint64(validators.Index(validator.Address))

			if !bitmap.IsSet(index) {
				bitmap.Set(index)
				j++
			}
		}

		bitmaps[i] = bitmap
	}

	return bitmaps
}

func createTestExtraForAccounts(t *testing.T, epoch uint64, validators AccountSet, b bitmap.Bitmap) []byte {
	t.Helper()

	dummySignature := [64]byte{}
	extraData := Extra{
		Validators: &ValidatorSetDelta{
			Added:   validators,
			Removed: bitmap.Bitmap{},
		},
		Parent:     &Signature{Bitmap: b, AggregatedSignature: dummySignature[:]},
		Committed:  &Signature{Bitmap: b, AggregatedSignature: dummySignature[:]},
		Checkpoint: &CheckpointData{EpochNumber: epoch},
	}

	marshaled := extraData.MarshalRLPTo(nil)
	result := make([]byte, ExtraVanity+len(marshaled))

	copy(result[ExtraVanity:], marshaled)

	return result
}

func setupExitEventsForProofVerification(t *testing.T, state *State,
	numOfBlocks, numOfEventsPerBlock uint64) [][]byte {
	t.Helper()

	encodedEvents := make([][]byte, numOfBlocks*numOfEventsPerBlock)
	index := uint64(0)

	for i := uint64(1); i <= numOfBlocks; i++ {
		for j := uint64(1); j <= numOfEventsPerBlock; j++ {
			e := &ExitEvent{index, ethgo.ZeroAddress, ethgo.ZeroAddress, []byte{0, 1}, 1, i}
			require.NoError(t, state.insertExitEvent(e))

			b, err := ExitEventABIType.Encode(e)

			require.NoError(t, err)

			encodedEvents[index] = b
			index++
		}
	}

	return encodedEvents
}

func insertTestStateSyncEvents(t *testing.T, numberOfEvents int, startIndex uint64, state *State) []*types.StateSyncEvent {
	t.Helper()

	stateSyncs := generateStateSyncEvents(t, numberOfEvents, startIndex)
	for _, stateSync := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(stateSync))
	}

	return stateSyncs
}
