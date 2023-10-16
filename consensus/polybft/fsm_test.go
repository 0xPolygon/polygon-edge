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
	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFSM_ValidateHeader(t *testing.T) {
	t.Parallel()

	blockTimeDrift := uint64(1)
	extra := createTestExtra(validator.AccountSet{}, validator.AccountSet{}, 0, 0, 0)
	parent := &types.Header{Number: 0, Hash: types.BytesToHash([]byte{1, 2, 3})}
	header := &types.Header{Number: 0}

	// extra data
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "extra-data shorter than")
	header.ExtraData = extra

	// parent hash
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "incorrect header parent hash")
	header.ParentHash = parent.Hash

	// sequence number
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "invalid number")
	header.Number = 1

	// failed timestamp
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "timestamp older than parent")
	header.Timestamp = 10

	// failed nonce
	header.SetNonce(1)
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "invalid nonce")

	header.SetNonce(0)

	// failed gas
	header.GasLimit = 10
	header.GasUsed = 11
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "invalid gas limit")
	header.GasLimit = 10
	header.GasUsed = 10

	// mix digest
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "mix digest is not correct")
	header.MixHash = PolyBFTMixDigest

	// difficulty
	header.Difficulty = 0
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "difficulty should be greater than zero")

	header.Difficulty = 1
	header.Hash = types.BytesToHash([]byte{11, 22, 33})
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "invalid header hash")
	header.Timestamp = uint64(time.Now().UTC().Unix() + 150)
	require.ErrorContains(t, validateHeaderFields(parent, header, blockTimeDrift), "block from the future")

	header.Timestamp = uint64(time.Now().UTC().Unix())

	header.ComputeHash()
	require.NoError(t, validateHeaderFields(parent, header, blockTimeDrift))
}

func TestFSM_verifyCommitEpochTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		parent:           &types.Header{},
	}

	// include commit epoch transaction to the epoch ending block
	commitEpochTx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)
	assert.NotNil(t, commitEpochTx)

	assert.NoError(t, fsm.verifyCommitEpochTx(commitEpochTx))

	// submit tampered commit epoch transaction to the epoch ending block
	alteredCommitEpochTx := &types.Transaction{
		To:    &contracts.ValidatorSetContract,
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

	validators := validator.NewTestValidators(t, accountCount)
	validatorSet := validators.GetPublicIdentities()
	extra := createTestExtra(validatorSet, validator.AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	blockchainMock := &blockchainMock{}
	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{
			Key:        wallet.NewKey(validators.GetPrivateIdentities()[0]),
			blockchain: blockchainMock,
		},
	}

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: blockchainMock,
		validators: validators.ToValidatorSet(), exitEventRootHash: eventRoot, logger: hclog.NewNullLogger()}

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

	validators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	mBlockBuilder := newBlockBuilderMock(stateBlock)
	mBlockBuilder.On("WriteTx", mock.Anything).Return(error(nil)).Twice()

	blockChainMock := new(blockchainMock)

	runtime := &consensusRuntime{
		logger: hclog.NewNullLogger(),
		config: &runtimeConfig{
			Key:        wallet.NewKey(validators.GetPrivateIdentities()[0]),
			blockchain: blockChainMock,
		},
	}

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: blockChainMock,
		isEndOfEpoch:           true,
		validators:             validators.ToValidatorSet(),
		commitEpochInput:       createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput: createTestDistributeRewardsInput(t, 0, nil, 10),
		exitEventRootHash:      eventRoot,
		logger:                 hclog.NewNullLogger(),
	}

	proposal, err := fsm.BuildProposal(currentRound)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	assert.Equal(t, stateBlock.Block.MarshalRLP(), proposal)

	currentValidatorsHash, err := validators.GetPublicIdentities().Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := validators.ToValidatorSet().Accounts().Hash()
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
}

