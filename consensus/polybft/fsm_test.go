package polybft

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFSM_ValidateHeader(t *testing.T) {
	t.Parallel()

	parent := &types.Header{Number: 0, Hash: types.BytesToHash([]byte{1, 2, 3})}
	header := &types.Header{Number: 0}

	// parent hash
	require.ErrorContains(t, validateHeaderFields(parent, header), "incorrect header parent hash")
	header.ParentHash = parent.Hash

	// sequence number
	require.ErrorContains(t, validateHeaderFields(parent, header), "invalid number")
	header.Number = 1

	// failed timestamp
	require.ErrorContains(t, validateHeaderFields(parent, header), "timestamp older than parent")
	header.Timestamp = 10

	// mix digest
	require.ErrorContains(t, validateHeaderFields(parent, header), "mix digest is not correct")
	header.MixHash = PolyBFTMixDigest

	// difficulty
	header.Difficulty = 0
	require.ErrorContains(t, validateHeaderFields(parent, header), "difficulty should be greater than zero")

	header.Difficulty = 1
	header.Hash = types.BytesToHash([]byte{11, 22, 33})
	require.ErrorContains(t, validateHeaderFields(parent, header), "invalid header hash")

	header.ComputeHash()
	require.NoError(t, validateHeaderFields(parent, header))
}

func TestFSM_verifyCommitEpochTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{
		config:           &PolyBFTConfig{ValidatorSetAddr: contracts.ValidatorSetContract},
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10),
	}

	// include commit epoch transaction to the epoch ending block
	commitEpochTx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)
	assert.NotNil(t, commitEpochTx)

	assert.NoError(t, fsm.verifyCommitEpochTx(commitEpochTx))

	// submit tampered commit epoch transaction to the epoch ending block
	alteredCommitEpochTx := &types.Transaction{
		To:    &fsm.config.ValidatorSetAddr,
		Input: []byte{},
		Gas:   0,
		Type:  types.StateTx,
	}
	assert.ErrorContains(t, fsm.verifyCommitEpochTx(alteredCommitEpochTx), "invalid commit epoch transaction")

	// submit validators commit epoch transaction to the non-epoch ending block
	fsm.isEndOfEpoch = false
	commitEpochTx, err = fsm.createCommitEpochTx()
	assert.NoError(t, err)
	assert.NotNil(t, commitEpochTx)
	assert.ErrorContains(t, fsm.verifyCommitEpochTx(commitEpochTx), errCommitEpochTxNotExpected.Error())
}

func TestFSM_BuildProposal_WithoutCommitEpochTxGood(t *testing.T) {
	t.Parallel()

	const (
		accountCount             = 5
		committedCount           = 4
		parentCount              = 3
		confirmedStateSyncsCount = 5
		parentBlockNumber        = 1023
		currentRound             = 1
	)

	eventRoot := types.ZeroHash

	validators := newTestValidators(t, accountCount)
	validatorSet := validators.getPublicIdentities()
	extra := createTestExtra(validatorSet, AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	blockchainMock := &blockchainMock{}
	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{
			Key:        wallet.NewKey(validators.getPrivateIdentities()[0], bls.DomainCheckpointManager),
			blockchain: blockchainMock,
		},
	}

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: blockchainMock,
		validators: validators.toValidatorSet(), exitEventRootHash: eventRoot, logger: hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(currentRound)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	currentValidatorsHash, err := validatorSet.Hash()
	require.NoError(t, err)

	rlpBlock := stateBlock.Block.MarshalRLP()
	assert.Equal(t, rlpBlock, proposal)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	checkpoint := &CheckpointData{
		BlockRound:            currentRound,
		EpochNumber:           fsm.epochNumber,
		EventRoot:             eventRoot,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    currentValidatorsHash,
	}

	checkpointHash, err := checkpoint.Hash(fsm.backend.GetChainID(), block.Number(), block.Hash())
	require.NoError(t, err)

	msg := runtime.BuildPrePrepareMessage(proposal, nil, &proto.View{})
	require.Equal(t, checkpointHash.Bytes(), msg.GetPreprepareData().ProposalHash)

	mBlockBuilder.AssertExpectations(t)
}

