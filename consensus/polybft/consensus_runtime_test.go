package polybft

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func TestConsensusRuntime_GetVotes(t *testing.T) {
	const (
		epoch           = uint64(1)
		validatorsCount = 7
		bundleSize      = 5
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

	commitment, _, _ := buildCommitmentAndStateSyncs(t, stateSyncsCount, epoch, bundleSize, 0)

	quorumSize := uint(getQuorumSize(len(runtime.epoch.Validators)))
	require.NoError(t, state.insertEpoch(epoch))

	votesCount := quorumSize + 1
	hash, err := commitment.Hash()
	require.NoError(t, err)

	for i := 0; i < int(votesCount); i++ {
		validator := validatorAccounts.getValidator(validatorIds[i])
		signature := validator.mustSign(hash.Bytes())

		_, err := state.insertMessageVote(epoch, hash.Bytes(),
			&MessageSignature{
				From:      validator.Key().NodeID(),
				Signature: signature,
			})
		require.NoError(t, err)
	}

	votes, err := runtime.state.getMessageVotes(runtime.epoch.Number, hash.Bytes())
	require.NoError(t, err)
	require.Len(t, votes, int(votesCount))
}

func TestConsensusRuntime_GetVotesError(t *testing.T) {
	const (
		epoch           = uint64(1)
		stateSyncsCount = 30
		startIndex      = 0
		bundleSize      = uint64(5)
	)

	state := newTestState(t)
	runtime := &consensusRuntime{state: state}
	commitment, _, _ := buildCommitmentAndStateSyncs(t, 5, epoch, bundleSize, startIndex)
	hash, err := commitment.Hash()
	require.NoError(t, err)
	_, err = runtime.state.getMessageVotes(epoch, hash.Bytes())
	assert.ErrorContains(t, err, "could not find")
}

func TestConsensusRuntime_deliverMessage_MessageWhenEpochNotStarted(t *testing.T) {
	const epoch = uint64(5)

	validatorIds := []string{"A", "B", "C", "D", "E", "F", "G"}
	state := newTestState(t)
	validators := newTestValidatorsWithAliases(validatorIds)
	localValidator := validators.getValidator("A")
	runtime := &consensusRuntime{
		logger:              newTestLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config:              &runtimeConfig{Key: localValidator.Key()},
		epoch: &epochMetadata{
			Number:     epoch,
			Validators: validators.getPublicIdentities(),
		},
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
	msgProcessed, err := runtime.deliverMessage(createTestTransportMessage(t, hash, epoch, validators.getValidator(senderID).Key()))
	require.NoError(t, err)
	require.True(t, msgProcessed)

	// assert that no additional message signatures aren't inserted into the consensus runtime state
	// (other than the one we have previously inserted by ourselves)
	signatures, err := runtime.state.getMessageVotes(epoch, hash)
	require.NoError(t, err)
	require.Len(t, signatures, 2)
}

func TestConsensusRuntime_AddLog(t *testing.T) {
	state := newTestState(t)
	runtime := &consensusRuntime{
		logger: newTestLogger(),
		state:  state,
		config: &runtimeConfig{Key: createTestKey(t)},
	}
	topics := make([]ethgo.Hash, 4)
	topics[0] = stateTransferEvent.ID()
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
	event, err := decodeEvent(log)
	require.NoError(t, err)
	runtime.AddLog(log)

	stateSyncs, err := runtime.state.getStateSyncEventsForCommitment(1, 1)
	require.NoError(t, err)
	require.Len(t, stateSyncs, 1)
	require.Equal(t, event.ID, stateSyncs[0].ID)
}

func TestConsensusRuntime_getQuorumSize(t *testing.T) {
	var cases = []struct {
		num, quorum int
	}{
		{4, 2},
		{5, 3},
	}

	for _, c := range cases {
		assert.Equal(t, c.quorum, getQuorumSize(c.num))
	}
}

func TestConsensusRuntime_isEndOfEpoch_NotReachedEnd(t *testing.T) {
	var cases = []struct {
		epochSize, parentBlockNumber uint64
	}{
		{4, 2},
		{5, 3},
		{6, 6},
		{7, 7},
		{8, 8},
		{9, 9},
		{10, 10},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{},
		},
		lastBuiltBlock: &types.Header{},
	}

	for _, c := range cases {
		runtime.lastBuiltBlock.Number = c.parentBlockNumber
		runtime.config.PolyBFTConfig.EpochSize = c.epochSize
		assert.False(
			t,
			runtime.isEndOfEpoch(runtime.getPendingBlockNumber()),
			fmt.Sprintf(
				"Not expected end of epoch for epoch size=%v and parent block number=%v",
				c.epochSize,
				c.parentBlockNumber),
		)
	}
}

func TestConsensusRuntime_isEndOfEpoch_ReachedEnd(t *testing.T) {
	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
			},
		},
		lastBuiltBlock: &types.Header{
			Number: 9,
		},
	}
	assert.True(t, runtime.isEndOfEpoch(runtime.getPendingBlockNumber()))
}

func TestConsensusRuntime_isEndOfEpoch_Block0(t *testing.T) {
	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
			},
		},
		lastBuiltBlock: &types.Header{
			Number: 0,
		},
	}
	assert.False(t, runtime.isEndOfEpoch(runtime.getPendingBlockNumber()))
}