func TestFSM_BuildProposal_EpochEndingBlock_FailedToApplyStateTx(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		committedCount    = 4
		parentCount       = 3
		parentBlockNumber = 1023
	)

	validators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}

	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("WriteTx", mock.Anything).Return(errors.New("error")).Once()
	mBlockBuilder.On("Reset").Return(error(nil)).Once()

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, backend: &blockchainMock{},
		isEndOfEpoch:      true,
		validators:        validatorSet,
		commitEpochInput:  createTestCommitEpochInput(t, 0, 10),
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

	validators := validator.NewTestValidators(t, validatorsCount).GetPublicIdentities()
	extra := createTestExtraObject(validators, validator.AccountSet{}, validatorsCount-1, signaturesCount, signaturesCount)
	extra.Validators = nil

	extraData := extra.MarshalRLPTo(nil)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extraData}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extraData)

	blockBuilderMock := newBlockBuilderMock(stateBlock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Twice()

	addedValidators := validator.NewTestValidators(t, 2).GetPublicIdentities()
	removedValidators := [3]uint64{3, 4, 5}
	removedBitmap := &bitmap.Bitmap{}

	for _, i := range removedValidators {
		removedBitmap.Set(i)
	}

	newDelta := &validator.ValidatorSetDelta{
		Added:   addedValidators,
		Updated: validator.AccountSet{},
		Removed: *removedBitmap,
	}

	blockChainMock := new(blockchainMock)

	validatorSet := validator.NewValidatorSet(validators, hclog.NewNullLogger())

	fsm := &fsm{
		parent:                 parent,
		blockBuilder:           blockBuilderMock,
		config:                 &PolyBFTConfig{},
		backend:                blockChainMock,
		isEndOfEpoch:           true,
		validators:             validatorSet,
		commitEpochInput:       createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput: createTestDistributeRewardsInput(t, 0, validators, 10),
		exitEventRootHash:      types.ZeroHash,
		logger:                 hclog.NewNullLogger(),
		newValidatorsDelta:     newDelta,
	}

	proposal, err := fsm.BuildProposal(0)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.Len(t, blockExtra.Validators.Added, 2)
	assert.False(t, blockExtra.Validators.IsEmpty())

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
	blockChainMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_NonEpochEndingBlock_ValidatorsDeltaNil(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 9
	)

	testValidators := validator.NewTestValidators(t, accountCount)
	extra := createTestExtra(testValidators.GetPublicIdentities(), validator.AccountSet{}, accountCount-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	blockBuilderMock := &blockBuilderMock{}
	blockBuilderMock.On("Build", mock.Anything).Return(stateBlock).Once()
	blockBuilderMock.On("Fill").Once()
	blockBuilderMock.On("Reset").Return(error(nil)).Once()

	fsm := &fsm{parent: parent, blockBuilder: blockBuilderMock,
		config: &PolyBFTConfig{}, backend: &blockchainMock{},
		isEndOfEpoch: false, validators: testValidators.ToValidatorSet(),
		exitEventRootHash: types.ZeroHash, logger: hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(0)
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.Nil(t, blockExtra.Validators)

	blockBuilderMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_FailToGetNextValidatorsHash(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 49
	)

	testValidators := validator.NewTestValidators(t, accountCount)
	allAccounts := testValidators.GetPublicIdentities()
	extra := createTestExtraObject(allAccounts, validator.AccountSet{}, accountCount-1, signaturesCount, signaturesCount)
	extra.Validators = nil

	newValidatorDelta := &validator.ValidatorSetDelta{
		// this will prompt an error since all the validators are already in the validator set
		Added: testValidators.ToValidatorSet().Accounts(),
	}

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra.MarshalRLPTo(nil)}

	blockBuilderMock := new(blockBuilderMock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Twice()
	blockBuilderMock.On("Reset").Return(error(nil)).Once()
	blockBuilderMock.On("Fill").Once()

	fsm := &fsm{parent: parent,
		blockBuilder:           blockBuilderMock,
		config:                 &PolyBFTConfig{},
		isEndOfEpoch:           true,
		validators:             testValidators.ToValidatorSet(),
		commitEpochInput:       createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput: createTestDistributeRewardsInput(t, 0, allAccounts, 10),
		exitEventRootHash:      types.ZeroHash,
		newValidatorsDelta:     newValidatorDelta,
	}

	proposal, err := fsm.BuildProposal(0)
	assert.ErrorContains(t, err, "already present in the validators snapshot")
	assert.Nil(t, proposal)

	blockBuilderMock.AssertNotCalled(t, "Build")
	blockBuilderMock.AssertExpectations(t)
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{commitEpochInput: createTestCommitEpochInput(t, 0, 10), parent: &types.Header{}}
	tx, err := fsm.createCommitEpochTx()
	assert.NoError(t, err)
	err = fsm.VerifyStateTransactions([]*types.Transaction{tx})
	assert.ErrorContains(t, err, err.Error())
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{commitEpochInput: createTestCommitEpochInput(t, 0, 10), parent: &types.Header{}}
	err := fsm.VerifyStateTransactions([]*types.Transaction{})
	assert.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_EndOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{isEndOfEpoch: true, commitEpochInput: createTestCommitEpochInput(t, 0, 10)}
	assert.EqualError(t, fsm.VerifyStateTransactions([]*types.Transaction{}),
		"commit epoch transaction is not found in the epoch ending block")
}

func TestFSM_VerifyStateTransactions_EndOfEpochWrongCommitEpochTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		parent:           &types.Header{},
	}
	commitEpochInput, err := createTestCommitEpochInput(t, 1, 5).EncodeAbi()
	require.NoError(t, err)

	commitEpochTx := createStateTransactionWithData(1, contracts.ValidatorSetContract, commitEpochInput)
	assert.ErrorContains(t, fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx}), "invalid commit epoch transaction")
}

