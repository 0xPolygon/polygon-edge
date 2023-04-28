package polybft

import (
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

func TestConsensusRuntime_OnBlockInserted_EndOfEpoch(t *testing.T) {
	t.Parallel()

	const (
		epochSize       = uint64(10)
		validatorsCount = 7
	)

	currentEpochNumber := getEpochNumber(t, epochSize, epochSize)
	validatorSet := newTestValidators(t, validatorsCount).getPublicIdentities()
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
		lastBuiltBlock:    &types.Header{Number: header.Number - 1},
		stateSyncManager:  &dummyStateSyncManager{},
		checkpointManager: &dummyCheckpointManager{},
		stakeManager:      &dummyStakeManager{},
	}
	runtime.OnBlockInserted(&types.FullBlock{Block: builtBlock})

	require.True(t, runtime.state.EpochStore.isEpochInserted(currentEpochNumber+1))
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
	runtime.OnBlockInserted(&types.FullBlock{Block: builtBlock})

	require.Equal(t, header.Number, runtime.lastBuiltBlock.Number)
}

func TestConsensusRuntime_FSM_NotInValidatorSet(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D"})

	snapshot := NewProposerSnapshot(1, nil)
	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize: 1,
		},
		Key: createTestKey(t),
	}
	runtime := &consensusRuntime{
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		config:             config,
		epoch: &epochMetadata{
			Number:     1,
			Validators: validators.getPublicIdentities(),
		},
		lastBuiltBlock: &types.Header{},
	}
	runtime.setIsActiveValidator(true)

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
		ExtraData: extra.MarshalRLPTo(nil),
	}

	validators := newTestValidators(t, 3)
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
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		logger:             hclog.NewNullLogger(),
		config:             config,
		epoch: &epochMetadata{
			Number:            1,
			Validators:        validators.getPublicIdentities(),
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock:    lastBlock,
		state:             newTestState(t),
		stateSyncManager:  &dummyStateSyncManager{},
		checkpointManager: &dummyCheckpointManager{},
	}
	runtime.setIsActiveValidator(true)

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

func TestConsensusRuntime_FSM_EndOfEpoch_BuildCommitEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch             = 0
		epochSize         = uint64(10)
		sprintSize        = uint64(3)
		firstBlockInEpoch = uint64(1)
		fromIndex         = uint64(0)
		toIndex           = uint64(9)
	)

	validatorAccounts := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	validators := validatorAccounts.getPublicIdentities()

	lastBuiltBlock, headerMap := createTestBlocks(t, 9, epochSize, validators)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("NewBlockBuilder", mock.Anything).Return(&BlockBuilder{}, nil).Once()
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headerMap.getHeader)

	state := newTestState(t)
	require.NoError(t, state.EpochStore.insertEpoch(epoch))

	metadata := &epochMetadata{
		Validators:        validators,
		Number:            epoch,
		FirstBlockInEpoch: firstBlockInEpoch,
	}

	config := &runtimeConfig{
		PolyBFTConfig: &PolyBFTConfig{
			EpochSize:  epochSize,
			SprintSize: sprintSize,
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
		stateSyncManager:   &dummyStateSyncManager{},
		checkpointManager:  &dummyCheckpointManager{},
		stakeManager:       &dummyStakeManager{},
	}

	err := runtime.FSM()
	fsm := runtime.fsm

	assert.NoError(t, err)
	assert.True(t, fsm.isEndOfEpoch)
	assert.NotNil(t, fsm.commitEpochInput)
	assert.NotEmpty(t, fsm.commitEpochInput)

	blockchainMock.AssertExpectations(t)
}