func TestFSM_BuildProposal_WithCommitEpochTxGood(t *testing.T) {
	t.Parallel()

	const (
		accountCount             = 5
		committedCount           = 4
		parentCount              = 3
		confirmedStateSyncsCount = 5
		currentRound             = 0
		parentBlockNumber        = 1023
	)

	eventRoot := types.ZeroHash

	validators := newTestValidators(t, accountCount)
	extra := createTestExtra(validators.getPublicIdentities(), AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	transition := &state.Transition{}
	mBlockBuilder := newBlockBuilderMock(stateBlock)
	mBlockBuilder.On("WriteTx", mock.Anything).Return(error(nil)).Once()
	mBlockBuilder.On("GetState").Return(transition).Once()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetValidatorSet").Return(nil).Once()

	blockChainMock := new(blockchainMock)
	blockChainMock.On("GetStateProvider", mock.Anything).
		Return(NewStateProvider(transition)).Once()
	blockChainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{
			Key:        wallet.NewKey(validators.getPrivateIdentities()[0], bls.DomainCheckpointManager),
			blockchain: blockChainMock,
		},
	}

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: blockChainMock,
		isEndOfEpoch:      true,
		validators:        validators.toValidatorSet(),
		commitEpochInput:  createTestCommitEpochInput(t, 0, nil, 10),
		exitEventRootHash: eventRoot,
		logger:            hclog.NewNullLogger(),
	}

	proposal, err := fsm.BuildProposal(currentRound)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	assert.Equal(t, stateBlock.Block.MarshalRLP(), proposal)

	currentValidatorsHash, err := validators.getPublicIdentities().Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := AccountSet{}.Hash()
	require.NoError(t, err)

	checkpoint := &CheckpointData{
		BlockRound:            currentRound,
		EpochNumber:           fsm.epochNumber,
		EventRoot:             eventRoot,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    nextValidatorsHash,
	}

	checkpointHash, err := checkpoint.Hash(fsm.backend.GetChainID(), block.Number(), block.Hash())
	require.NoError(t, err)

	msg := runtime.BuildPrePrepareMessage(proposal, nil, &proto.View{})
	require.Equal(t, checkpointHash.Bytes(), msg.GetPreprepareData().ProposalHash)

	mBlockBuilder.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockChainMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_FailedToApplyStateTx(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		committedCount    = 4
		parentCount       = 3
		parentBlockNumber = 1023
	)

	validators := newTestValidators(t, accountCount)
	extra := createTestExtra(validators.getPublicIdentities(), AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}

	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("WriteTx", mock.Anything).Return(errors.New("error")).Once()
	mBlockBuilder.On("Reset").Return(error(nil)).Once()

	validatorSet := NewValidatorSet(validators.getPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		isEndOfEpoch:      true,
		validators:        validatorSet,
		commitEpochInput:  createTestCommitEpochInput(t, 0, nil, 10),
		exitEventRootHash: types.ZeroHash,
	}

	_, err := fsm.BuildProposal(0)
	assert.ErrorContains(t, err, "failed to apply commit epoch transaction")
	mBlockBuilder.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_ValidatorsDeltaExists(t *testing.T) {
	t.Parallel()

	const (
		validatorsCount          = 6
		remainingValidatorsCount = 3
		signaturesCount          = 4
		parentBlockNumber        = 49
	)

	validators := newTestValidators(t, validatorsCount).getPublicIdentities()
	extra := createTestExtra(validators, AccountSet{}, validatorsCount-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	transition := &state.Transition{}
	blockBuilderMock := newBlockBuilderMock(stateBlock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Once()
	blockBuilderMock.On("GetState").Return(transition).Once()

	newValidators := validators[:remainingValidatorsCount].Copy()
	addedValidators := newTestValidators(t, 2).getPublicIdentities()
	newValidators = append(newValidators, addedValidators...)
	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetValidatorSet").Return(newValidators).Once()

	blockChainMock := new(blockchainMock)
	blockChainMock.On("GetStateProvider", mock.Anything).
		Return(NewStateProvider(transition)).Once()
	blockChainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	validatorSet := NewValidatorSet(validators, hclog.NewNullLogger())

	fsm := &fsm{
		parent:            parent,
		blockBuilder:      blockBuilderMock,
		config:            &PolyBFTConfig{},
		backend:           blockChainMock,
		isEndOfEpoch:      true,
		validators:        validatorSet,
		commitEpochInput:  createTestCommitEpochInput(t, 0, validators, 10),
		exitEventRootHash: types.ZeroHash,
		logger:            hclog.NewNullLogger(),
	}

	proposal, err := fsm.BuildProposal(0)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.Len(t, blockExtra.Validators.Added, 2)
	assert.False(t, blockExtra.Validators.IsEmpty())

	removedValidators := [3]uint64{3, 4, 5}

	for _, addedValidator := range addedValidators {
		assert.True(t, blockExtra.Validators.Added.ContainsAddress(addedValidator.Address))
	}

	for _, removedValidator := range removedValidators {
		assert.True(
			t,
			blockExtra.Validators.Removed.IsSet(removedValidator),
			fmt.Sprintf("Expected validator at index %d to be marked as removed, but it wasn't", removedValidator),
		)
	}

	blockBuilderMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockChainMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_NonEpochEndingBlock_ValidatorsDeltaEmpty(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 9
	)

	testValidators := newTestValidators(t, accountCount)
	extra := createTestExtra(testValidators.getPublicIdentities(), AccountSet{}, accountCount-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	blockBuilderMock := &blockBuilderMock{}
	blockBuilderMock.On("Build", mock.Anything).Return(stateBlock).Once()
	blockBuilderMock.On("Fill").Once()
	blockBuilderMock.On("Reset").Return(error(nil)).Once()

	systemStateMock := new(systemStateMock)

	fsm := &fsm{parent: parent, blockBuilder: blockBuilderMock,
		config: &PolyBFTConfig{}, backend: &blockchainMock{},
		isEndOfEpoch: false, validators: testValidators.toValidatorSet(),
		exitEventRootHash: types.ZeroHash, logger: hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(0)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.True(t, blockExtra.Validators.IsEmpty())

	blockBuilderMock.AssertExpectations(t)
	systemStateMock.AssertNotCalled(t, "GetValidatorSet")
}

func TestFSM_BuildProposal_EpochEndingBlock_FailToCreateValidatorsDelta(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 49
	)

	testValidators := newTestValidators(t, accountCount)
	allAccounts := testValidators.getPublicIdentities()
	extra := createTestExtra(allAccounts, AccountSet{}, accountCount-1, signaturesCount, signaturesCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}

	transition := &state.Transition{}
	blockBuilderMock := new(blockBuilderMock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Once()
	blockBuilderMock.On("GetState").Return(transition).Once()
	blockBuilderMock.On("Reset").Return(error(nil)).Once()
	blockBuilderMock.On("Fill").Once()

	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetValidatorSet").Return(nil, errors.New("failed to get validators set")).Once()

	blockChainMock := new(blockchainMock)
	blockChainMock.On("GetStateProvider", mock.Anything).
		Return(NewStateProvider(transition)).Once()
	blockChainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	fsm := &fsm{parent: parent,
		blockBuilder:      blockBuilderMock,
		config:            &PolyBFTConfig{},
		backend:           blockChainMock,
		isEndOfEpoch:      true,
		validators:        testValidators.toValidatorSet(),
		commitEpochInput:  createTestCommitEpochInput(t, 0, allAccounts, 10),
		exitEventRootHash: types.ZeroHash,
	}

	proposal, err := fsm.BuildProposal(0)
	assert.ErrorContains(t, err, "failed to retrieve validator set for current block: failed to get validators set")
	assert.Nil(t, proposal)

	blockBuilderMock.AssertNotCalled(t, "Build")
	blockBuilderMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockChainMock.AssertExpectations(t)
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10)}
	tx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)
	err = fsm.VerifyStateTransactions([]*types.Transaction{tx})
	assert.ErrorContains(t, err, err.Error())
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10)}
	err := fsm.VerifyStateTransactions([]*types.Transaction{})
	assert.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_EndOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, isEndOfEpoch: true, commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10)}
	assert.EqualError(t, fsm.VerifyStateTransactions([]*types.Transaction{}),
		"commit epoch transaction is not found in the epoch ending block")
}