func TestConsensusRuntime_isEndOfSprint_NotReachedEnd(t *testing.T) {
	var cases = []struct {
		sprintSize, parentBlockNumber uint64
	}{
		{4, 2},
		{5, 3},
		{6, 6},
		{7, 7},
		{8, 8},
		{9, 9},
		{10, 10},
	}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{},
		},
		lastBuiltBlock: &types.Header{},
	}

	for _, c := range cases {
		runtime.lastBuiltBlock.Number = c.parentBlockNumber
		runtime.config.PolyBFTConfig.SprintSize = c.sprintSize
		assert.False(t,
			runtime.isEndOfSprint(runtime.getPendingBlockNumber()),
			fmt.Sprintf(
				"Not expected end of sprint for sprint size=%v and parent block number=%v",
				c.sprintSize,
				c.parentBlockNumber),
		)
	}
}

func TestConsensusRuntime_isEndOfSprint_ReachedEnd(t *testing.T) {
	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
			},
		},
		lastBuiltBlock: &types.Header{
			Number: 4,
		},
	}
	assert.True(t, runtime.isEndOfSprint(runtime.getPendingBlockNumber()))
}

func TestConsensusRuntime_isEndOfSprint_Block0(t *testing.T) {
	runtime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
			},
		},
		lastBuiltBlock: &types.Header{
			Number: 0,
		},
	}
	assert.False(t, runtime.isEndOfSprint(runtime.getPendingBlockNumber()))
}

func TestConsensusRuntime_deliverMessage_EpochNotStarted(t *testing.T) {
	state := newTestState(t)
	err := state.insertEpoch(1)
	assert.NoError(t, err)

	// random node not among validator set
	account := newTestValidator("A")

	runtime := &consensusRuntime{
		logger: newTestLogger(),
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
	}

	msg := createTestTransportMessage(t, generateRandomBytes(t), 1, account.Key())
	isProcessed, err := runtime.deliverMessage(msg)
	assert.False(t, isProcessed)
	assert.ErrorContains(t, err, "not among the active validator set")

	votes, err := state.getMessageVotes(1, msg.Hash)
	assert.NoError(t, err)
	assert.Empty(t, votes)
}

func TestConsensusRuntime_deliverMessage_ForExistingEpochAndCommitmentMessage(t *testing.T) {
	state := newTestState(t)
	err := state.insertEpoch(1)
	require.NoError(t, err)

	validators := newTestValidatorsWithAliases([]string{"SENDER", "RECEIVER"})
	validatorSet := validators.getPublicIdentities()
	sender := validators.getValidator("SENDER").Key()

	runtime := &consensusRuntime{
		logger:              newTestLogger(),
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
	}

	msg := createTestTransportMessage(t, generateRandomBytes(t), 1, sender)
	isProcessed, err := runtime.deliverMessage(msg)
	assert.True(t, isProcessed)
	assert.NoError(t, err)

	votes, err := state.getMessageVotes(1, msg.Hash)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(votes))
	assert.True(t, bytes.Equal(msg.Signature, votes[0].Signature))
}

func TestConsensusRuntime_deliverMessage_SenderMessageNotInCurrentValidatorset(t *testing.T) {
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
	}

	msg := createTestTransportMessage(t, generateRandomBytes(t), 1, createTestKey(t))
	isProcessed, err := runtime.deliverMessage(msg)
	assert.False(t, isProcessed)
	assert.Error(t, err)
	assert.ErrorContains(t, err,
		fmt.Sprintf("message is received from sender %s, which is not in current validator set", msg.NodeID))
}

func TestConsensusRuntime_NotifyProposalInserted_EndOfEpoch(t *testing.T) {
	const (
		epochSize       = uint64(10)
		validatorsCount = 7
	)

	header := &types.Header{Number: epochSize}
	builtBlock := &StateBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{
		Header: header,
	})}

	validatorSet := newTestValidators(validatorsCount).getPublicIdentities()

	currentEpochNumber := getEpochNumber(header.Number, epochSize)
	newEpochNumber := currentEpochNumber + 1
	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(newEpochNumber).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorSet).Once()

	runtime := &consensusRuntime{
		logger: newTestLogger(),
		state:  newTestState(t),
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize: epochSize,
			},
			blockchain:     blockchainMock,
			polybftBackend: polybftBackendMock,
		},
		epoch: &epochMetadata{
			Number: currentEpochNumber,
		},
	}
	runtime.NotifyProposalInserted(builtBlock)

	require.True(t, runtime.state.isEpochInserted(currentEpochNumber+1))
	require.Equal(t, newEpochNumber, runtime.epoch.Number)

	blockchainMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_NotifyProposalInserted_MiddleOfEpoch(t *testing.T) {
	const (
		epochSize   = uint64(10)
		blockNumber = epochSize + 2
	)

	header := &types.Header{Number: blockNumber}
	builtBlock := &StateBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{
		Header: header,
	})}

	runtime := &consensusRuntime{
		lastBuiltBlock: header,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{EpochSize: epochSize},
			blockchain:    new(blockchainMock)},
	}
	runtime.NotifyProposalInserted(builtBlock)

	require.Equal(t, header.Number, runtime.lastBuiltBlock.Number)
}

func TestConsensusRuntime_FSM_NotInValidatorSet(t *testing.T) {
	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D"})
	runtime := &consensusRuntime{
		activeValidatorFlag: 1,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize: 1,
			},
			Key: createTestKey(t),
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.getPublicIdentities(),
		},
	}

	_, err := runtime.FSM()
	assert.ErrorIs(t, err, errNotAValidator)
}