func Test_NewConsensusRuntime(t *testing.T) {
	t.Parallel()

	_, err := os.Create("/tmp/consensusState.db")
	require.NoError(t, err)

	polyBftConfig := &PolyBFTConfig{
		Bridge: &BridgeConfig{
			StateSenderAddr:       types.Address{0x13},
			CheckpointManagerAddr: types.Address{0x10},
			JSONRPCEndpoint:       "testEndpoint",
		},
		EpochSize:  10,
		SprintSize: 10,
		BlockTime:  common.Duration{Duration: 2 * time.Second},
	}

	validators := newTestValidators(t, 3).getPublicIdentities()

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
		bridgeTopic:    &mockTopic{},
	}
	runtime, err := newConsensusRuntime(hclog.NewNullLogger(), config)
	require.NoError(t, err)

	assert.False(t, runtime.isActiveValidator())
	assert.Equal(t, runtime.config.DataDir, tmpDir)
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.SprintSize)
	assert.Equal(t, uint64(10), runtime.config.PolyBFTConfig.EpochSize)
	assert.Equal(t, "0x0000000000000000000000000000000000000101", contracts.ValidatorSetContract.String())
	assert.Equal(t, "0x1300000000000000000000000000000000000000", runtime.config.PolyBFTConfig.Bridge.StateSenderAddr.String())
	assert.Equal(t, "0x1000000000000000000000000000000000000000", runtime.config.PolyBFTConfig.Bridge.CheckpointManagerAddr.String())
	assert.True(t, runtime.IsBridgeEnabled())
	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_restartEpoch_SameEpochNumberAsTheLastOne(t *testing.T) {
	t.Parallel()

	const originalBlockNumber = uint64(5)

	newCurrentHeader := &types.Header{Number: originalBlockNumber + 1}
	validatorSet := newTestValidators(t, 3).getPublicIdentities()

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
		proposerCalculator: NewProposerCalculatorFromSnapshot(snapshot, config, hclog.NewNullLogger()),
		config:             config,
		epoch: &epochMetadata{
			Number:            1,
			Validators:        validatorSet,
			FirstBlockInEpoch: 1,
		},
		lastBuiltBlock: &types.Header{
			Number: originalBlockNumber,
		},
	}
	runtime.setIsActiveValidator(true)

	epoch, err := runtime.restartEpoch(newCurrentHeader)

	require.NoError(t, err)

	for _, a := range validatorSet.GetAddresses() {
		assert.True(t, epoch.Validators.ContainsAddress(a))
	}

	systemStateMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestConsensusRuntime_calculateCommitEpochInput_SecondEpoch(t *testing.T) {
	t.Parallel()

	const (
		epoch           = 2
		epochSize       = 10
		epochStartBlock = 11
		epochEndBlock   = 20
		sprintSize      = 5
	)

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	polybftConfig := &PolyBFTConfig{
		EpochSize:  epochSize,
		SprintSize: sprintSize,
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

	commitEpochInput, distributeRewardsInput, err := consensusRuntime.calculateCommitEpochInput(lastBuiltBlock,
		consensusRuntime.epoch)
	assert.NoError(t, err)
	assert.NotEmpty(t, commitEpochInput)
	assert.Equal(t, uint64(epoch), commitEpochInput.ID.Uint64())
	assert.Equal(t, uint64(epochStartBlock), commitEpochInput.Epoch.StartBlock.Uint64())
	assert.Equal(t, uint64(epochEndBlock), commitEpochInput.Epoch.EndBlock.Uint64())
	assert.Equal(t, uint64(epoch), distributeRewardsInput.EpochID.Uint64())

	blockchainMock.AssertExpectations(t)
	polybftBackendMock.AssertExpectations(t)
}

func TestConsensusRuntime_IsValidValidator_BasicCases(t *testing.T) {
	t.Parallel()

	setupFn := func(t *testing.T) (*consensusRuntime, *testValidators) {
		t.Helper()

		validatorAccounts := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
		epoch := &epochMetadata{
			Validators: validatorAccounts.getPublicIdentities("A", "B", "C", "D"),
		}
		runtime := &consensusRuntime{
			epoch:  epoch,
			logger: hclog.NewNullLogger(),
			fsm:    &fsm{validators: NewValidatorSet(epoch.Validators, hclog.NewNullLogger())},
		}

		return runtime, validatorAccounts
	}

	cases := []struct {
		name          string
		signerAlias   string
		senderAlias   string
		isValidSender bool
	}{
		{
			name:          "Valid sender",
			signerAlias:   "A",
			senderAlias:   "A",
			isValidSender: true,
		},
		{
			name:          "Sender not amongst current validators",
			signerAlias:   "F",
			senderAlias:   "F",
			isValidSender: false,
		},
		{
			name:          "Sender and signer accounts mismatch",
			signerAlias:   "A",
			senderAlias:   "B",
			isValidSender: false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			runtime, validatorAccounts := setupFn(t)
			signer := validatorAccounts.getValidator(c.signerAlias)
			sender := validatorAccounts.getValidator(c.senderAlias)
			msg, err := signer.Key().SignIBFTMessage(&proto.Message{From: sender.Address().Bytes()})

			require.NoError(t, err)
			require.Equal(t, c.isValidSender, runtime.IsValidValidator(msg))
		})
	}
}

func TestConsensusRuntime_IsValidValidator_TamperSignature(t *testing.T) {
	t.Parallel()

	validatorAccounts := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	epoch := &epochMetadata{
		Validators: validatorAccounts.getPublicIdentities("A", "B", "C", "D"),
	}
	runtime := &consensusRuntime{
		epoch:  epoch,
		logger: hclog.NewNullLogger(),
		fsm:    &fsm{validators: NewValidatorSet(epoch.Validators, hclog.NewNullLogger())},
	}

	// provide invalid signature
	sender := validatorAccounts.getValidator("A")
	msg := &proto.Message{
		From:      sender.Address().Bytes(),
		Signature: []byte{1, 2, 3, 4, 5},
	}
	require.False(t, runtime.IsValidValidator(msg))
}