func TestFSM_VerifyStateTransactions_EndOfEpochWrongCommitEpochTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, isEndOfEpoch: true, commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10)}
	commitEpochInput, err := createTestCommitEpochInput(t, 0, nil, 5).EncodeAbi()
	require.NoError(t, err)

	commitEpochTx := createStateTransactionWithData(contracts.ValidatorSetContract, commitEpochInput)
	assert.ErrorContains(t, fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx}), "invalid commit epoch transaction")
}

func TestFSM_VerifyStateTransactions_CommitmentTransactionAndSprintIsFalse(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}}

	encodedCommitment, err := createTestCommitmentMessage(t, 1).EncodeAbi()
	require.NoError(t, err)

	tx := createStateTransactionWithData(contracts.StateReceiverContract, encodedCommitment)
	assert.ErrorContains(t, fsm.VerifyStateTransactions([]*types.Transaction{tx}),
		"found commitment tx in block which should not contain it")
}

func TestFSM_VerifyStateTransactions_EndOfEpochMoreThanOneCommitEpochTx(t *testing.T) {
	t.Parallel()

	txs := make([]*types.Transaction, 2)
	fsm := &fsm{config: &PolyBFTConfig{}, isEndOfEpoch: true, commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10)}

	commitEpochTxOne, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	txs[0] = commitEpochTxOne

	commitEpochTxTwo := createTestCommitEpochInput(t, 0, nil, 100)
	input, err := commitEpochTxTwo.EncodeAbi()
	require.NoError(t, err)

	txs[1] = createStateTransactionWithData(types.ZeroAddress, input)

	assert.ErrorIs(t, fsm.VerifyStateTransactions(txs), errCommitEpochTxSingleExpected)
}