func TestConsensusRuntime_FSM_NotEndOfEpoch_NotEndOfSprint(t *testing.T) {
	lastBlock := &types.Header{Number: 1}

	validators := newTestValidators(3)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()

	runtime := &consensusRuntime{
		logger:              newTestLogger(),
		activeValidatorFlag: 1,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
			},
			Key:        wallet.NewKey(validators.getPrivateIdentities()[0]),
			blockchain: blockchainMock,
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.getPublicIdentities(),
		},
		lastBuiltBlock: lastBlock,
	}

	fsm, err := runtime.FSM()
	assert.NoError(t, err)

	assert.True(t, runtime.isActiveValidator())
	assert.False(t, fsm.isEndOfEpoch)
	assert.False(t, fsm.isEndOfSprint)
	assert.True(t, fsm.ValidatorSet().Includes(runtime.config.Key.NodeID()))
	assert.Equal(t, lastBlock.Number, fsm.parent.Number)
	assert.Equal(t, runtime.epoch.Number, fsm.epoch)

	assert.NotNil(t, fsm.blockBuilder)
	assert.NotNil(t, fsm.backend)

	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_FSM_EndOfEpoch_BuildRegisterCommitment_And_Uptime(t *testing.T) {
	const (
		epoch               = 0
		epochSize           = uint64(10)
		sprintSize          = uint64(3)
		beginStateSyncIndex = uint64(0)
		bundleSize          = stateSyncBundleSize
		fromIndex           = uint64(0)
		toIndex             = uint64(9)
	)

	validatorAccounts := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccounts.getPublicIdentities()
	accounts := validatorAccounts.getPrivateIdentities()

	lastBuiltBlock, headerMap := createTestBlocksForUptime(t, 9, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextExecutionIndex").Return(beginStateSyncIndex, nil).Once()
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(epoch))

	stateSyncs := generateStateSyncEvents(t, bundleSize, 0)
	for _, event := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(event))
	}

	trie, err := createMerkleTree(stateSyncs, bundleSize)
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
			From:      pbft.NodeID(a.Ecdsa.Address().String()),
			Signature: signatureRaw,
		})
		require.NoError(t, err)
	}

	metadata := &epochMetadata{
		Validators: validators,
		Number:     epoch,
		Commitment: commitment,
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

	runtime := &consensusRuntime{
		logger:         newTestLogger(),
		state:          state,
		epoch:          metadata,
		config:         config,
		lastBuiltBlock: lastBuiltBlock,
	}

	fsm, err := runtime.FSM()
	assert.NoError(t, err)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.uptimeCounter)
	assert.NotEmpty(t, fsm.uptimeCounter)
	assert.NotNil(t, fsm.proposerCommitmentToRegister)
	assert.Equal(t, fromIndex, fsm.proposerCommitmentToRegister.Message.FromIndex)
	assert.Equal(t, toIndex, fsm.proposerCommitmentToRegister.Message.ToIndex)
	assert.Equal(t, uint64(bundleSize), fsm.proposerCommitmentToRegister.Message.BundleSize)
	assert.Equal(t, trie.Hash(), fsm.proposerCommitmentToRegister.Message.MerkleRootHash)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_FSM_EndOfEpoch_RegisterCommitmentNotFound(t *testing.T) {
	const (
		epochSize           = uint64(10)
		sprintSize          = uint64(5)
		beginStateSyncIndex = uint64(5)
	)

	validatorAccs := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccs.getPublicIdentities()
	lastBuiltBlock, headerMap := createTestBlocksForUptime(t, 9, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextExecutionIndex").Return(beginStateSyncIndex, nil).Once()
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(new(blockBuilderMock), nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	metadata := &epochMetadata{
		Validators: validators,
		Number:     getEpochNumber(lastBuiltBlock.Number, epochSize),
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

	runtime := &consensusRuntime{
		logger:         newTestLogger(),
		epoch:          metadata,
		config:         config,
		lastBuiltBlock: lastBuiltBlock,
		state:          newTestState(t),
	}

	fsm, err := runtime.FSM()

	require.NoError(t, err)
	require.NotNil(t, fsm)
	require.Nil(t, fsm.proposerCommitmentToRegister)
	require.True(t, fsm.isEndOfEpoch)
	require.NotNil(t, fsm.uptimeCounter)
	require.NotEmpty(t, fsm.uptimeCounter)
}

func TestConsensusRuntime_FSM_EndOfEpoch_BuildRegisterCommitment_QuorumNotReached(t *testing.T) {
	const (
		epoch               = 0
		epochSize           = uint64(10)
		sprintSize          = uint64(5)
		beginStateSyncIndex = uint64(0)
		bundleSize          = stateSyncBundleSize
	)

	validatorAccs := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccs.getPublicIdentities()
	lastBuiltBlock, headerMap := createTestBlocksForUptime(t, 9, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextExecutionIndex").Return(beginStateSyncIndex, nil).Once()
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(epoch))

	stateSyncs := generateStateSyncEvents(t, bundleSize, 0)
	for _, event := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(event))
	}

	trie, err := createMerkleTree(stateSyncs, bundleSize)
	require.NoError(t, err)

	commitment := &Commitment{MerkleTree: trie, Epoch: epoch}

	hash, err := commitment.Hash()
	require.NoError(t, err)

	validatorKey := validatorAccs.getValidator("C").Key()
	signature, err := validatorKey.Sign(hash.Bytes())
	require.NoError(t, err)
	_, err = state.insertMessageVote(epoch, hash.Bytes(), &MessageSignature{
		From:      pbft.NodeID(validators[0].Address.String()),
		Signature: signature,
	})
	require.NoError(t, err)

	metadata := &epochMetadata{
		Validators: validators,
		Number:     epoch,
		Commitment: commitment,
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

	runtime := &consensusRuntime{
		logger:         newTestLogger(),
		state:          state,
		epoch:          metadata,
		config:         config,
		lastBuiltBlock: lastBuiltBlock,
	}

	fsm, err := runtime.FSM()
	assert.NoError(t, err)
	assert.Nil(t, fsm.proposerCommitmentToRegister)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.uptimeCounter)
	assert.NotEmpty(t, fsm.uptimeCounter)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func Test_NewConsensusRuntime(t *testing.T) {
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
	}

	key := createTestKey(t)

	tmpDir := t.TempDir()
	config := &runtimeConfig{
		PolyBFTConfig: polyBftConfig,
		DataDir:       tmpDir,
		Key:           key,
		blockchain:    &blockchainMock{},
	}
	runtime, err := newConsensusRuntime(newTestLogger(), config)
	assert.NoError(t, err)

	assert.False(t, runtime.isActiveValidator())
	assert.Equal(t, runtime.config.DataDir, tmpDir)
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.SprintSize)
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.EpochSize)
	assert.Equal(t, "0x1100000000000000000000000000000000000000", runtime.config.PolyBFTConfig.ValidatorSetAddr.String())
	assert.Equal(t, "0x1300000000000000000000000000000000000000", runtime.config.PolyBFTConfig.Bridge.BridgeAddr.String())
	assert.Equal(t, "0x1000000000000000000000000000000000000000", runtime.config.PolyBFTConfig.Bridge.CheckpointAddr.String())
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.EpochSize)
	assert.True(t, runtime.IsBridgeEnabled())
}