func TestConsensusRuntime_TamperMessageContent(t *testing.T) {
	t.Parallel()

	validatorAccounts := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	epoch := &epochMetadata{
		Validators: validatorAccounts.getPublicIdentities("A", "B", "C", "D"),
	}
	runtime := &consensusRuntime{
		epoch:  epoch,
		logger: hclog.NewNullLogger(),
		fsm:    &fsm{validators: NewValidatorSet(epoch.Validators, hclog.NewNullLogger())},
	}
	sender := validatorAccounts.getValidator("A")
	proposalHash := []byte{2, 4, 6, 8, 10}
	proposalSignature, err := sender.Key().SignWithDomain(proposalHash, bls.DomainCheckpointManager)
	require.NoError(t, err)

	msg := &proto.Message{
		View: &proto.View{},
		From: sender.Address().Bytes(),
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: proposalSignature,
			},
		},
	}
	// sign the message itself
	msg, err = sender.Key().SignIBFTMessage(msg)
	assert.NoError(t, err)
	// signature verification works
	assert.True(t, runtime.IsValidValidator(msg))

	// modify message without signing it again
	msg.Payload = &proto.Message_CommitData{
		CommitData: &proto.CommitMessage{
			ProposalHash:  []byte{1, 3, 5, 7, 9}, // modification
			CommittedSeal: proposalSignature,
		},
	}
	// signature isn't valid, because message was tampered
	assert.False(t, runtime.IsValidValidator(msg))
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
			ExtraData: extra.MarshalRLPTo(nil),
		},
	}
	block.Header.ComputeHash()

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	require.True(t, runtime.IsValidProposalHash(&proto.Proposal{RawProposal: block.MarshalRLP()}, proposalHash.Bytes()))
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
			ExtraData: extra.MarshalRLPTo(nil),
		},
	}

	proposalHash, err := extra.Checkpoint.Hash(0, block.Number(), block.Hash())
	require.NoError(t, err)

	extra.Checkpoint.BlockRound = 2 // change it so it is not the same as in proposal hash
	block.Header.ExtraData = extra.MarshalRLPTo(nil)
	block.Header.ComputeHash()

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{blockchain: new(blockchainMock)},
	}

	require.False(t, runtime.IsValidProposalHash(&proto.Proposal{RawProposal: block.MarshalRLP()}, proposalHash.Bytes()))
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

	require.False(t, runtime.IsValidProposalHash(&proto.Proposal{RawProposal: block.MarshalRLP()}, proposalHash.Bytes()))
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

	require.Nil(t, runtime.BuildProposal(&proto.View{Round: 5}))
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

	validatorAccounts := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})

	extra := &Extra{
		Checkpoint: &CheckpointData{},
	}

	lastBuildBlock := &types.Header{
		Number:    1,
		ExtraData: extra.MarshalRLPTo(nil),
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
		stateSyncManager:   &dummyStateSyncManager{},
		checkpointManager:  &dummyCheckpointManager{},
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
	view, rawProposal, certificate := &proto.View{}, []byte{1}, &proto.PreparedCertificate{}

	runtime := &consensusRuntime{
		config: &runtimeConfig{
			Key: key,
		},
	}

	proposal := &proto.Proposal{
		RawProposal: rawProposal,
		Round:       view.Round,
	}

	expected := proto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{RoundChangeData: &proto.RoundChangeMessage{
			LatestPreparedCertificate: certificate,
			LastPreparedProposal:      proposal,
		}},
	}

	signedMsg, err := key.SignIBFTMessage(&expected)
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

	committedSeal, err := key.SignWithDomain(proposalHash, bls.DomainCheckpointManager)
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

	signedMsg, err := key.SignIBFTMessage(&expected)
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

	assert.False(t, runtime.IsValidProposalHash(&proto.Proposal{}, []byte("hash")))
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

	signedMsg, err := key.SignIBFTMessage(&expected)
	require.NoError(t, err)

	assert.Equal(t, signedMsg, runtime.BuildPrepareMessage(proposalHash, view))
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
		ExtraData: extra.MarshalRLPTo(nil),
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

	rand.Seed(time.Now().UTC().Unix())

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

	return extraData.MarshalRLPTo(nil)
}

func encodeExitEvents(t *testing.T, exitEvents []*ExitEvent) [][]byte {
	t.Helper()

	encodedEvents := make([][]byte, len(exitEvents))

	var exitEventAPI contractsapi.L2StateSyncedEvent
	for i, e := range exitEvents {
		encodedEvent, err := exitEventAPI.Encode(e)
		require.NoError(t, err)

		encodedEvents[i] = encodedEvent
	}

	return encodedEvents
}