func TestFSM_VerifyStateTransactions_StateTransactionPass(t *testing.T) {
	t.Parallel()

	validators := newTestValidators(t, 5)

	validatorSet := NewValidatorSet(validators.getPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		config:           &PolyBFTConfig{},
		isEndOfEpoch:     true,
		isEndOfSprint:    true,
		validators:       validatorSet,
		commitEpochInput: createTestCommitEpochInput(t, 0, nil, 10),
		logger:           hclog.NewNullLogger(),
	}
	txs := fsm.stateTransactions()

	// add commit epoch tx to the end of transactions list
	tx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)

	txs = append([]*types.Transaction{tx}, txs...)

	err = fsm.VerifyStateTransactions(txs)
	assert.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_StateTransactionQuorumNotReached(t *testing.T) {
	t.Parallel()

	validators := newTestValidators(t, 5)
	commitment := createTestCommitment(t, validators.getPrivateIdentities())
	commitment.AggSignature = Signature{
		AggregatedSignature: []byte{1, 2},
		Bitmap:              []byte{},
	}

	validatorSet := NewValidatorSet(validators.getPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		config:                       &PolyBFTConfig{},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		validators:                   validatorSet,
		proposerCommitmentToRegister: commitment,
		commitEpochInput:             createTestCommitEpochInput(t, 0, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	txs := fsm.stateTransactions()

	// add commit epoch tx to the end of transactions list
	tx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)

	txs = append([]*types.Transaction{tx}, txs...)

	err = fsm.VerifyStateTransactions(txs)
	assert.ErrorContains(t, err, "quorum size not reached")
}

func TestFSM_VerifyStateTransactions_StateTransactionInvalidSignature(t *testing.T) {
	t.Parallel()

	validators := newTestValidators(t, 5)
	commitment := createTestCommitment(t, validators.getPrivateIdentities())
	nonValidators := newTestValidators(t, 3)
	aggregatedSigs := bls.Signatures{}

	nonValidators.iterAcct(nil, func(t *testValidator) {
		aggregatedSigs = append(aggregatedSigs, t.mustSign([]byte("dummyHash")))
	})

	sig, err := aggregatedSigs.Aggregate().Marshal()
	require.NoError(t, err)

	commitment.AggSignature.AggregatedSignature = sig

	validatorSet := NewValidatorSet(validators.getPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		config:                       &PolyBFTConfig{},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		validators:                   validatorSet,
		proposerCommitmentToRegister: commitment,
		commitEpochInput:             createTestCommitEpochInput(t, 0, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	txs := fsm.stateTransactions()

	// add commit epoch tx to the end of transactions list
	tx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)

	txs = append([]*types.Transaction{tx}, txs...)

	err = fsm.VerifyStateTransactions(txs)
	assert.ErrorContains(t, err, "invalid signature")
}

func TestFSM_ValidateCommit_WrongValidator(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := newTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), logger: hclog.NewNullLogger(), exitEventRootHash: types.ZeroHash}

	_, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	err = fsm.ValidateCommit([]byte("0x7467674"), types.ZeroAddress.Bytes(), []byte{})
	require.ErrorContains(t, err, "unable to resolve validator")
}

func TestFSM_ValidateCommit_InvalidHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := newTestValidators(t, accountsCount)

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), exitEventRootHash: types.ZeroHash, logger: hclog.NewNullLogger()}

	_, err := fsm.BuildProposal(0)
	assert.NoError(t, err)

	nonValidatorAcc := newTestValidator(t, "non_validator", 1)
	wrongSignature, err := nonValidatorAcc.mustSign([]byte("Foo")).Marshal()
	require.NoError(t, err)

	err = fsm.ValidateCommit(validators.getValidator("0").Address().Bytes(), wrongSignature, []byte{})
	require.ErrorContains(t, err, "incorrect commit signature from")
}

func TestFSM_ValidateCommit_Good(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.getPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber, ExtraData: createTestExtra(validatorsMetadata, AccountSet{}, 5, 3, 3)}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	validatorSet := NewValidatorSet(validatorsMetadata, hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators:        validatorSet,
		exitEventRootHash: types.ZeroHash,
		logger:            hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	validator := validators.getValidator("A")
	seal, err := validator.mustSign(block.Hash().Bytes()).Marshal()
	require.NoError(t, err)
	err = fsm.ValidateCommit(validator.Key().Address().Bytes(), seal, block.Hash().Bytes())
	require.NoError(t, err)
}

func TestFSM_Validate_IncorrectHeaderParentHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := newTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	fsm := &fsm{parent: parent, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), logger: hclog.NewNullLogger()}

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, parent.ExtraData)

	hash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = hash
	proposal := stateBlock.Block.MarshalRLP()

	err = fsm.Validate(proposal)
	require.ErrorContains(t, err, "incorrect header parent hash")
}

func TestFSM_Validate_InvalidNumber(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := newTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	// try some invalid block numbers, parentBlockNumber + 1 should be correct
	for _, blockNum := range []uint64{parentBlockNumber - 1, parentBlockNumber, parentBlockNumber + 2} {
		stateBlock := createDummyStateBlock(blockNum, parent.Hash, parent.ExtraData)
		mBlockBuilder := newBlockBuilderMock(stateBlock)
		fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
			validators: validators.toValidatorSet(), logger: hclog.NewNullLogger()}

		proposalHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
		require.NoError(t, err)

		stateBlock.Block.Header.Hash = proposalHash
		proposal := stateBlock.Block.MarshalRLP()

		err = fsm.Validate(proposal)
		require.ErrorContains(t, err, "invalid number")
	}
}