func TestFSM_VerifyStateTransactions_CommitmentTransactionAndSprintIsFalse(t *testing.T) {
	t.Parallel()

	fsm := &fsm{parent: &types.Header{}}

	encodedCommitment, err := createTestCommitmentMessage(t, 1).EncodeAbi()
	require.NoError(t, err)

	tx := createStateTransactionWithData(1, contracts.StateReceiverContract, encodedCommitment)
	assert.ErrorContains(t, fsm.VerifyStateTransactions([]*types.Transaction{tx}),
		"found commitment tx in block which should not contain it")
}

func TestFSM_VerifyStateTransactions_EndOfEpochMoreThanOneCommitEpochTx(t *testing.T) {
	t.Parallel()

	txs := make([]*types.Transaction, 2)
	fsm := &fsm{
		isEndOfEpoch:     true,
		commitEpochInput: createTestCommitEpochInput(t, 0, 10),
		parent:           &types.Header{},
	}

	commitEpochTxOne, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	txs[0] = commitEpochTxOne

	commitEpochTxTwo := createTestCommitEpochInput(t, 0, 100)
	input, err := commitEpochTxTwo.EncodeAbi()
	require.NoError(t, err)

	txs[1] = createStateTransactionWithData(1, types.ZeroAddress, input)

	assert.ErrorIs(t, fsm.VerifyStateTransactions(txs), errCommitEpochTxSingleExpected)
}

func TestFSM_VerifyStateTransactions_StateTransactionPass(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 5)

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		parent:                 &types.Header{Number: 1},
		isEndOfEpoch:           true,
		isEndOfSprint:          true,
		validators:             validatorSet,
		commitEpochInput:       createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput: createTestDistributeRewardsInput(t, 0, validators.GetPublicIdentities(), 10),
		logger:                 hclog.NewNullLogger(),
	}

	// add commit epoch commitEpochTx to the end of transactions list
	commitEpochTx, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	distributeRewardsTx, err := fsm.createDistributeRewardsTx()
	require.NoError(t, err)

	err = fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx, distributeRewardsTx})
	require.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_StateTransactionQuorumNotReached(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 5)
	commitment := createTestCommitment(t, validators.GetPrivateIdentities())
	commitment.AggSignature = Signature{
		AggregatedSignature: []byte{1, 2},
		Bitmap:              []byte{},
	}

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		parent:                       &types.Header{Number: 2},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		validators:                   validatorSet,
		proposerCommitmentToRegister: commitment,
		commitEpochInput:             createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput:       createTestDistributeRewardsInput(t, 0, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	bridgeCommitmentTx, err := fsm.createBridgeCommitmentTx()
	require.NoError(t, err)

	// add commit epoch commitEpochTx to the end of transactions list
	commitEpochTx, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	stateTxs := []*types.Transaction{commitEpochTx, bridgeCommitmentTx}

	err = fsm.VerifyStateTransactions(stateTxs)
	require.ErrorContains(t, err, "quorum size not reached")
}