func TestConsensusRuntime_FSM_EndOfSprint_HasBundlesToExecute(t *testing.T) {
	const (
		epochNumber        = uint64(2)
		fromIndex          = uint64(5)
		nextCommittedIndex = uint64(3)
		stateSyncsCount    = 30
		bundleSize         = stateSyncsCount / 3
	)

	state := newTestState(t)
	err := state.insertEpoch(epochNumber)
	require.NoError(t, err)

	stateSyncs := insertTestStateSyncEvents(t, stateSyncsCount, fromIndex, state)

	commitment, err := NewCommitment(epochNumber, fromIndex, fromIndex+bundleSize, bundleSize, stateSyncs)
	require.NoError(t, err)

	commitmentMsg := NewCommitmentMessage(commitment.MerkleTree.Hash(), fromIndex,
		fromIndex+bundleSize, bundleSize)
	signedCommitmentMsg := &CommitmentMessageSigned{
		Message:      commitmentMsg,
		AggSignature: Signature{},
	}
	require.NoError(t, state.insertCommitmentMessage(signedCommitmentMsg))

	validatorAccs := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F", "G"})
	validatorSet := validatorAccs.getPublicIdentities()

	lastBlock := types.Header{Number: 24}

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(nextCommittedIndex, nil).Once()
	systemStateMock.On("GetNextExecutionIndex").Return(uint64(stateSyncMainBundleSize), nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder").Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	runtime := &consensusRuntime{
		logger:              newTestLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
				Bridge: &BridgeConfig{
					BridgeAddr: types.BytesToAddress(big.NewInt(23).Bytes()),
				},
			},
			Key:        validatorAccs.getValidator("A").Key(),
			blockchain: blockchainMock,
		},
		epoch: &epochMetadata{
			Number:     epochNumber,
			Validators: validatorSet,
			Commitment: commitment,
		},
		lastBuiltBlock: &lastBlock,
	}

	require.NoError(t, runtime.buildBundles(runtime.epoch, commitmentMsg, fromIndex))

	fsm, err := runtime.FSM()
	require.NoError(t, err)

	// check if it is end of sprint
	require.True(t, fsm.isEndOfSprint)

	// check if it the correct epoch
	require.Equal(t, epochNumber, fsm.epoch)

	// check if commitment message to execute is attached to fsm
	require.Len(t, fsm.bundleProofs, 1)
	require.Len(t, fsm.commitmentsToVerifyBundles, 1)
	require.Nil(t, fsm.proposerCommitmentToRegister)

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_IsEndOfPeriod(t *testing.T) {
	config := &runtimeConfig{PolyBFTConfig: &PolyBFTConfig{SprintSize: 5, EpochSize: 10}}
	runtime := &consensusRuntime{
		config:         config,
		lastBuiltBlock: &types.Header{},
	}

	var epochCases = []struct {
		blockNumber         uint64
		expectedPeriodEvent bool
	}{
		{0, false},
		{4, false},
		{8, false},
		{9, true},
		{19, true},
		{20, false},
	}

	assertPeriodEvent(
		t,
		epochCases,
		func(_ uint64) bool { return runtime.isEndOfEpoch(runtime.getPendingBlockNumber()) },
		"end of epoch",
		runtime,
	)

	var sprintCases = []struct {
		blockNumber         uint64
		expectedPeriodEvent bool
	}{
		{0, false},
		{4, true},
		{8, false},
		{9, true},
		{19, true},
		{20, false},
	}

	assertPeriodEvent(
		t,
		sprintCases,
		func(_ uint64) bool { return runtime.isEndOfSprint(runtime.getPendingBlockNumber()) },
		"end of sprint",
		runtime,
	)
}