func TestFSM_Validate_TimestampOlder(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := newTestValidators(t, 5)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 4, 3, 3),
		Timestamp: uint64(time.Now().Unix()),
	}
	parent.ComputeHash()

	// try some invalid times
	for _, blockTime := range []uint64{parent.Timestamp - 1, parent.Timestamp} {
		header := &types.Header{
			Number:     parentBlockNumber + 1,
			ParentHash: parent.Hash,
			Timestamp:  blockTime,
			ExtraData:  parent.ExtraData,
		}
		stateBlock := &types.FullBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{Header: header})}
		fsm := &fsm{parent: parent, config: &PolyBFTConfig{}, backend: &blockchainMock{},
			validators: validators.toValidatorSet(), logger: hclog.NewNullLogger()}

		checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), header.Number, header.Hash)
		require.NoError(t, err)

		stateBlock.Block.Header.Hash = checkpointHash
		proposal := stateBlock.Block.MarshalRLP()

		err = fsm.Validate(proposal)
		assert.ErrorContains(t, err, "timestamp older than parent")
	}
}

func TestFSM_Validate_IncorrectMixHash(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := newTestValidators(t, 5)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 4, 3, 3),
		Timestamp: uint64(100),
	}
	parent.ComputeHash()

	header := &types.Header{
		Number:     parentBlockNumber + 1,
		ParentHash: parent.Hash,
		Timestamp:  parent.Timestamp + 1,
		MixHash:    types.Hash{},
		ExtraData:  parent.ExtraData,
	}

	buildBlock := &types.FullBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{Header: header})}

	fsm := &fsm{
		parent:     parent,
		config:     &PolyBFTConfig{},
		backend:    &blockchainMock{},
		validators: validators.toValidatorSet(),
		logger:     hclog.NewNullLogger(),
	}
	rlpBlock := buildBlock.Block.MarshalRLP()

	_, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), header.Number, header.Hash)
	require.NoError(t, err)

	err = fsm.Validate(rlpBlock)
	assert.ErrorContains(t, err, "mix digest is not correct")
}