func TestFSM_VerifyStateTransactions_StateTransactionInvalidSignature(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 5)
	commitment := createTestCommitment(t, validators.GetPrivateIdentities())
	nonValidators := validator.NewTestValidators(t, 3)
	aggregatedSigs := bls.Signatures{}

	nonValidators.IterAcct(nil, func(t *validator.TestValidator) {
		aggregatedSigs = append(aggregatedSigs, t.MustSign([]byte("dummyHash"), signer.DomainStateReceiver))
	})

	sig, err := aggregatedSigs.Aggregate().Marshal()
	require.NoError(t, err)

	commitment.AggSignature.AggregatedSignature = sig

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	fsm := &fsm{
		parent:                       &types.Header{Number: 1},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		validators:                   validatorSet,
		proposerCommitmentToRegister: commitment,
		commitEpochInput:             createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput:       createTestDistributeRewardsInput(t, 0, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	bridgeCommitmentTx, err := fsm.createBridgeCommitmentTx()
	require.NoError(t, err)

	// add commit epoch commitEpochTx to the end of transactions list
	commitEpochTx, err := fsm.createCommitEpochTx()
	require.NoError(t, err)

	err = fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx, bridgeCommitmentTx})
	require.ErrorContains(t, err, "invalid signature")
}

func TestFSM_ValidateCommit_WrongValidator(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.ToValidatorSet(), logger: hclog.NewNullLogger(), exitEventRootHash: types.ZeroHash}

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

	validators := validator.NewTestValidators(t, accountsCount)

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.ToValidatorSet(), exitEventRootHash: types.ZeroHash, logger: hclog.NewNullLogger()}

	_, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	nonValidatorAcc := validator.NewTestValidator(t, "non_validator", 1)
	wrongSignature, err := nonValidatorAcc.MustSign([]byte("Foo"), signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	err = fsm.ValidateCommit(validators.GetValidator("0").Address().Bytes(), wrongSignature, []byte{})
	require.ErrorContains(t, err, "incorrect commit signature from")
}

func TestFSM_ValidateCommit_Good(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.GetPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber, ExtraData: createTestExtra(validatorsMetadata, validator.AccountSet{}, 5, 3, 3)}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)

	validatorSet := validator.NewValidatorSet(validatorsMetadata, hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators:        validatorSet,
		exitEventRootHash: types.ZeroHash,
		logger:            hclog.NewNullLogger()}

	proposal, err := fsm.BuildProposal(0)
	require.NoError(t, err)

	block := types.Block{}
	require.NoError(t, block.UnmarshalRLP(proposal))

	validator := validators.GetValidator("A")
	seal, err := validator.MustSign(block.Hash().Bytes(), signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)
	err = fsm.ValidateCommit(validator.Key().Address().Bytes(), seal, block.Hash().Bytes())
	require.NoError(t, err)
}

func TestFSM_Validate_ExitEventRootNotExpected(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities(), nil).Once()

	extra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	extra.Validators = nil
	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	currentValSetHash, err := validators.GetPublicIdentities().Hash()
	require.NoError(t, err)

	extra.Parent = createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)
	extra.Checkpoint.EpochNumber = 1
	extra.Checkpoint.CurrentValidatorsHash = currentValSetHash
	extra.Checkpoint.NextValidatorsHash = currentValSetHash

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, extra.MarshalRLPTo(nil))

	proposalHash, err := extra.Checkpoint.Hash(0, stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	stateBlock.Block.Header.Timestamp = uint64(time.Now().UTC().Unix())
	stateBlock.Block.Transactions = []*types.Transaction{}

	proposal := stateBlock.Block.MarshalRLP()

	fsm := &fsm{
		parent:            parent,
		backend:           new(blockchainMock),
		validators:        validators.ToValidatorSet(),
		logger:            hclog.NewNullLogger(),
		polybftBackend:    polybftBackendMock,
		config:            &PolyBFTConfig{BlockTimeDrift: 1},
		exitEventRootHash: types.BytesToHash([]byte{0, 1, 2, 3, 4}), // expect this to be in proposal extra
	}

	err = fsm.Validate(proposal)
	require.ErrorContains(t, err, "exit root hash not as expected")

	polybftBackendMock.AssertExpectations(t)
}