// Helper function which enables us to make assertions on expected period event (begin/end of sprint/epoch)
func assertPeriodEvent(t *testing.T,
	cases []struct {
		blockNumber         uint64
		expectedPeriodEvent bool
	},
	checkPeriodEvent func(blockNumber uint64) bool,
	outcomeName string,
	runtime *consensusRuntime) {
	t.Helper()

	for _, c := range cases {
		runtime.lastBuiltBlock.Number = c.blockNumber
		periodEvent := checkPeriodEvent(c.blockNumber)
		assert.True(t,
			periodEvent == c.expectedPeriodEvent,
			fmt.Sprintf(
				"Unexpected %v for block %d. Expected: %v, got: %v",
				outcomeName,
				c.blockNumber,
				c.expectedPeriodEvent,
				periodEvent),
		)
	}
}

func TestConsensusRuntime_GetEpochNumber(t *testing.T) {
	var cases = []struct {
		blockNumber         uint64
		epochSize           uint64
		expectedEpochNumber uint64
	}{
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{21, 10, 3},

		{0, 23, 0},
		{43, 35, 2},
		{48, 49, 1},
	}

	for _, c := range cases {
		assert.Equal(t, c.expectedEpochNumber, getEpochNumber(c.blockNumber, c.epochSize))
	}
}

func TestConsensusRuntime_GetEndEpochBlockNumber(t *testing.T) {
	var cases = []struct {
		epochNumber   uint64
		epochSize     uint64
		endEpochBlock uint64
	}{
		{0, 10, 0},
		{10, 0, 0},
		{1, 10, 10},
		{10, 10, 100},
		{5, 15, 75},
	}

	for _, c := range cases {
		assert.Equal(t, c.endEpochBlock, getEndEpochBlockNumber(c.epochNumber, c.epochSize))
	}
}

func TestConsensusRuntime_calculateFirstBlockOfPeriod(t *testing.T) {
	var cases = []struct {
		blockNumber    uint64
		periodSize     uint64
		expectedResult uint64
	}{
		{1, 10, 1},
		{11, 10, 11},
		{12, 10, 11},
		{144, 12, 133},
		{5, 10, 1},
		{95, 100, 1},
		{195, 100, 101},
		{20, 10, 11},
	}

	for _, c := range cases {
		actual := calculateFirstBlockOfPeriod(c.blockNumber, c.periodSize)
		assert.Equal(t, c.expectedResult, actual, fmt.Sprintf("Expected: %v. Actual: %v", c.expectedResult, actual))
	}
}