func TestFSM_Insert_Good(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = uint64(10)
		signaturesCount   = 3
	)

	setupFn := func() (*fsm, []*messages.CommittedSeal, *types.FullBlock, *blockchainMock) {
		validators := newTestValidators(t, accountCount)
		allAccounts := validators.getPrivateIdentities()
		validatorsMetadata := validators.getPublicIdentities()

		extraParent := createTestExtra(validatorsMetadata, AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
		parent := &types.Header{Number: parentBlockNumber, ExtraData: extraParent}
		extraBlock := createTestExtra(validatorsMetadata, AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
		block := consensus.BuildBlock(
			consensus.BuildBlockParams{
				Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
			})

		builtBlock := &types.FullBlock{Block: block}

		builderMock := newBlockBuilderMock(builtBlock)
		chainMock := &blockchainMock{}
		chainMock.On("CommitBlock", mock.Anything).Return(error(nil)).Once()
		chainMock.On("ProcessBlock", mock.Anything, mock.Anything, mock.Anything).
			Return(builtBlock, error(nil)).
			Maybe()

		f := &fsm{
			parent:       parent,
			blockBuilder: builderMock,
			config:       &PolyBFTConfig{},
			target:       builtBlock,
			backend:      chainMock,
			validators:   NewValidatorSet(validatorsMetadata[0:len(validatorsMetadata)-1], hclog.NewNullLogger()),
			logger:       hclog.NewNullLogger(),
		}

		seals := make([]*messages.CommittedSeal, signaturesCount)

		for i := 0; i < signaturesCount; i++ {
			sign, err := allAccounts[i].Bls.Sign(builtBlock.Block.Hash().Bytes(), bls.DomainValidatorSet)
			require.NoError(t, err)
			sigRaw, err := sign.Marshal()
			require.NoError(t, err)

			seals[i] = &messages.CommittedSeal{
				Signer:    validatorsMetadata[i].Address.Bytes(),
				Signature: sigRaw,
			}
		}

		return f, seals, builtBlock, chainMock
	}

	t.Run("Insert with target block defined", func(t *testing.T) {
		t.Parallel()

		fsm, seals, builtBlock, chainMock := setupFn()
		proposal := builtBlock.Block.MarshalRLP()
		fullBlock, err := fsm.Insert(proposal, seals)

		require.NoError(t, err)
		require.Equal(t, parentBlockNumber+1, fullBlock.Block.Number())
		chainMock.AssertExpectations(t)
	})

	t.Run("Insert with target block undefined", func(t *testing.T) {
		t.Parallel()

		fsm, seals, builtBlock, _ := setupFn()
		fsm.target = nil
		proposal := builtBlock.Block.MarshalRLP()
		_, err := fsm.Insert(proposal, seals)

		require.ErrorIs(t, err, errProposalDontMatch)
	})

	t.Run("Insert with target block hash not match", func(t *testing.T) {
		t.Parallel()

		fsm, seals, builtBlock, _ := setupFn()
		proposal := builtBlock.Block.MarshalRLP()
		fsm.target = builtBlock
		fsm.target.Block.Header.Hash = types.BytesToHash(generateRandomBytes(t))
		_, err := fsm.Insert(proposal, seals)

		require.ErrorIs(t, err, errProposalDontMatch)
	})
}

func TestFSM_Insert_InvalidNode(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.getPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber}
	parent.ComputeHash()

	extraBlock := createTestExtra(validatorsMetadata, AccountSet{}, len(validators.validators)-1, signaturesCount, signaturesCount)
	finalBlock := consensus.BuildBlock(
		consensus.BuildBlockParams{
			Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
		})

	buildBlock := &types.FullBlock{Block: finalBlock, Receipts: []*types.Receipt{}}
	mBlockBuilder := newBlockBuilderMock(buildBlock)

	validatorSet := NewValidatorSet(validatorsMetadata[0:len(validatorsMetadata)-1], hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validatorSet,
	}

	proposal := buildBlock.Block.MarshalRLP()
	validatorA := validators.getValidator("A")
	validatorB := validators.getValidator("B")
	proposalHash := buildBlock.Block.Hash().Bytes()
	sigA, err := validatorA.mustSign(proposalHash).Marshal()
	require.NoError(t, err)

	sigB, err := validatorB.mustSign(proposalHash).Marshal()
	require.NoError(t, err)

	// create test account outside of validator set
	nonValidatorAccount := newTestValidator(t, "non_validator", 1)
	nonValidatorSignature, err := nonValidatorAccount.mustSign(proposalHash).Marshal()
	require.NoError(t, err)

	commitedSeals := []*messages.CommittedSeal{
		{Signer: validatorA.Address().Bytes(), Signature: sigA},
		{Signer: validatorB.Address().Bytes(), Signature: sigB},
		{Signer: nonValidatorAccount.Address().Bytes(), Signature: nonValidatorSignature}, // this one should fail
	}

	fsm.target = buildBlock

	_, err = fsm.Insert(proposal, commitedSeals)
	assert.ErrorContains(t, err, "invalid node id")
}

func TestFSM_Height(t *testing.T) {
	t.Parallel()

	parentNumber := uint64(3)
	parent := &types.Header{Number: parentNumber}
	fsm := &fsm{parent: parent}
	assert.Equal(t, parentNumber+1, fsm.Height())
}