func TestFSM_Validate_EpochEndingBlock_MismatchInDeltas(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities(), nil).Once()

	extra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	extra.Validators = &validator.ValidatorSetDelta{} // this will cause test to fail
	extra.Parent = createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, extra.MarshalRLPTo(nil))

	proposalHash, err := new(CheckpointData).Hash(0, stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	commitEpoch := createTestCommitEpochInput(t, 1, 10)
	commitEpochTxInput, err := commitEpoch.EncodeAbi()
	require.NoError(t, err)

	distributeRewards := createTestDistributeRewardsInput(t, 1, validators.GetPublicIdentities(), 10)
	distributeRewardsTxInput, err := distributeRewards.EncodeAbi()
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	stateBlock.Block.Header.Timestamp = uint64(time.Now().UTC().Unix())
	stateBlock.Block.Transactions = []*types.Transaction{
		createStateTransactionWithData(1, contracts.ValidatorSetContract, commitEpochTxInput),
		createStateTransactionWithData(1, contracts.RewardPoolContract, distributeRewardsTxInput),
	}

	proposal := stateBlock.Block.MarshalRLP()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("ProcessBlock", mock.Anything, mock.Anything).
		Return(stateBlock, error(nil)).
		Maybe()

	// a new validator is added to delta which proposers block does not have
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorDelta := &validator.ValidatorSetDelta{
		Added: validator.AccountSet{&validator.ValidatorMetadata{
			Address:     types.BytesToAddress([]byte{0, 1, 2, 3}),
			BlsKey:      privateKey.PublicKey(),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		}},
	}

	fsm := &fsm{
		parent:                 parent,
		backend:                blockchainMock,
		validators:             validators.ToValidatorSet(),
		logger:                 hclog.NewNullLogger(),
		isEndOfEpoch:           true,
		commitEpochInput:       commitEpoch,
		distributeRewardsInput: distributeRewards,
		polybftBackend:         polybftBackendMock,
		newValidatorsDelta:     newValidatorDelta,
		config:                 &PolyBFTConfig{BlockTimeDrift: 1},
	}

	err = fsm.Validate(proposal)
	require.ErrorIs(t, err, errValidatorSetDeltaMismatch)

	polybftBackendMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestFSM_Validate_EpochEndingBlock_UpdatingValidatorSetInNonEpochEndingBlock(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parentExtra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentExtra.Validators = nil

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: parentExtra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities(), nil).Once()

	// a new validator is added to delta which proposers block does not have
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorDelta := &validator.ValidatorSetDelta{
		Added: validator.AccountSet{&validator.ValidatorMetadata{
			Address:     types.BytesToAddress([]byte{0, 1, 2, 3}),
			BlsKey:      privateKey.PublicKey(),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		}},
	}

	extra := createTestExtraObject(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	parentCheckpointHash, err := extra.Checkpoint.Hash(0, parentBlockNumber, parent.Hash)
	require.NoError(t, err)

	extra.Validators = newValidatorDelta // this will cause test to fail
	extra.Parent = createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, extra.MarshalRLPTo(nil))

	proposalHash, err := new(CheckpointData).Hash(0, stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	stateBlock.Block.Header.Hash = proposalHash
	stateBlock.Block.Header.ParentHash = parent.Hash
	stateBlock.Block.Header.Timestamp = uint64(time.Now().UTC().Unix())

	proposal := stateBlock.Block.MarshalRLP()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("ProcessBlock", mock.Anything, mock.Anything).
		Return(stateBlock, error(nil)).
		Maybe()

	fsm := &fsm{
		parent:         parent,
		backend:        blockchainMock,
		validators:     validators.ToValidatorSet(),
		logger:         hclog.NewNullLogger(),
		polybftBackend: polybftBackendMock,
		config:         &PolyBFTConfig{BlockTimeDrift: 1},
	}

	err = fsm.Validate(proposal)
	require.ErrorIs(t, err, errValidatorsUpdateInNonEpochEnding)

	polybftBackendMock.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestFSM_Validate_IncorrectHeaderParentHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := validator.NewTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	fsm := &fsm{
		parent:     parent,
		backend:    &blockchainMock{},
		validators: validators.ToValidatorSet(),
		logger:     hclog.NewNullLogger(),
		config: &PolyBFTConfig{
			BlockTimeDrift: 1,
		},
	}

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

	validators := validator.NewTestValidators(t, accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	// try some invalid block numbers, parentBlockNumber + 1 should be correct
	for _, blockNum := range []uint64{parentBlockNumber - 1, parentBlockNumber, parentBlockNumber + 2} {
		stateBlock := createDummyStateBlock(blockNum, parent.Hash, parent.ExtraData)
		mBlockBuilder := newBlockBuilderMock(stateBlock)
		fsm := &fsm{
			parent:       parent,
			blockBuilder: mBlockBuilder,
			backend:      &blockchainMock{},
			validators:   validators.ToValidatorSet(),
			logger:       hclog.NewNullLogger(),
			config:       &PolyBFTConfig{BlockTimeDrift: 1},
		}

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

	validators := validator.NewTestValidators(t, 5)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, 3, 3),
		Timestamp: uint64(time.Now().UTC().Unix()),
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
		fsm := &fsm{
			parent:     parent,
			backend:    &blockchainMock{},
			validators: validators.ToValidatorSet(),
			logger:     hclog.NewNullLogger(),
			config: &PolyBFTConfig{
				BlockTimeDrift: 1,
			}}

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

	validators := validator.NewTestValidators(t, 5)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.GetPublicIdentities(), validator.AccountSet{}, 4, 3, 3),
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
		backend:    &blockchainMock{},
		validators: validators.ToValidatorSet(),
		logger:     hclog.NewNullLogger(),
		config: &PolyBFTConfig{
			BlockTimeDrift: 1,
		},
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
		validators := validator.NewTestValidators(t, accountCount)
		allAccounts := validators.GetPrivateIdentities()
		validatorsMetadata := validators.GetPublicIdentities()

		extraParent := createTestExtra(validatorsMetadata, validator.AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
		parent := &types.Header{Number: parentBlockNumber, ExtraData: extraParent}
		extraBlock := createTestExtra(validatorsMetadata, validator.AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
		block := consensus.BuildBlock(
			consensus.BuildBlockParams{
				Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
			})

		builtBlock := &types.FullBlock{Block: block}

		builderMock := newBlockBuilderMock(builtBlock)
		chainMock := &blockchainMock{}
		chainMock.On("CommitBlock", mock.Anything).Return(error(nil)).Once()
		chainMock.On("ProcessBlock", mock.Anything, mock.Anything).
			Return(builtBlock, error(nil)).
			Maybe()

		f := &fsm{
			parent:       parent,
			blockBuilder: builderMock,
			target:       builtBlock,
			backend:      chainMock,
			validators:   validator.NewValidatorSet(validatorsMetadata[0:len(validatorsMetadata)-1], hclog.NewNullLogger()),
			logger:       hclog.NewNullLogger(),
		}

		seals := make([]*messages.CommittedSeal, signaturesCount)

		for i := 0; i < signaturesCount; i++ {
			sign, err := allAccounts[i].Bls.Sign(builtBlock.Block.Hash().Bytes(), signer.DomainCheckpointManager)
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

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.GetPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber}
	parent.ComputeHash()

	extraBlock := createTestExtra(validatorsMetadata, validator.AccountSet{}, len(validators.Validators)-1, signaturesCount, signaturesCount)
	finalBlock := consensus.BuildBlock(
		consensus.BuildBlockParams{
			Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
		})

	buildBlock := &types.FullBlock{Block: finalBlock, Receipts: []*types.Receipt{}}
	mBlockBuilder := newBlockBuilderMock(buildBlock)

	validatorSet := validator.NewValidatorSet(validatorsMetadata[0:len(validatorsMetadata)-1], hclog.NewNullLogger())

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, backend: &blockchainMock{},
		validators: validatorSet,
	}

	proposal := buildBlock.Block.MarshalRLP()
	validatorA := validators.GetValidator("A")
	validatorB := validators.GetValidator("B")
	proposalHash := buildBlock.Block.Hash().Bytes()
	sigA, err := validatorA.MustSign(proposalHash, signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	sigB, err := validatorB.MustSign(proposalHash, signer.DomainCheckpointManager).Marshal()
	require.NoError(t, err)

	// create test account outside of validator set
	nonValidatorAccount := validator.NewTestValidator(t, "non_validator", 1)
	nonValidatorSignature, err := nonValidatorAccount.MustSign(proposalHash, signer.DomainCheckpointManager).Marshal()
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
		proposerCommitmentToRegister: signedCommitment,
		commitEpochInput:             createTestCommitEpochInput(t, 0, 10),
		distributeRewardsInput:       createTestDistributeRewardsInput(t, 0, nil, 10),
		logger:                       hclog.NewNullLogger(),
		parent:                       &types.Header{},
	}

	bridgeCommitmentTx, err := f.createBridgeCommitmentTx()
	require.NoError(t, err)

	decodedData, err := decodeStateTransaction(bridgeCommitmentTx.Input)
	require.NoError(t, err)

	decodedCommitmentMsg, ok := decodedData.(*CommitmentMessageSigned)
	require.True(t, ok)
	require.Equal(t, signedCommitment, decodedCommitmentMsg)
}

func TestFSM_DecodeCommitEpochStateTx(t *testing.T) {
	t.Parallel()

	commitEpoch := createTestCommitEpochInput(t, 0, 10)
	input, err := commitEpoch.EncodeAbi()
	require.NoError(t, err)
	require.NotNil(t, input)

	tx := createStateTransactionWithData(1, contracts.ValidatorSetContract, input)
	decodedInputData, err := decodeStateTransaction(tx.Input)
	require.NoError(t, err)

	decodedCommitEpoch, ok := decodedInputData.(*contractsapi.CommitEpochValidatorSetFn)
	require.True(t, ok)
	require.True(t, commitEpoch.ID.Cmp(decodedCommitEpoch.ID) == 0)
	require.NotNil(t, decodedCommitEpoch.Epoch)
	require.True(t, commitEpoch.Epoch.StartBlock.Cmp(decodedCommitEpoch.Epoch.StartBlock) == 0)
	require.True(t, commitEpoch.Epoch.EndBlock.Cmp(decodedCommitEpoch.Epoch.EndBlock) == 0)
}

func TestFSM_VerifyStateTransaction_ValidBothTypesOfStateTransactions(t *testing.T) {
	t.Parallel()

	var (
		commitments       [2]*PendingCommitment
		stateSyncs        [2][]*contractsapi.StateSyncedEvent
		signedCommitments [2]*CommitmentMessageSigned
	)

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"})
	commitments[0], signedCommitments[0], stateSyncs[0] = buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)
	commitments[1], signedCommitments[1], stateSyncs[1] = buildCommitmentAndStateSyncs(t, 10, uint64(3), 12)

	executeForValidators := func(aliases ...string) error {
		for _, sc := range signedCommitments {
			// add register commitment state transaction
			hash, err := sc.Hash()
			require.NoError(t, err)
			signature := createSignature(t, validators.GetPrivateIdentities(aliases...), hash, signer.DomainStateReceiver)
			sc.AggSignature = *signature
		}

		f := &fsm{
			parent:        &types.Header{Number: 9},
			isEndOfSprint: true,
			validators:    validators.ToValidatorSet(),
		}

		var txns []*types.Transaction

		for i, sc := range signedCommitments {
			inputData, err := sc.EncodeAbi()
			require.NoError(t, err)

			if i == 0 {
				tx := createStateTransactionWithData(1, contracts.StateReceiverContract, inputData)
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
	}

	var txns []*types.Transaction
	txns = append(txns,
		createStateTransactionWithData(1, contracts.StateReceiverContract, []byte{9, 3, 1, 1}))

	require.ErrorContains(t, f.VerifyStateTransactions(txns), "unknown state transaction")
}

func TestFSM_VerifyStateTransaction_QuorumNotReached(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessageSigned, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)
	f := &fsm{
		parent:        &types.Header{Number: 9},
		isEndOfSprint: true,
		validators:    validators.ToValidatorSet(),
	}

	hash, err := commitmentMessageSigned.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.GetPrivateIdentities("A", "B"), hash, signer.DomainStateReceiver)
	commitmentMessageSigned.AggSignature = *signature

	inputData, err := commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(1, contracts.StateReceiverContract, inputData))

	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "quorum size not reached for state tx")
}