func TestConsensusRuntime_restartEpoch_SameEpochNumberAsTheLastOne(t *testing.T) {
	newCurrentHeader := &types.Header{Number: 6}
	validatorSet := newTestValidators(3).getPublicIdentities()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetEpoch").Return(uint64(1), nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	runtime := &consensusRuntime{
		activeValidatorFlag: 1,
		config: &runtimeConfig{
			blockchain: blockchainMock,
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: validatorSet,
		},
		lastBuiltBlock: &types.Header{
			Number: 5,
		},
	}

	assert.NoError(t, runtime.restartEpoch(newCurrentHeader))
	assert.Equal(t, newCurrentHeader.Number, runtime.lastBuiltBlock.Number)

	for _, a := range validatorSet.GetAddresses() {
		assert.True(t, runtime.epoch.Validators.ContainsAddress(a))
	}

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_FirstRestart_NoStateSyncEvents(t *testing.T) {
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

	runtime := &consensusRuntime{
		logger:              newTestLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config: &runtimeConfig{
			blockchain:     blockchainMock,
			polybftBackend: polybftBackendMock,
			PolyBFTConfig:  &PolyBFTConfig{},
		},
	}

	require.NoError(t, runtime.restartEpoch(newCurrentHeader))
	require.Equal(t, uint64(1), runtime.epoch.Number)
	require.Equal(t, 3, len(runtime.epoch.Validators))
	require.Equal(t, newCurrentHeader.Number, runtime.lastBuiltBlock.Number)
	require.True(t, runtime.isActiveValidator())
	require.True(t, state.isEpochInserted(1))

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_FirstRestart_BuildsCommitment(t *testing.T) {
	const (
		epoch              = uint64(3)
		epochSize          = uint64(10)
		nextCommittedIndex = uint64(10)
		stateSyncsCount    = 20
	)

	header := &types.Header{Number: epoch*epochSize + 3}
	state := newTestState(t)
	stateSyncs := insertTestStateSyncEvents(t, stateSyncsCount, 0, state)
	validatorIds := []string{"A", "B", "C", "D", "E", "F"}
	validatorAccs := newTestValidatorsWithAliases(validatorIds)
	validators := validatorAccs.getPublicIdentities()

	transportMock := new(transportMock)
	transportMock.On("Gossip", mock.Anything).Once()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(nextCommittedIndex, nil).Once()
	systemStateMock.On("GetEpoch").Return(epoch, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators).Once()

	localValidatorID := validatorIds[rand.Intn(len(validatorIds))]
	localValidator := validatorAccs.getValidator(localValidatorID)
	runtime := &consensusRuntime{
		logger:              newTestLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config: &runtimeConfig{
			blockchain:     blockchainMock,
			polybftBackend: polybftBackendMock,
			Transport:      transportMock,
			Key:            localValidator.Key(),
			PolyBFTConfig: &PolyBFTConfig{
				Bridge: &BridgeConfig{},
			},
		},
	}

	require.NoError(t, runtime.restartEpoch(header))
	require.Equal(t, epoch, runtime.epoch.Number)
	require.Equal(t, len(validatorIds), len(runtime.epoch.Validators))
	require.Equal(t, header.Number, runtime.lastBuiltBlock.Number)
	require.True(t, runtime.isActiveValidator())
	require.True(t, state.isEpochInserted(epoch))

	commitment := runtime.epoch.Commitment
	require.NotNil(t, commitment)
	require.Equal(t, epoch, commitment.Epoch)

	commitmentHash, err := commitment.Hash()
	require.NoError(t, err)
	stateSyncsTrie, err := createMerkleTree(stateSyncs[nextCommittedIndex:nextCommittedIndex+stateSyncMainBundleSize], stateSyncBundleSize)
	require.NoError(t, err)
	require.Equal(t, stateSyncsTrie.Hash(), commitment.MerkleTree.Hash())

	votes, err := state.getMessageVotes(epoch, commitmentHash.Bytes())
	require.NoError(t, err)
	require.Equal(t, 1, len(votes))
	require.Equal(t, localValidator.Key().NodeID(), votes[0].From)

	signature := localValidator.mustSign(commitmentHash.Bytes())
	require.Equal(t, signature, votes[0].Signature)

	for _, validator := range validatorAccs.validators {
		if localValidator.Key().NodeID() == validator.Key().NodeID() {
			continue
		}

		signature := validator.mustSign(commitmentHash.Bytes())

		require.NoError(t, err)

		_, err = state.insertMessageVote(runtime.epoch.Number, commitmentHash.Bytes(),
			&MessageSignature{
				From:      validator.Key().NodeID(),
				Signature: signature,
			})
		require.NoError(t, err)
	}

	commitmentMsgSigned, err := runtime.getCommitmentToRegister(runtime.epoch, nextCommittedIndex)
	require.NoError(t, err)
	require.NotNil(t, commitmentMsgSigned)
	require.Equal(t, nextCommittedIndex, commitmentMsgSigned.Message.FromIndex)

	transportMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_NewEpochToRun_BuildCommitment(t *testing.T) {
	const (
		blockNumber        = 11
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

	newValidatorSet[validatorsCount-1] = newTestValidator("G").ValidatorAccount()

	header := &types.Header{Number: blockNumber}

	state := newTestState(t)
	allStateSyncs := insertTestStateSyncEvents(t, 4*stateSyncMainBundleSize, 0, state)

	transportMock := &transportMock{}
	transportMock.On("Gossip", mock.Anything).Once()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(nextCommittedIndex, nil).Once()
	systemStateMock.On("GetEpoch").Return(newEpoch, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(newValidatorSet).Once()

	runtime := &consensusRuntime{
		logger:              newTestLogger(),
		activeValidatorFlag: 1,
		state:               state,
		config: &runtimeConfig{
			PolyBFTConfig: &PolyBFTConfig{
				EpochSize:  10,
				SprintSize: 5,
				Bridge: &BridgeConfig{
					BridgeAddr: types.BytesToAddress(big.NewInt(23).Bytes()),
				},
			},
			Transport:      transportMock,
			Key:            originalValidators.getValidator("A").Key(),
			blockchain:     blockchainMock,
			polybftBackend: polybftBackendMock,
		},
		epoch: &epochMetadata{
			Number:     oldEpoch,
			Validators: originalValidators.getPublicIdentities(),
		},
		lastBuiltBlock: header,
	}

	require.NoError(t, runtime.restartEpoch(header))

	// check new epoch number
	assert.Equal(t, newEpoch, runtime.epoch.Number)

	// check new epoch number
	assert.Equal(t, header.Number, runtime.lastBuiltBlock.Number)

	// check if it is validator
	assert.True(t, runtime.isActiveValidator())

	// check if new epoch is inserted
	assert.True(t, state.isEpochInserted(newEpoch))

	// check if new epoch is created
	assert.NotNil(t, runtime.epoch)

	// check new validators
	assert.Equal(t, len(originalValidatorIds), len(runtime.epoch.Validators))

	for _, a := range newValidatorSet.GetAddresses() {
		assert.True(t, runtime.epoch.Validators.ContainsAddress(a))
	}

	commitment := runtime.epoch.Commitment
	require.NotNil(t, commitment)
	require.Equal(t, newEpoch, commitment.Epoch)

	commitmentHash, err := commitment.Hash()
	require.NoError(t, err)

	for _, validatorID := range originalValidatorIds {
		validator := originalValidators.getValidator(validatorID)
		signature := validator.mustSign(commitmentHash.Bytes())
		_, err := state.insertMessageVote(runtime.epoch.Number, commitmentHash.Bytes(),
			&MessageSignature{
				From:      validator.Key().NodeID(),
				Signature: signature,
			})
		require.NoError(t, err)
	}

	stateSyncTrie, err := createMerkleTree(allStateSyncs[nextCommittedIndex:nextCommittedIndex+stateSyncMainBundleSize], stateSyncBundleSize)
	require.NoError(t, err)
	require.NotNil(t, stateSyncTrie)

	commitmentMsgSigned, err := runtime.getCommitmentToRegister(runtime.epoch, nextCommittedIndex)
	require.NoError(t, err)
	require.NotNil(t, commitmentMsgSigned)
	require.Equal(t, stateSyncTrie.Hash(), commitmentMsgSigned.Message.MerkleRootHash)
	require.Equal(t, nextCommittedIndex, commitmentMsgSigned.Message.FromIndex)
	require.Equal(t, nextCommittedIndex+stateSyncMainBundleSize-1, commitmentMsgSigned.Message.ToIndex)

	transportMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_calculateUptime_EpochSizeToSmall(t *testing.T) {
	config := &PolyBFTConfig{
		ValidatorSetAddr: contracts.ValidatorSetContract,
		EpochSize:        2,
	}

	consensusRuntime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig: config,
		},
		epoch: &epochMetadata{
			Number: 0,
		},
		lastBuiltBlock: &types.Header{Number: 2},
	}

	_, err := consensusRuntime.calculateUptime(consensusRuntime.lastBuiltBlock)
	assert.Error(t, err)
}

func TestConsensusRuntime_calculateUptime_SecondEpoch(t *testing.T) {
	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	config := &PolyBFTConfig{
		ValidatorSetAddr: contracts.ValidatorSetContract,
		EpochSize:        10,
		SprintSize:       5,
	}
	lastBuiltBlock, headerMap := createTestBlocksForUptime(t, 19, validators.getPublicIdentities())

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.getPublicIdentities()).Twice()

	consensusRuntime := &consensusRuntime{
		config: &runtimeConfig{
			PolyBFTConfig:  config,
			blockchain:     blockchainMock,
			polybftBackend: polybftBackendMock,
			Key:            validators.getValidator("A").Key(),
		},
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.getPublicIdentities(),
		},
		lastBuiltBlock: lastBuiltBlock,
	}

	uptime, err := consensusRuntime.calculateUptime(lastBuiltBlock)
	assert.NoError(t, err)
	assert.NotEmpty(t, uptime)

	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_validateVote_VoteSentFromUnknownValidator(t *testing.T) {
	epoch := &epochMetadata{Validators: newTestValidators(5).getPublicIdentities()}
	nonValidatorAccount := createTestKey(t)
	hash := crypto.Keccak256Hash(generateRandomBytes(t)).Bytes()
	// Sign content by non validator account
	signature, err := nonValidatorAccount.Sign(hash)
	require.NoError(t, err)

	vote := &MessageSignature{
		From:      nonValidatorAccount.NodeID(),
		Signature: signature}
	assert.ErrorContains(t, validateVote(vote, epoch),
		fmt.Sprintf("message is received from sender %s, which is not in current validator set", vote.From))
}

func TestConsensusRuntime_buildBundles_NoCommitment(t *testing.T) {
	state := newTestState(t)

	commitmentMsg := NewCommitmentMessage(types.Hash{}, 0, 4, 5)

	runtime := &consensusRuntime{
		logger: newTestLogger(),
		state:  state,
		epoch:  &epochMetadata{Number: 0},
	}

	assert.NoError(t, runtime.buildBundles(runtime.epoch, commitmentMsg, 0))

	bundles, err := state.getBundles(0, 4)

	assert.NoError(t, err)
	assert.Nil(t, bundles)
}

func TestConsensusRuntime_buildBundles(t *testing.T) {
	const (
		epoch                 = 0
		bundleSize            = 5
		fromIndex             = 0
		toIndex               = 4
		expectedBundlesNumber = 1
	)

	state := newTestState(t)
	stateSyncs := insertTestStateSyncEvents(t, bundleSize, 0, state)
	trie, err := createMerkleTree(stateSyncs, bundleSize)
	require.NoError(t, err)

	commitmentMsg := NewCommitmentMessage(trie.Hash(), fromIndex, toIndex, bundleSize)
	commitmentMsgSigned := &CommitmentMessageSigned{
		Message: commitmentMsg,
		AggSignature: Signature{
			Bitmap:              []byte{5, 1},
			AggregatedSignature: []byte{1, 1},
		},
	}
	require.NoError(t, state.insertCommitmentMessage(commitmentMsgSigned))

	runtime := &consensusRuntime{
		logger: newTestLogger(),
		state:  state,
		epoch: &epochMetadata{
			Number: epoch,
			Commitment: &Commitment{
				MerkleTree: trie,
				Epoch:      epoch,
			},
		},
	}

	assert.NoError(t, runtime.buildBundles(runtime.epoch, commitmentMsg, 0))

	bundles, err := state.getBundles(fromIndex, maxBundlesPerSprint)
	assert.NoError(t, err)
	assert.Equal(t, expectedBundlesNumber, len(bundles))
}

func TestConsensusRuntime_FSM_EndOfEpoch_PostHook(t *testing.T) {
	const (
		epoch               = 0
		epochSize           = uint64(10)
		sprintSize          = uint64(5)
		beginStateSyncIndex = uint64(0)
		fromIndex           = uint64(0)
		toIndex             = uint64(9)
	)

	validatorAccounts := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	signingAccounts := validatorAccounts.getPrivateIdentities()
	validators := validatorAccounts.getPublicIdentities()
	lastBuiltBlock, headerMap := createTestBlocksForUptime(t, 9, validators)

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetNextCommittedIndex").Return(beginStateSyncIndex, nil).Once()
	systemStateMock.On("GetNextExecutionIndex").Return(beginStateSyncIndex, nil).Once()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
	blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(epoch))

	stateSyncs := generateStateSyncEvents(t, stateSyncMainBundleSize, 0)
	for _, event := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(event))
	}

	trie, err := createMerkleTree(stateSyncs, stateSyncBundleSize)
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
			From:      pbft.NodeID(a.Ecdsa.Address().String()),
			Signature: signatureRaw,
		})
		require.NoError(t, err)
	}

	metadata := &epochMetadata{
		Validators: validators,
		Number:     epoch,
		Commitment: commitment,
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

	runtime := &consensusRuntime{
		logger:         newTestLogger(),
		state:          state,
		epoch:          metadata,
		config:         config,
		lastBuiltBlock: lastBuiltBlock,
	}

	fsm, err := runtime.FSM()
	assert.NoError(t, err)
	assert.NotNil(t, fsm.proposerCommitmentToRegister)
	assert.Equal(t, fromIndex, fsm.proposerCommitmentToRegister.Message.FromIndex)
	assert.Equal(t, toIndex, fsm.proposerCommitmentToRegister.Message.ToIndex)
	assert.Equal(t, uint64(stateSyncBundleSize), fsm.proposerCommitmentToRegister.Message.BundleSize)
	assert.Equal(t, trie.Hash(), fsm.proposerCommitmentToRegister.Message.MerkleRootHash)
	assert.NotNil(t, fsm.proposerCommitmentToRegister.AggSignature)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.uptimeCounter)
	assert.NotEmpty(t, fsm.uptimeCounter)

	// we add this for NotifyProposalInserted,
	// and we are adding first block so we do not need to mock the restart epoch on block insert
	// since we only care if the commitment data gets saved in db on postHook
	fsm.block = &StateBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{Number: 1},
	})}

	// we registered commitment in fsm
	fsm.commitmentToSaveOnRegister = fsm.proposerCommitmentToRegister

	assert.NoError(t, fsm.postInsertHook())

	commitmentMsgFromDB, err := state.getCommitmentMessage(toIndex)
	assert.NoError(t, err)
	assert.Equal(t, fromIndex, commitmentMsgFromDB.Message.FromIndex)
	assert.Equal(t, toIndex, commitmentMsgFromDB.Message.ToIndex)
	assert.Equal(t, uint64(stateSyncBundleSize), commitmentMsgFromDB.Message.BundleSize)
	assert.Equal(t, trie.Hash(), commitmentMsgFromDB.Message.MerkleRootHash)
	assert.NotNil(t, commitmentMsgFromDB.AggSignature)

	bundles, err := state.getBundles(fromIndex, maxBundlesPerSprint)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(bundles))

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func createTestTransportMessage(t *testing.T, hash []byte, epochNumber uint64, key *wallet.Key) *TransportMessage {
	t.Helper()

	signature, _ := key.Sign(hash)

	return &TransportMessage{
		Hash:        hash,
		Signature:   signature,
		NodeID:      key.NodeID(),
		EpochNumber: epochNumber,
	}
}