func TestFSM_DecodeCommitmentStateTxs(t *testing.T) {
	t.Parallel()

	const (
		commitmentsCount = 8
		from             = 15
		eventsSize       = 40
	)

	_, signedCommitment, _ := buildCommitmentAndStateSyncs(t, eventsSize, uint64(3), from)

	f := &fsm{
		config:                       &PolyBFTConfig{},
		proposerCommitmentToRegister: signedCommitment,
		commitEpochInput:             createTestCommitEpochInput(t, 0, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	for i, tx := range f.stateTransactions() {
		decodedData, err := decodeStateTransaction(tx.Input)
		require.NoError(t, err)

		decodedCommitmentMsg, ok := decodedData.(*CommitmentMessageSigned)
		require.True(t, ok)
		require.Equal(t, 0, i, "failed for tx number %d", i)
		require.Equal(t, signedCommitment, decodedCommitmentMsg, "failed for tx number %d", i)
	}
}

func TestFSM_DecodeCommitEpochStateTx(t *testing.T) {
	t.Parallel()

	commitEpoch := createTestCommitEpochInput(t, 0, nil, 10)
	input, err := commitEpoch.EncodeAbi()
	require.NoError(t, err)
	require.NotNil(t, input)

	tx := createStateTransactionWithData(contracts.ValidatorSetContract, input)
	decodedInputData, err := decodeStateTransaction(tx.Input)
	require.NoError(t, err)

	decodedCommitEpoch, ok := decodedInputData.(*contractsapi.CommitEpochFunction)
	require.True(t, ok)
	require.True(t, commitEpoch.ID.Cmp(decodedCommitEpoch.ID) == 0)
	require.NotNil(t, decodedCommitEpoch.Epoch)
	require.True(t, commitEpoch.Epoch.StartBlock.Cmp(decodedCommitEpoch.Epoch.StartBlock) == 0)
	require.True(t, commitEpoch.Epoch.EndBlock.Cmp(decodedCommitEpoch.Epoch.EndBlock) == 0)
	require.NotNil(t, decodedCommitEpoch.Uptime)
	require.True(t, commitEpoch.Uptime.TotalBlocks.Cmp(decodedCommitEpoch.Uptime.TotalBlocks) == 0)
}

func TestFSM_VerifyStateTransaction_ValidBothTypesOfStateTransactions(t *testing.T) {
	t.Parallel()

	var (
		commitments       [2]*PendingCommitment
		stateSyncs        [2][]*contractsapi.StateSyncedEvent
		signedCommitments [2]*CommitmentMessageSigned
	)

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	commitments[0], signedCommitments[0], stateSyncs[0] = buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)
	commitments[1], signedCommitments[1], stateSyncs[1] = buildCommitmentAndStateSyncs(t, 10, uint64(3), 12)

	executeForValidators := func(aliases ...string) error {
		for _, sc := range signedCommitments {
			// add register commitment state transaction
			hash, err := sc.Hash()
			require.NoError(t, err)
			signature := createSignature(t, validators.getPrivateIdentities(aliases...), hash)
			sc.AggSignature = *signature
		}

		f := &fsm{
			isEndOfSprint: true,
			config:        &PolyBFTConfig{},
			validators:    validators.toValidatorSet(),
		}

		var txns []*types.Transaction

		for i, sc := range signedCommitments {
			inputData, err := sc.EncodeAbi()
			require.NoError(t, err)

			if i == 0 {
				tx := createStateTransactionWithData(f.config.StateReceiverAddr, inputData)
				txns = append(txns, tx)
			}
		}

		return f.VerifyStateTransactions(txns)
	}

	assert.NoError(t, executeForValidators("A", "B", "C", "D"))
	assert.ErrorContains(t, executeForValidators("A", "B", "C"), "quorum size not reached for state tx")
}

func TestFSM_VerifyStateTransaction_InvalidTypeOfStateTransactions(t *testing.T) {
	t.Parallel()

	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
	}

	var txns []*types.Transaction
	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, []byte{9, 3, 1, 1}))

	require.ErrorContains(t, f.VerifyStateTransactions(txns), "unknown state transaction")
}

func TestFSM_VerifyStateTransaction_QuorumNotReached(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessageSigned, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)
	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
		validators:    validators.toValidatorSet(),
	}

	hash, err := commitmentMessageSigned.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.getPrivateIdentities("A", "B"), hash)
	commitmentMessageSigned.AggSignature = *signature

	inputData, err := commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))

	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "quorum size not reached for state tx")
}

func TestFSM_VerifyStateTransaction_InvalidSignature(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessageSigned, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)
	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
		validators:    validators.toValidatorSet(),
	}

	hash, err := commitmentMessageSigned.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D"), hash)
	invalidValidator := newTestValidator(t, "G", 1)
	invalidSignature, err := invalidValidator.mustSign([]byte("malicious message")).Marshal()
	require.NoError(t, err)

	commitmentMessageSigned.AggSignature = Signature{
		Bitmap:              signature.Bitmap,
		AggregatedSignature: invalidSignature,
	}

	inputData, err := commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))

	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "invalid signature for tx")
}

func TestFSM_VerifyStateTransaction_TwoCommitmentMessages(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessageSigned, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)

	validatorSet := NewValidatorSet(validators.getPublicIdentities(), hclog.NewNullLogger())

	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
		validators:    validatorSet,
	}

	hash, err := commitmentMessageSigned.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D"), hash)
	commitmentMessageSigned.AggSignature = *signature

	inputData, err := commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
	inputData, err = commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "only one commitment tx is allowed per block")
}

func TestFSM_Validate_FailToVerifySignatures(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := newTestValidators(t, accountsCount)
	validatorsMetadata := validators.getPublicIdentities()

	extra := createTestExtraObject(validatorsMetadata, AccountSet{}, 4, signaturesCount, signaturesCount)
	validatorsHash, err := validatorsMetadata.Hash()
	require.NoError(t, err)

	extra.Checkpoint = &CheckpointData{CurrentValidatorsHash: validatorsHash, NextValidatorsHash: validatorsHash}
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}
	parent.ComputeHash()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorsMetadata, nil).Once()

	validatorSet := NewValidatorSet(validatorsMetadata, hclog.NewNullLogger())

	fsm := &fsm{
		parent:         parent,
		config:         &PolyBFTConfig{Bridge: &BridgeConfig{}},
		backend:        &blockchainMock{},
		polybftBackend: polybftBackendMock,
		validators:     validatorSet,
		logger:         hclog.NewNullLogger(),
	}

	finalBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{
			Number:     parentBlockNumber + 1,
			ParentHash: parent.Hash,
			Timestamp:  parent.Timestamp + 1,
			MixHash:    PolyBFTMixDigest,
			Difficulty: 1,
			ExtraData:  parent.ExtraData,
		},
	})

	checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), finalBlock.Number(), finalBlock.Hash())
	require.NoError(t, err)

	finalBlock.Header.Hash = checkpointHash
	proposal := finalBlock.MarshalRLP()

	assert.ErrorContains(t, fsm.Validate(proposal), "failed to verify signatures")

	polybftBackendMock.AssertExpectations(t)
}