func TestFSM_VerifyStateTransaction_InvalidSignature(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessageSigned, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)
	f := &fsm{
		parent:        &types.Header{Number: 9},
		isEndOfSprint: true,
		validators:    validators.ToValidatorSet(),
	}

	hash, err := commitmentMessageSigned.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.GetPrivateIdentities("A", "B", "C", "D"), hash, signer.DomainStateReceiver)
	invalidValidator := validator.NewTestValidator(t, "G", 1)
	invalidSignature, err := invalidValidator.MustSign([]byte("malicious message"), signer.DomainStateReceiver).Marshal()
	require.NoError(t, err)

	commitmentMessageSigned.AggSignature = Signature{
		Bitmap:              signature.Bitmap,
		AggregatedSignature: invalidSignature,
	}

	inputData, err := commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(1, contracts.StateReceiverContract, inputData))

	require.ErrorContains(t, f.VerifyStateTransactions(txns), "invalid signature for state tx")
}

func TestFSM_VerifyStateTransaction_TwoCommitmentMessages(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessageSigned, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), 2)

	validatorSet := validator.NewValidatorSet(validators.GetPublicIdentities(), hclog.NewNullLogger())

	f := &fsm{
		parent:        &types.Header{Number: 9},
		isEndOfSprint: true,
		validators:    validatorSet,
	}

	hash, err := commitmentMessageSigned.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.GetPrivateIdentities("A", "B", "C", "D"), hash, signer.DomainStateReceiver)
	commitmentMessageSigned.AggSignature = *signature

	inputData, err := commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(1, contracts.StateReceiverContract, inputData))
	inputData, err = commitmentMessageSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(1, contracts.StateReceiverContract, inputData))
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

	validators := validator.NewTestValidators(t, accountsCount)
	validatorsMetadata := validators.GetPublicIdentities()

	extra := createTestExtraObject(validatorsMetadata, validator.AccountSet{}, 4, signaturesCount, signaturesCount)
	validatorsHash, err := validatorsMetadata.Hash()
	require.NoError(t, err)

	extra.Checkpoint = &CheckpointData{CurrentValidatorsHash: validatorsHash, NextValidatorsHash: validatorsHash}
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: extra.MarshalRLPTo(nil),
	}
	parent.ComputeHash()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorsMetadata, nil).Once()

	validatorSet := validator.NewValidatorSet(validatorsMetadata, hclog.NewNullLogger())

	fsm := &fsm{
		parent:         parent,
		backend:        &blockchainMock{},
		polybftBackend: polybftBackendMock,
		validators:     validatorSet,
		logger:         hclog.NewNullLogger(),
		config: &PolyBFTConfig{
			BlockTimeDrift: 1,
		},
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
			MixHash:    PolyBFTMixDigest,
		},
	})

	return &types.FullBlock{Block: finalBlock}
}

func createTestExtra(
	allAccounts,
	previousValidatorSet validator.AccountSet,
	validatorsCount,
	committedSignaturesCount,
	parentSignaturesCount int,
) []byte {
	extraData := createTestExtraObject(allAccounts, previousValidatorSet, validatorsCount, committedSignaturesCount, parentSignaturesCount)

	return extraData.MarshalRLPTo(nil)
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
		signature, err := a.Bls.Sign(hash.Bytes(), signer.DomainStateReceiver)
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
	previousValidatorSet validator.AccountSet,
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

func generateValidatorDelta(validatorCount int, allAccounts, previousValidatorSet validator.AccountSet) (vd *validator.ValidatorSetDelta) {
	oldMap := make(map[types.Address]int, previousValidatorSet.Len())
	for i, x := range previousValidatorSet {
		oldMap[x.Address] = i
	}

	vd = &validator.ValidatorSetDelta{}
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