func createTestMessageVote(t *testing.T, hash []byte, validator *testValidator) *MessageSignature {
	t.Helper()

	signature := validator.mustSign(hash)

	return &MessageSignature{
		From:      validator.Key().NodeID(),
		Signature: signature,
	}
}

func createTestKey(t *testing.T) *wallet.Key {
	t.Helper()

	return wallet.NewKey(wallet.GenerateAccount())
}

func createTestBlocksForUptime(t *testing.T, numberOfBlocks uint64,
	validatorSet AccountSet) (*types.Header, *testHeadersMap) {
	t.Helper()

	headerMap := &testHeadersMap{}
	bitmaps := createTestBitmapsForUptime(t, validatorSet, numberOfBlocks)

	genesisBlock := &types.Header{
		Number:    0,
		ExtraData: []byte{},
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
			ExtraData:  createTestExtraForAccounts(t, validatorSet, bitmaps[i]),
			GasLimit:   types.StateTransactionGasLimit,
		}

		headerMap.addHeader(header)

		parentHash = hash
		blockHeader = header
	}

	return blockHeader, headerMap
}

func createTestBitmapsForUptime(t *testing.T, validators AccountSet, numberOfBlocks uint64) map[uint64]bitmap.Bitmap {
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

func createTestExtraForAccounts(t *testing.T, validators AccountSet, b bitmap.Bitmap) []byte {
	t.Helper()

	dummySignature := [64]byte{}
	extraData := Extra{
		Validators: &ValidatorSetDelta{
			Added:   validators,
			Removed: bitmap.Bitmap{},
		},
		Parent:    &Signature{Bitmap: b, AggregatedSignature: dummySignature[:]},
		Committed: &Signature{Bitmap: b, AggregatedSignature: dummySignature[:]},
	}

	marshaled := extraData.MarshalRLPTo(nil)
	result := make([]byte, ExtraVanity+len(marshaled))

	copy(result[ExtraVanity:], marshaled)

	return result
}

func insertTestStateSyncEvents(t *testing.T, numberOfEvents int, startIndex uint64, state *State) []*StateSyncEvent {
	t.Helper()

	stateSyncs := generateStateSyncEvents(t, numberOfEvents, startIndex)
	for _, stateSync := range stateSyncs {
		require.NoError(t, state.insertStateSyncEvent(stateSync))
	}

	return stateSyncs
}