func createDummyStateBlock(blockNumber uint64, parentHash types.Hash, extraData []byte) *types.FullBlock {
	finalBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{
			Number:     blockNumber,
			ParentHash: parentHash,
			Difficulty: 1,
			ExtraData:  extraData,
		},
	})

	return &types.FullBlock{Block: finalBlock}
}

func createTestExtra(
	allAccounts,
	previousValidatorSet AccountSet,
	validatorsCount,
	committedSignaturesCount,
	parentSignaturesCount int,
) []byte {
	extraData := createTestExtraObject(allAccounts, previousValidatorSet, validatorsCount, committedSignaturesCount, parentSignaturesCount)
	marshaled := extraData.MarshalRLPTo(nil)
	result := make([]byte, ExtraVanity+len(marshaled))
	copy(result[ExtraVanity:], marshaled)

	return result
}

func createTestCommitment(t *testing.T, accounts []*wallet.Account) *CommitmentMessageSigned {
	t.Helper()

	bitmap := bitmap.Bitmap{}
	stateSyncEvents := make([]*contractsapi.StateSyncedEvent, len(accounts))

	for i := 0; i < len(accounts); i++ {
		stateSyncEvents[i] = &contractsapi.StateSyncedEvent{
			ID:       big.NewInt(int64(i)),
			Sender:   types.Address(accounts[i].Ecdsa.Address()),
			Receiver: types.Address(accounts[0].Ecdsa.Address()),
			Data:     []byte{},
		}

		bitmap.Set(uint64(i))
	}

	commitment, err := NewPendingCommitment(1, stateSyncEvents)
	require.NoError(t, err)

	hash, err := commitment.Hash()
	require.NoError(t, err)

	var signatures bls.Signatures

	for _, a := range accounts {
		signature, err := a.Bls.Sign(hash.Bytes(), bls.DomainValidatorSet)
		assert.NoError(t, err)

		signatures = append(signatures, signature)
	}

	aggregatedSignature, err := signatures.Aggregate().Marshal()
	assert.NoError(t, err)

	signature := Signature{
		AggregatedSignature: aggregatedSignature,
		Bitmap:              bitmap,
	}

	assert.NoError(t, err)

	return &CommitmentMessageSigned{
		Message:      commitment.StateSyncCommitment,
		AggSignature: signature,
	}
}

func newBlockBuilderMock(stateBlock *types.FullBlock) *blockBuilderMock {
	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("Build", mock.Anything).Return(stateBlock).Once()
	mBlockBuilder.On("Fill", mock.Anything).Once()
	mBlockBuilder.On("Reset", mock.Anything).Return(error(nil)).Once()

	return mBlockBuilder
}

func createTestExtraObject(allAccounts,
	previousValidatorSet AccountSet,
	validatorsCount,
	committedSignaturesCount,
	parentSignaturesCount int) *Extra {
	accountCount := len(allAccounts)
	dummySignature := [64]byte{}
	bitmapCommitted, bitmapParent := bitmap.Bitmap{}, bitmap.Bitmap{}
	extraData := &Extra{}
	extraData.Validators = generateValidatorDelta(validatorsCount, allAccounts, previousValidatorSet)

	for j := range rand.Perm(accountCount)[:committedSignaturesCount] {
		bitmapCommitted.Set(uint64(j))
	}

	for j := range rand.Perm(accountCount)[:parentSignaturesCount] {
		bitmapParent.Set(uint64(j))
	}

	extraData.Parent = &Signature{Bitmap: bitmapCommitted, AggregatedSignature: dummySignature[:]}
	extraData.Committed = &Signature{Bitmap: bitmapParent, AggregatedSignature: dummySignature[:]}
	extraData.Checkpoint = &CheckpointData{}

	return extraData
}

func generateValidatorDelta(validatorCount int, allAccounts, previousValidatorSet AccountSet) (vd *ValidatorSetDelta) {
	oldMap := make(map[types.Address]int, previousValidatorSet.Len())
	for i, x := range previousValidatorSet {
		oldMap[x.Address] = i
	}

	vd = &ValidatorSetDelta{}
	vd.Removed = bitmap.Bitmap{}

	for _, id := range rand.Perm(len(allAccounts))[:validatorCount] {
		_, exists := oldMap[allAccounts[id].Address]
		if !exists {
			vd.Added = append(vd.Added, allAccounts[id])
		}

		delete(oldMap, allAccounts[id].Address)
	}

	for _, v := range oldMap {
		vd.Removed.Set(uint64(v))
	}

	return
}
