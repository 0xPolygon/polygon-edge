package polybft

/*
import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	pbft "github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func TestFSM_ValidateHeader(t *testing.T) {
	t.Parallel()

	parent := &types.Header{Number: 0}
	parent.ComputeHash()

	header := &types.Header{Number: 0}

	// parent hash
	assert.ErrorContains(t, validateHeaderFields(parent, header), "incorrect header parent hash")
	header.ParentHash = parent.Hash

	// sequence number
	assert.ErrorContains(t, validateHeaderFields(parent, header), "invalid number")
	header.Number = 1

	// failed timestamp
	assert.ErrorContains(t, validateHeaderFields(parent, header), "timestamp older than parent")
	header.Timestamp = 10

	// mix digest
	assert.ErrorContains(t, validateHeaderFields(parent, header), "mix digest is not correct")
	header.MixHash = PolyBFTMixDigest

	// difficulty
	header.Difficulty = 0
	assert.ErrorContains(t, validateHeaderFields(parent, header), "difficulty should be greater than zero")

	header.Difficulty = 1
	assert.NoError(t, validateHeaderFields(parent, header))
}

func TestFSM_verifyValidatorsUptimeTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{
		config:        &PolyBFTConfig{ValidatorSetAddr: contracts.ValidatorSetContract},
		isEndOfEpoch:  true,
		uptimeCounter: createTestUptimeCounter(t, nil, 10),
	}

	// include uptime transaction to the epoch ending block
	uptimeTx, err := fsm.createValidatorsUptimeTx()
	assert.NoError(t, err)
	assert.NotNil(t, uptimeTx)
	transactions := []*types.Transaction{uptimeTx}
	block := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{GasLimit: types.StateTransactionGasLimit},
		Txns:   transactions,
	})
	assert.NoError(t, fsm.verifyValidatorsUptimeTx(block.Transactions))

	// don't include validators uptime transaction to the epoch ending block
	block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{GasLimit: types.StateTransactionGasLimit},
	})
	assert.Error(t, fsm.verifyValidatorsUptimeTx(block.Transactions))

	// submit tampered validators uptime transaction to the epoch ending block
	alteredUptimeTx := &types.Transaction{
		To:    &fsm.config.ValidatorSetAddr,
		Input: []byte{},
		Gas:   0,
		Type:  types.StateTx,
	}
	transactions = []*types.Transaction{alteredUptimeTx}
	block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{GasLimit: types.StateTransactionGasLimit},
		Txns:   transactions,
	})
	assert.Error(t, fsm.verifyValidatorsUptimeTx(block.Transactions))

	fsm.isEndOfEpoch = false
	// submit validators uptime transaction to the non-epoch ending block
	uptimeTx, err = fsm.createValidatorsUptimeTx()
	assert.NoError(t, err)
	assert.NotNil(t, uptimeTx)
	transactions = []*types.Transaction{uptimeTx}
	block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{GasLimit: types.StateTransactionGasLimit},
		Txns:   transactions,
	})
	assert.Error(t, fsm.verifyValidatorsUptimeTx(block.Transactions))

	// create block with dummy transaction in non-epoch ending block
	dummyTx := &types.Transaction{
		Nonce: 1,
		Gas:   1000000,
		To:    &types.Address{},
		Value: big.NewInt(1),
	}
	transactions = []*types.Transaction{dummyTx}
	block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{GasLimit: types.StateTransactionGasLimit},
		Txns:   transactions,
	})
	assert.NoError(t, fsm.verifyValidatorsUptimeTx(block.Transactions))
}

func TestFSM_Init(t *testing.T) {
	t.Parallel()

	mblockBuilder := &blockBuilderMock{}
	mblockBuilder.On("Reset").Once()

	fsm := &fsm{blockBuilder: mblockBuilder}
	fsm.Init(&pbft.RoundInfo{})
	assert.Nil(t, fsm.proposal)
	mblockBuilder.AssertExpectations(t)
}

func TestFSM_BuildProposal_WithExitEvents(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		committedCount    = 4
		parentCount       = 3
		parentBlockNumber = 1023
		numOfReceipts     = 10
		epoch             = 100
	)

	runtime := &consensusRuntime{
		state: newTestState(t),
	}

	validators := newTestValidators(accountCount)
	validatorSet := validators.getPublicIdentities()
	extra := createTestExtra(validatorSet, AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)
	receipts := make([]*types.Receipt, numOfReceipts)

	personType := abi.MustNewType("tuple(string company, string address)")
	encodedData, err := personType.Encode(map[string]string{"company": "Polygon", "address": "Planet Earth"})
	require.NoError(t, err)

	for i := 0; i < numOfReceipts; i++ {
		receipts[i] = &types.Receipt{Logs: []*types.Log{
			{
				Address: types.ZeroAddress,
				Data:    encodedData,
				Topics: []types.Hash{
					types.Hash(exitEventABI.ID()),
					types.BytesToHash([]byte{uint8(i)}),
					types.BytesToHash(types.StringToAddress("0x1111").Bytes()),
					types.BytesToHash(types.StringToAddress("0x2222").Bytes()),
				},
			},
		}}
	}

	blockchainMock := new(blockchainMock)
	blockchainMock.On("CommitBlock", mock.Anything).Return(nil).Once()

	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("Build", mock.Anything).Return(stateBlock).Once()
	mBlockBuilder.On("Fill").Once()
	mBlockBuilder.On("Receipts", mock.Anything).Return(receipts).Once()
	stateBlock.Receipts = receipts

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: blockchainMock,
		validators: validators.toValidatorSet(), checkpointBackend: runtime, logger: hclog.NewNullLogger(),
		epochNumber: epoch, postInsertHook: func() error { return nil },
		roundInfo: &pbft.RoundInfo{},
	}

	proposal, err := fsm.BuildProposal()
	require.NoError(t, err)
	require.NotNil(t, proposal)

	sealedProposal := &pbft.SealedProposal{
		Proposal: proposal,
	}

	require.NoError(t, fsm.Insert(sealedProposal))

	events, err := runtime.state.getExitEventsByEpoch(epoch)
	require.NoError(t, err)
	require.Len(t, events, numOfReceipts)

	mBlockBuilder.AssertExpectations(t)
	blockchainMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_WithExitEvents_ErrorInDecoding(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		committedCount    = 4
		parentCount       = 3
		parentBlockNumber = 1023
		epoch             = 100
	)

	runtime := &consensusRuntime{
		state: newTestState(t),
	}

	validators := newTestValidators(accountCount)
	validatorSet := validators.getPublicIdentities()
	extra := createTestExtra(validatorSet, AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()

	receipt := &types.Receipt{Logs: []*types.Log{
		{
			Address: types.ZeroAddress,
			Data:    []byte{0, 1}, // invalid data
			Topics: []types.Hash{
				types.Hash(exitEventABI.ID()),
				types.BytesToHash([]byte{111}),
				types.BytesToHash(types.StringToAddress("0x1111").Bytes()),
				types.BytesToHash(types.StringToAddress("0x2222").Bytes()),
			},
		},
	}}

	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("Fill").Once()
	mBlockBuilder.On("Receipts", mock.Anything).Return([]*types.Receipt{receipt}).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), checkpointBackend: runtime, logger: hclog.NewNullLogger(), epochNumber: epoch}

	proposal, err := fsm.BuildProposal()
	assert.Error(t, err)
	assert.Nil(t, proposal)

	events, err := runtime.state.getExitEventsByEpoch(epoch)
	require.NoError(t, err)
	require.Len(t, events, 0)

	mBlockBuilder.AssertExpectations(t)
}

func TestFSM_BuildProposal_WithoutUptimeTxGood(t *testing.T) {
	t.Parallel()

	const (
		accountCount             = 5
		committedCount           = 4
		parentCount              = 3
		confirmedStateSyncsCount = 5
		parentBlockNumber        = 1023
	)

	validators := newTestValidators(accountCount)
	validatorSet := validators.getPublicIdentities()
	extra := createTestExtra(validatorSet, AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)
	mBlockBuilder := newBlockBuilderMock(stateBlock)
	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), checkpointBackend: checkpointBackendMock, logger: hclog.NewNullLogger(),
		roundInfo: &pbft.RoundInfo{}}

	proposal, err := fsm.BuildProposal()
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	currentValidatorsHash, err := validatorSet.Hash()
	require.NoError(t, err)

	checkpoint := &CheckpointData{
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    currentValidatorsHash,
	}

	checkpointHash, err := checkpoint.Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	rlpBlock := stateBlock.Block.MarshalRLP()
	assert.Equal(t, rlpBlock, proposal.Data)
	assert.Equal(t, checkpointHash, types.BytesToHash(proposal.Hash))

	mBlockBuilder.AssertExpectations(t)
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_WithUptimeTxGood(t *testing.T) {
	t.Parallel()

	const (
		accountCount             = 5
		committedCount           = 4
		parentCount              = 3
		confirmedStateSyncsCount = 5
		parentBlockNumber        = 1023
	)

	validators := newTestValidators(accountCount)
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

	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: blockChainMock,
		isEndOfEpoch:      true,
		validators:        validators.toValidatorSet(),
		uptimeCounter:     createTestUptimeCounter(t, nil, 10),
		checkpointBackend: checkpointBackendMock,
		logger:            hclog.NewNullLogger(),
		roundInfo:         &pbft.RoundInfo{},
	}

	proposal, err := fsm.BuildProposal()
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	rlpBlock := stateBlock.Block.MarshalRLP()
	currentValidatorsHash, err := validators.getPublicIdentities().Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := AccountSet{}.Hash()
	require.NoError(t, err)

	checkpoint := &CheckpointData{
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    nextValidatorsHash,
	}

	checkpointHash, err := checkpoint.Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)
	assert.Equal(t, rlpBlock, proposal.Data)
	assert.Equal(t, checkpointHash.Bytes(), proposal.Hash)

	mBlockBuilder.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockChainMock.AssertExpectations(t)
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_FailedToCommitStateTx(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		committedCount    = 4
		parentCount       = 3
		parentBlockNumber = 1023
	)

	validators := newTestValidators(accountCount)
	extra := createTestExtra(validators.getPublicIdentities(), AccountSet{}, accountCount-1, committedCount, parentCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}

	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("WriteTx", mock.Anything).Return(errors.New("error")).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		isEndOfEpoch:      true,
		validators:        newValidatorSet(types.Address{}, validators.getPublicIdentities()),
		uptimeCounter:     createTestUptimeCounter(t, nil, 10),
		checkpointBackend: new(checkpointBackendMock),
		roundInfo:         &pbft.RoundInfo{},
	}

	_, err := fsm.BuildProposal()
	assert.ErrorContains(t, err, "failed to commit validators uptime transaction")
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

	validatorSet := newTestValidators(validatorsCount).getPublicIdentities()
	extra := createTestExtra(validatorSet, AccountSet{}, validatorsCount-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	transition := &state.Transition{}
	blockBuilderMock := newBlockBuilderMock(stateBlock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Once()
	blockBuilderMock.On("GetState").Return(transition).Once()

	newValidators := validatorSet[:remainingValidatorsCount].Copy()
	addedValidators := newTestValidators(2).getPublicIdentities()
	newValidators = append(newValidators, addedValidators...)
	systemStateMock := new(systemStateMock)
	systemStateMock.On("GetValidatorSet").Return(newValidators).Once()

	blockChainMock := new(blockchainMock)
	blockChainMock.On("GetStateProvider", mock.Anything).
		Return(NewStateProvider(transition)).Once()
	blockChainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock).Once()

	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{
		parent:            parent,
		blockBuilder:      blockBuilderMock,
		config:            &PolyBFTConfig{},
		backend:           blockChainMock,
		isEndOfEpoch:      true,
		validators:        newValidatorSet(types.Address{}, validatorSet),
		uptimeCounter:     createTestUptimeCounter(t, validatorSet, 10),
		checkpointBackend: checkpointBackendMock,
		logger:            hclog.NewNullLogger(),
		roundInfo:         &pbft.RoundInfo{},
	}

	proposal, err := fsm.BuildProposal()
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
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_NonEpochEndingBlock_ValidatorsDeltaEmpty(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 9
	)

	testValidators := newTestValidators(accountCount)
	extra := createTestExtra(testValidators.getPublicIdentities(), AccountSet{}, accountCount-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, extra)

	blockBuilderMock := &blockBuilderMock{}
	blockBuilderMock.On("Build", mock.Anything).Return(stateBlock).Once()
	blockBuilderMock.On("Fill").Once()
	blockBuilderMock.On("Receipts", mock.Anything).Return([]*types.Receipt{}).Once()

	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	systemStateMock := new(systemStateMock)

	fsm := &fsm{parent: parent, blockBuilder: blockBuilderMock,
		config: &PolyBFTConfig{}, backend: &blockchainMock{},
		isEndOfEpoch: false, validators: testValidators.toValidatorSet(),
		checkpointBackend: checkpointBackendMock, logger: hclog.NewNullLogger(),
		roundInfo: &pbft.RoundInfo{}}

	proposal, err := fsm.BuildProposal()
	assert.NoError(t, err)
	assert.NotNil(t, proposal)

	blockExtra, err := GetIbftExtra(stateBlock.Block.Header.ExtraData)
	assert.NoError(t, err)
	assert.True(t, blockExtra.Validators.IsEmpty())

	blockBuilderMock.AssertExpectations(t)
	systemStateMock.AssertNotCalled(t, "GetValidatorSet")
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_BuildProposal_EpochEndingBlock_FailToCreateValidatorsDelta(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 6
		signaturesCount   = 4
		parentBlockNumber = 49
	)

	testValidators := newTestValidators(accountCount)
	allAccounts := testValidators.getPublicIdentities()
	extra := createTestExtra(allAccounts, AccountSet{}, accountCount-1, signaturesCount, signaturesCount)

	parent := &types.Header{Number: parentBlockNumber, ExtraData: extra}

	transition := &state.Transition{}
	blockBuilderMock := new(blockBuilderMock)
	blockBuilderMock.On("WriteTx", mock.Anything).Return(error(nil)).Once()
	blockBuilderMock.On("GetState").Return(transition).Once()

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
		uptimeCounter:     createTestUptimeCounter(t, allAccounts, 10),
		checkpointBackend: new(checkpointBackendMock),
		roundInfo:         &pbft.RoundInfo{},
	}

	proposal, err := fsm.BuildProposal()
	assert.ErrorContains(t, err, fmt.Sprintf("failed to retrieve validator set for block#%d: failed to get validators set", parentBlockNumber+1))
	assert.Nil(t, proposal)

	blockBuilderMock.AssertNotCalled(t, "Build")
	blockBuilderMock.AssertNotCalled(t, "Fill")
	blockBuilderMock.AssertExpectations(t)
	systemStateMock.AssertExpectations(t)
	blockChainMock.AssertExpectations(t)
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, uptimeCounter: createTestUptimeCounter(t, nil, 10)}
	tx, err := fsm.createValidatorsUptimeTx()
	assert.NoError(t, err)
	err = fsm.VerifyStateTransactions([]*types.Transaction{tx})
	assert.ErrorContains(t, err, "state transaction in block which should not contain it")
}

func TestFSM_VerifyStateTransactions_MiddleOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, uptimeCounter: createTestUptimeCounter(t, nil, 10)}
	err := fsm.VerifyStateTransactions([]*types.Transaction{})
	assert.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_EndOfEpochWithoutTransaction(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, isEndOfEpoch: true, uptimeCounter: createTestUptimeCounter(t, nil, 10)}
	err := fsm.VerifyStateTransactions([]*types.Transaction{})
	assert.EqualError(t, err, "uptime transaction is not found in the epoch ending block")
}

func TestFSM_VerifyStateTransactions_EndOfEpochWrongValidatorsUptimeTx(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, isEndOfEpoch: true, uptimeCounter: createTestUptimeCounter(t, nil, 10)}
	uptimeCounter, err := createTestUptimeCounter(t, nil, 5).EncodeAbi()
	require.NoError(t, err)

	commitEpochTx := createStateTransactionWithData(contracts.ValidatorSetContract, uptimeCounter)
	err = fsm.VerifyStateTransactions([]*types.Transaction{commitEpochTx})
	assert.ErrorContains(t, err, "invalid uptime transaction")
}

func TestFSM_VerifyStateTransactions_StateTransactionAndSprintIsFalse(t *testing.T) {
	t.Parallel()

	fsm := &fsm{config: &PolyBFTConfig{}, uptimeCounter: createTestUptimeCounter(t, nil, 10)}
	dummyStateTx := &types.Transaction{To: &contracts.StateReceiverContract, Type: types.StateTx}
	err := fsm.VerifyStateTransactions([]*types.Transaction{dummyStateTx})
	assert.ErrorContains(t, err, "state transaction in block which should not contain")
}

func TestFSM_VerifyStateTransactions_StateTransactionPass(t *testing.T) {
	t.Parallel()

	validators := newTestValidators(5)
	fsm := &fsm{
		config:        &PolyBFTConfig{},
		isEndOfEpoch:  true,
		isEndOfSprint: true,
		validators:    newValidatorSet(types.Address{}, validators.getPublicIdentities()),
		uptimeCounter: createTestUptimeCounter(t, nil, 10),
		logger:        hclog.NewNullLogger(),
	}
	txs := fsm.stateTransactions()

	// add validators uptime tx to the end of transactions list
	tx, err := fsm.createValidatorsUptimeTx()
	assert.NoError(t, err)

	txs = append([]*types.Transaction{tx}, txs...)

	err = fsm.VerifyStateTransactions(txs)
	assert.NoError(t, err)
}

func TestFSM_VerifyStateTransactions_StateTransactionQuorumNotReached(t *testing.T) {
	t.Parallel()

	validators := newTestValidators(5)
	commitment := createTestCommitment(t, validators.getPrivateIdentities())
	commitment.AggSignature = Signature{
		AggregatedSignature: []byte{1, 2},
		Bitmap:              []byte{},
	}

	fsm := &fsm{
		config:                       &PolyBFTConfig{},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		validators:                   newValidatorSet(types.Address{}, validators.getPublicIdentities()),
		proposerCommitmentToRegister: commitment,
		uptimeCounter:                createTestUptimeCounter(t, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	txs := fsm.stateTransactions()

	// add validators uptime tx to the end of transactions list
	tx, err := fsm.createValidatorsUptimeTx()
	assert.NoError(t, err)

	txs = append([]*types.Transaction{tx}, txs...)

	err = fsm.VerifyStateTransactions(txs)
	assert.ErrorContains(t, err, "quorum size not reached")
}

func TestFSM_VerifyStateTransactions_StateTransactionInvalidSignature(t *testing.T) {
	t.Parallel()

	validators := newTestValidators(5)
	commitment := createTestCommitment(t, validators.getPrivateIdentities())
	nonValidators := newTestValidators(3)
	aggregatedSigs := bls.Signatures{}

	nonValidators.iterAcct(nil, func(t *testValidator) {
		aggregatedSigs = append(aggregatedSigs, t.mustSign([]byte("dummyHash")))
	})

	sig, err := aggregatedSigs.Aggregate().Marshal()
	require.NoError(t, err)

	commitment.AggSignature.AggregatedSignature = sig

	fsm := &fsm{
		config:                       &PolyBFTConfig{},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		validators:                   newValidatorSet(types.Address{}, validators.getPublicIdentities()),
		proposerCommitmentToRegister: commitment,
		uptimeCounter:                createTestUptimeCounter(t, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}

	txs := fsm.stateTransactions()

	// add validators uptime tx to the end of transactions list
	tx, err := fsm.createValidatorsUptimeTx()
	assert.NoError(t, err)

	txs = append([]*types.Transaction{tx}, txs...)

	err = fsm.VerifyStateTransactions(txs)
	assert.ErrorContains(t, err, "invalid signature")
}

func TestFSM_ValidateCommit_ProposalIsNil(t *testing.T) {
	t.Parallel()

	fsm := &fsm{}
	err := fsm.ValidateCommit("", []byte{})
	assert.ErrorContains(t, err, "proposal unavailable")

	fsm.proposal = &pbft.Proposal{}
	err = fsm.ValidateCommit("", []byte{})
	assert.ErrorContains(t, err, "proposal unavailable")
}

func TestFSM_ValidateCommit_WrongValidator(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := newTestValidators(accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)
	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), logger: hclog.NewNullLogger(), checkpointBackend: checkpointBackendMock,
		roundInfo: &pbft.RoundInfo{}}

	_, err := fsm.BuildProposal()
	require.NoError(t, err)

	err = fsm.ValidateCommit("0x7467674", types.ZeroAddress.Bytes())
	require.ErrorContains(t, err, "unable to resolve validator")
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_ValidateCommit_InvalidHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
	)

	validators := newTestValidators(accountsCount)

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 5, 3, 3),
	}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)
	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), checkpointBackend: checkpointBackendMock, logger: hclog.NewNullLogger(),
		roundInfo: &pbft.RoundInfo{}}

	_, err := fsm.BuildProposal()
	assert.NoError(t, err)

	nonValidatorAcc := newTestValidator("non_validator", 1)
	wrongSignature, err := nonValidatorAcc.mustSign([]byte("Foo")).Marshal()
	require.NoError(t, err)

	err = fsm.ValidateCommit(validators.getValidator("0").Address().String(), wrongSignature)
	require.ErrorContains(t, err, "incorrect commit signature from")
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_ValidateCommit_Good(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	validatorSet := validators.getPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber, ExtraData: createTestExtra(validatorSet, AccountSet{}, 5, 3, 3)}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)
	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: newValidatorSet(types.Address{}, validatorSet), checkpointBackend: checkpointBackendMock, logger: hclog.NewNullLogger(),
		roundInfo: &pbft.RoundInfo{}}
	_, err := fsm.BuildProposal()
	require.NoError(t, err)

	validator := validators.getValidator("A")
	seal, err := validator.mustSign(fsm.proposal.Hash).Marshal()
	require.NoError(t, err)
	err = fsm.ValidateCommit(validator.Key().NodeID(), seal)
	require.NoError(t, err)

	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_Validate_IncorrectSignHash(t *testing.T) {
	t.Parallel()

	const (
		parentBlockNumber = 10
		accountsCount     = 5
		signaturesCount   = 3
	)

	validators := newTestValidators(accountsCount)

	parent := &types.Header{Number: parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, accountsCount, signaturesCount, signaturesCount)}
	parent.ComputeHash()
	stateBlock := createDummyStateBlock(parentBlockNumber+1, parent.Hash, parent.ExtraData)
	mBlockBuilder := newBlockBuilderMock(stateBlock)
	checkpointBackendMock := new(checkpointBackendMock)
	checkpointBackendMock.On("BuildEventRoot", mock.Anything, mock.Anything).Return(types.ZeroHash, nil).Once()

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), checkpointBackend: checkpointBackendMock, logger: hclog.NewNullLogger(),
		roundInfo: &pbft.RoundInfo{},
	}
	proposal, err := fsm.BuildProposal()
	require.NoError(t, err)

	proposal.Hash = []byte{32, 33} // make the wrong hash

	err = fsm.Validate(proposal)
	require.ErrorContains(t, err, "incorrect sign hash")
	checkpointBackendMock.AssertExpectations(t)
}

func TestFSM_Validate_IncorrectHeaderParentHash(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 25
		signaturesCount   = 3
	)

	validators := newTestValidators(accountsCount)
	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validators.getPublicIdentities(), AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	fsm := &fsm{parent: parent, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: validators.toValidatorSet(), logger: hclog.NewNullLogger()}

	stateBlock := createDummyStateBlock(parent.Number+1, types.Hash{100, 15}, parent.ExtraData)

	checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), stateBlock.Block.Number(), stateBlock.Block.Hash())
	require.NoError(t, err)

	proposal := &pbft.Proposal{
		Data: stateBlock.Block.MarshalRLP(),
		Hash: checkpointHash.Bytes(),
	}

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

	validators := newTestValidators(accountsCount)
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

		proposal := &pbft.Proposal{
			Data: stateBlock.Block.MarshalRLP(),
			Hash: proposalHash.Bytes(),
		}

		err = fsm.Validate(proposal)
		require.ErrorContains(t, err, "invalid number")
	}
}

func TestFSM_Validate_TimestampOlder(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := newTestValidators(5)
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
		stateBlock := &StateBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{Header: header})}
		fsm := &fsm{parent: parent, config: &PolyBFTConfig{}, backend: &blockchainMock{},
			validators: validators.toValidatorSet(), logger: hclog.NewNullLogger()}

		checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), header.Number, header.Hash)
		require.NoError(t, err)

		proposal := &pbft.Proposal{
			Data: stateBlock.Block.MarshalRLP(),
			Hash: checkpointHash.Bytes(),
		}

		err = fsm.Validate(proposal)
		assert.ErrorContains(t, err, "timestamp older than parent")
	}
}

func TestFSM_Validate_IncorrectMixHash(t *testing.T) {
	t.Parallel()

	const parentBlockNumber = 10

	validators := newTestValidators(5)
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

	buildBlock := &StateBlock{Block: consensus.BuildBlock(consensus.BuildBlockParams{Header: header})}

	fsm := &fsm{
		parent:     parent,
		config:     &PolyBFTConfig{},
		backend:    &blockchainMock{},
		validators: validators.toValidatorSet(),
		logger:     hclog.NewNullLogger(),
	}
	rlpBlock := buildBlock.Block.MarshalRLP()

	checkpointHash, err := new(CheckpointData).Hash(fsm.backend.GetChainID(), header.Number, header.Hash)
	require.NoError(t, err)

	proposal := &pbft.Proposal{
		Data: rlpBlock,
		Hash: checkpointHash.Bytes(),
	}

	err = fsm.Validate(proposal)
	assert.ErrorContains(t, err, "mix digest is not correct")
}

func TestFSM_Insert_Good(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := newTestValidators(accountCount)
	allAccounts := validators.getPrivateIdentities()
	validatorSet := validators.getPublicIdentities()

	extraParent := createTestExtra(validatorSet, AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
	parent := &types.Header{Number: parentBlockNumber, ExtraData: extraParent}
	extraBlock := createTestExtra(validatorSet, AccountSet{}, len(allAccounts)-1, signaturesCount, signaturesCount)
	finalBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
	})

	buildBlock := &StateBlock{Block: finalBlock}
	mBlockBuilder := newBlockBuilderMock(buildBlock)
	mBackendMock := &blockchainMock{}
	mBackendMock.On("CommitBlock", mock.MatchedBy(func(i interface{}) bool {
		stateBlock, ok := i.(*StateBlock)
		require.True(t, ok)

		return stateBlock == buildBlock
	})).Return(error(nil)).Once()

	fsm := &fsm{parent: parent,
		blockBuilder: mBlockBuilder,
		config:       &PolyBFTConfig{},
		backend:      mBackendMock,
		validators:   newValidatorSet(types.Address{}, validatorSet[0:len(validatorSet)-1]),
		block:        buildBlock,
	}
	postInsertHookInvoked := false
	// postInsertHook should be called at the end of Insert
	fsm.postInsertHook = func() error {
		assert.Equal(t, buildBlock, fsm.block)

		postInsertHookInvoked = true

		return nil
	}

	rlpBlock := buildBlock.Block.MarshalRLP()
	proposal := &pbft.SealedProposal{
		Proposal: &pbft.Proposal{
			Data: rlpBlock,
			Hash: buildBlock.Block.Hash().Bytes(),
		},
		Proposer: pbft.NodeID(validatorSet[0].Address.String()),
		Number:   parentBlockNumber + 1,
	}

	for i := 0; i < signaturesCount; i++ {
		sign, err := allAccounts[i].Bls.Sign(buildBlock.Block.Hash().Bytes())
		assert.NoError(t, err)
		sigRaw, err := sign.Marshal()
		assert.NoError(t, err)

		proposal.CommittedSeals = append(proposal.CommittedSeals, pbft.CommittedSeal{
			NodeID:    pbft.NodeID(validatorSet[i].Address.String()),
			Signature: sigRaw,
		})
	}

	err := fsm.Insert(proposal)
	assert.NoError(t, err)
	assert.True(t, postInsertHookInvoked)
	mBackendMock.AssertExpectations(t)
}

func TestFSM_Insert_InvalidNode(t *testing.T) {
	t.Parallel()

	const (
		accountCount      = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	validatorSet := validators.getPublicIdentities()

	parent := &types.Header{Number: parentBlockNumber}
	parent.ComputeHash()

	extraBlock := createTestExtra(validatorSet, AccountSet{}, len(validators.validators)-1, signaturesCount, signaturesCount)
	finalBlock := consensus.BuildBlock(
		consensus.BuildBlockParams{
			Header: &types.Header{Number: parentBlockNumber + 1, ParentHash: parent.Hash, ExtraData: extraBlock},
		})

	buildBlock := &StateBlock{Block: finalBlock}
	mBlockBuilder := newBlockBuilderMock(buildBlock)

	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
		validators: newValidatorSet(types.Address{}, validatorSet[0:len(validatorSet)-1]),
		block:      buildBlock,
	}
	postInsertHookInvoked := false
	// postInsertHook should be called at the end of Insert.
	// In this test case it is not expected to be called
	fsm.postInsertHook = func() error {
		postInsertHookInvoked = true

		return nil
	}

	rlpBlock := buildBlock.Block.MarshalRLP()
	validatorA := validators.getValidator("A")
	validatorB := validators.getValidator("B")
	proposalHash := buildBlock.Block.Hash().Bytes()
	sigA, err := validatorA.mustSign(proposalHash).Marshal()
	require.NoError(t, err)

	sigB, err := validatorB.mustSign(proposalHash).Marshal()
	require.NoError(t, err)

	// create test account outside of validator set
	nonValidatorAccount := newTestValidator("non_validator", 1)
	nonValidatorSignature, err := nonValidatorAccount.mustSign(proposalHash).Marshal()
	require.NoError(t, err)

	proposal := &pbft.SealedProposal{
		Proposal: &pbft.Proposal{
			Data: rlpBlock,
			Hash: proposalHash,
		},
		CommittedSeals: []pbft.CommittedSeal{
			{NodeID: pbft.NodeID(validatorA.Address().String()), Signature: sigA},
			{NodeID: pbft.NodeID(validatorB.Address().String()), Signature: sigB},
			{NodeID: pbft.NodeID(nonValidatorAccount.Address().String()), Signature: nonValidatorSignature}, // this one should fail
		},
		Proposer: pbft.NodeID(validatorA.Address().String()),
		Number:   parentBlockNumber + 1,
	}

	err = fsm.Insert(proposal)
	assert.ErrorContains(t, err, "invalid node id")
	assert.False(t, postInsertHookInvoked)
}

func TestFSM_Height(t *testing.T) {
	t.Parallel()

	parentNumber := uint64(3)
	parent := &types.Header{Number: parentNumber}
	fsm := &fsm{parent: parent}
	assert.Equal(t, parentNumber+1, fsm.Height())
}

func TestFSM_StateTransactionsEndOfSprint(t *testing.T) {
	t.Parallel()

	const (
		commitmentsCount = 8
		from             = 15
		bundleSize       = 5 // 8 bundles per commitment, 5 sse per bundle
		eventsSize       = 40
	)

	var bundleProofs []*BundleProof

	var commitments [commitmentsCount]*CommitmentMessage

	for i := 0; i < commitmentsCount; i++ {
		commitment, commitmentMessage, sse := buildCommitmentAndStateSyncs(t, eventsSize, uint64(3), bundleSize, eventsSize*uint64(i))
		commitments[i] = commitmentMessage

		for j := uint64(0); j < commitmentMessage.BundlesCount(); j++ {
			until := (j + 1) * bundleSize
			if until > uint64(len(sse)) {
				until = uint64(len(sse))
			}

			proof := commitment.MerkleTree.GenerateProof(j, 0)

			bundleProofs = append(bundleProofs, &BundleProof{
				Proof:      proof,
				StateSyncs: sse[j*bundleSize : until],
			})
		}
	}

	signedCommitment := &CommitmentMessageSigned{
		Message: commitments[0],
		AggSignature: Signature{
			AggregatedSignature: []byte{1, 2},
			Bitmap:              []byte{1},
		},
		PublicKeys: [][]byte{},
	}
	f := &fsm{
		config:                       &PolyBFTConfig{},
		isEndOfEpoch:                 true,
		isEndOfSprint:                true,
		bundleProofs:                 bundleProofs[0 : len(bundleProofs)-1],
		proposerCommitmentToRegister: signedCommitment,
		uptimeCounter:                createTestUptimeCounter(t, nil, 10),
		logger:                       hclog.NewNullLogger(),
	}
	txs := f.stateTransactions()

	for i, tx := range txs {
		decodedData, err := decodeStateTransaction(tx.Input)
		require.NoError(t, err)

		switch stateTxData := decodedData.(type) {
		case *CommitmentMessageSigned:
			require.Equal(t, 0, i, "failed for tx number %d", i)
			require.Equal(t, signedCommitment, stateTxData, "failed for tx number %d", i)
		case *BundleProof:
			require.NotEqual(t, 0, i, "failed for tx number %d", i)

			for _, cm := range commitments {
				if cm.ContainsStateSync(stateTxData.StateSyncs[0].ID) {
					bundleIndx := cm.GetBundleIdxFromStateSyncEventIdx(stateTxData.StateSyncs[0].ID)
					require.Equal(t, uint64((i-1)%(eventsSize/bundleSize)), bundleIndx, "failed for tx number %d", i)
				}
			}
		}
	}
}

func TestFSM_CalcProposer(t *testing.T) {
	t.Parallel()

	const validatorsCount = 10
	validatorSet := newTestValidators(validatorsCount).getPublicIdentities()

	t.Run("Undefined last proposer", func(t *testing.T) {
		t.Parallel()

		cases := []struct{ round, expectedIndex uint64 }{
			{0, 0},
			{2, 2},
			{15, 5},
		}
		for _, c := range cases {
			f := &fsm{validators: newValidatorSet(types.Address{}, validatorSet.Copy())}
			proposer := f.validators.CalcProposer(c.round)
			assert.Equal(t, pbft.NodeID(validatorSet[c.expectedIndex].Address.String()), proposer)
		}
	})

	t.Run("Seed last proposer", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			lastProposer  types.Address
			round         uint64
			expectedIndex uint64
		}{
			{validatorSet[0].Address, 0, 1},
			{validatorSet[5].Address, 3, 9},
			{validatorSet[6].Address, 15, 2},
		}
		for _, c := range cases {
			f := &fsm{validators: newValidatorSet(c.lastProposer, validatorSet.Copy())}
			proposer := f.validators.CalcProposer(c.round)
			assert.Equal(t, pbft.NodeID(validatorSet[c.expectedIndex].Address.String()), proposer)
		}
	})
}

func TestFSM_VerifyStateTransaction_NotEndOfSprint(t *testing.T) {
	t.Parallel()

	cm, _, sse := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 2)
	f := &fsm{
		isEndOfSprint: false,
		config:        &PolyBFTConfig{},
	}

	proof := cm.MerkleTree.GenerateProof(0, 0)

	bf := &BundleProof{
		Proof:      proof,
		StateSyncs: sse[0:1],
	}
	inputData, err := bf.EncodeAbi()
	require.NoError(t, err)

	txns := []*types.Transaction{createStateTransactionWithData(f.config.StateReceiverAddr, inputData)}
	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "state transaction in block which should not contain it")
}

func TestFSM_VerifyStateTransaction_ValidBothTypesOfStateTransactions(t *testing.T) {
	t.Parallel()

	var (
		commitmentMessages [2]*CommitmentMessage
		commitments        [2]*Commitment
		stateSyncs         [2][]*StateSyncEvent
		signedCommitments  [2]*CommitmentMessageSigned
	)

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	commitments[0], commitmentMessages[0], stateSyncs[0] = buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 2)
	commitments[1], commitmentMessages[1], stateSyncs[1] = buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 12)

	for i, x := range commitmentMessages {
		// add register commitment state transaction
		hash, err := x.Hash()
		require.NoError(t, err)
		signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C"), hash)
		signedCommitments[i] = &CommitmentMessageSigned{
			Message:      x,
			AggSignature: *signature,
		}
	}

	f := &fsm{
		isEndOfSprint:              true,
		config:                     &PolyBFTConfig{},
		validators:                 validators.toValidatorSet(),
		commitmentsToVerifyBundles: signedCommitments[:],
		stateSyncExecutionIndex:    commitmentMessages[0].FromIndex,
	}

	var txns []*types.Transaction

	for i, x := range commitmentMessages {
		inputData, err := signedCommitments[i].EncodeAbi()
		require.NoError(t, err)

		if i == 0 {
			tx := createStateTransactionWithData(f.config.StateReceiverAddr, inputData)
			txns = append(txns, tx)
		}

		// add execute bundle state transactions
		end := x.BundlesCount()
		if i == 1 {
			end -= 2
		}

		for idx := uint64(0); idx < end; idx++ {
			proof := commitments[i].MerkleTree.GenerateProof(idx, 0)
			bf := &BundleProof{
				Proof:      proof,
				StateSyncs: stateSyncs[i][idx : idx+1],
			}
			inputData, err := bf.EncodeAbi()
			require.NoError(t, err)

			txns = append(txns,
				createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
		}
	}

	err := f.VerifyStateTransactions(txns)
	require.NoError(t, err)
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

	err := f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "state transaction error while decoding")
}

func TestFSM_VerifyStateTransaction_QuorumNotReached(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessage, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 2)
	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
		validators:    validators.toValidatorSet(),
	}

	hash, err := commitmentMessage.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.getPrivateIdentities("A", "B"), hash)
	cmSigned := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: *signature,
	}
	inputData, err := cmSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))

	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "quorum size not reached for state tx")
}

func TestFSM_VerifyStateTransaction_InvalidSignature(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessage, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 2)
	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
		validators:    validators.toValidatorSet(),
	}

	hash, err := commitmentMessage.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D"), hash)
	invalidValidator := newTestValidator("G", 1)
	invalidSignature, err := invalidValidator.mustSign([]byte("malicious message")).Marshal()
	require.NoError(t, err)

	cmSigned := &CommitmentMessageSigned{
		Message: commitmentMessage,
		AggSignature: Signature{
			Bitmap:              signature.Bitmap,
			AggregatedSignature: invalidSignature,
		},
	}

	inputData, err := cmSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))

	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "invalid signature for tx")
}

func TestFSM_VerifyStateTransaction_BundlesNotInSequentialOrder(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	commitment, commitmentMessage, stateSyncs := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(2), 2)

	hash, err := commitmentMessage.Hash()
	require.NoError(t, err)
	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D", "E", "F"), hash)
	cmSigned := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: *signature,
	}
	f := &fsm{
		isEndOfSprint:              true,
		config:                     &PolyBFTConfig{},
		commitmentsToVerifyBundles: []*CommitmentMessageSigned{cmSigned},
		stateSyncExecutionIndex:    cmSigned.Message.FromIndex,
	}

	stateSyncIndices := []int{0, 3}
	txns := make([]*types.Transaction, len(stateSyncIndices))

	for i, x := range stateSyncIndices {
		proof := commitment.MerkleTree.GenerateProof(uint64(x), 0)
		bf := &BundleProof{
			Proof:      proof,
			StateSyncs: stateSyncs[x*2 : x*2+2],
		}
		inputData, err := bf.EncodeAbi()
		require.NoError(t, err)

		txns[i] = createStateTransactionWithData(f.config.StateReceiverAddr, inputData)
	}

	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "bundles to execute are not in sequential order")
}

func TestFSM_VerifyStateTransaction_TwoCommitmentMessages(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	_, commitmentMessage, _ := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 2)
	f := &fsm{
		isEndOfSprint: true,
		config:        &PolyBFTConfig{},
		validators:    newValidatorSet(types.Address{}, validators.getPublicIdentities()),
	}

	hash, err := commitmentMessage.Hash()
	require.NoError(t, err)

	var txns []*types.Transaction

	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D"), hash)
	cmSigned := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: *signature,
	}
	inputData, err := cmSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
	inputData, err = cmSigned.EncodeAbi()
	require.NoError(t, err)

	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "only one commitment is allowed per block")
}

func TestFSM_VerifyStateTransaction_ProofError(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	commitment, commitmentMessage, stateSyncs := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(2), 2)

	proof := commitment.MerkleTree.GenerateProof(0, 0)
	bf := &BundleProof{
		Proof:      proof,
		StateSyncs: stateSyncs[:2],
	}
	inputData, err := bf.EncodeAbi()
	require.NoError(t, err)

	hash, err := commitmentMessage.Hash()
	require.NoError(t, err)
	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D", "E"), hash)
	commitmentMessage.MerkleRootHash[0] = (commitmentMessage.MerkleRootHash[0] + 1) % 255 // change merkle root hash
	cmSigned := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: *signature,
	}
	f := &fsm{
		isEndOfSprint:              true,
		config:                     &PolyBFTConfig{},
		commitmentsToVerifyBundles: []*CommitmentMessageSigned{cmSigned},
		stateSyncExecutionIndex:    cmSigned.Message.FromIndex,
	}

	var txns []*types.Transaction
	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "error while validating proof")
}

func TestFSM_VerifyStateTransaction_CommitmentDoesNotExist(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	commitment, commitmentMessage, stateSyncs := buildCommitmentAndStateSyncs(t, 10, uint64(3), uint64(1), 2)
	hash, err := commitmentMessage.Hash()
	require.NoError(t, err)
	signature := createSignature(t, validators.getPrivateIdentities("A", "B", "C", "D", "E"), hash)
	commitmentMessage.MerkleRootHash[0] = (commitmentMessage.MerkleRootHash[0] + 1) % 255 // change merkle root hash
	cmSigned := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: *signature,
	}

	f := &fsm{
		isEndOfSprint:              true,
		config:                     &PolyBFTConfig{},
		commitmentsToVerifyBundles: []*CommitmentMessageSigned{cmSigned},
		stateSyncExecutionIndex:    cmSigned.Message.FromIndex,
	}

	// bundle proof will not belong to any passed commitment to fsm
	cmSigned.Message.ToIndex += 1000
	cmSigned.Message.FromIndex += 1000

	proof := commitment.MerkleTree.GenerateProof(0, 0)
	bf := &BundleProof{
		Proof:      proof,
		StateSyncs: stateSyncs[:1],
	}

	inputData, err := bf.EncodeAbi()
	require.NoError(t, err)

	var txns []*types.Transaction
	txns = append(txns,
		createStateTransactionWithData(f.config.StateReceiverAddr, inputData))
	err = f.VerifyStateTransactions(txns)
	require.ErrorContains(t, err, "No appropriate commitment found to verify proof")
}

func TestFSM_Validate_FailToVerifySignatures(t *testing.T) {
	t.Parallel()

	const (
		accountsCount     = 5
		parentBlockNumber = 10
		signaturesCount   = 3
	)

	validators := newTestValidators(accountsCount)
	validatorSet := validators.getPublicIdentities()

	parent := &types.Header{
		Number:    parentBlockNumber,
		ExtraData: createTestExtra(validatorSet, AccountSet{}, 4, signaturesCount, signaturesCount),
	}
	parent.ComputeHash()

	polybftBackendMock := new(polybftBackendMock)
	polybftBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorSet, nil).Once()

	fsm := &fsm{
		parent:                       parent,
		config:                       &PolyBFTConfig{Bridge: &BridgeConfig{}},
		backend:                      &blockchainMock{},
		polybftBackend:               polybftBackendMock,
		validators:                   newValidatorSet(types.Address{}, validatorSet),
		proposerCommitmentToRegister: createTestCommitment(t, validators.getPrivateIdentities()),
		isEndOfEpoch:                 true,
		uptimeCounter:                createTestUptimeCounter(t, validatorSet, 10),
		logger:                       hclog.NewNullLogger(),
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

	proposal := &pbft.Proposal{
		Data: finalBlock.MarshalRLP(),
		Hash: checkpointHash.Bytes(),
	}

	assert.ErrorContains(t, fsm.Validate(proposal), "failed to verify signatures")

	polybftBackendMock.AssertExpectations(t)
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

func createDummyStateBlock(blockNumber uint64, parentHash types.Hash, extraData []byte) *StateBlock {
	finalBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header: &types.Header{
			Number:     blockNumber,
			ParentHash: parentHash,
			Difficulty: 1,
			ExtraData:  extraData,
		},
	})

	return &StateBlock{Block: finalBlock}
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

func createTestCommitment(t *testing.T, accounts []*wallet.Account) *CommitmentMessageSigned {
	t.Helper()

	bitmap := bitmap.Bitmap{}
	stateSyncEvents := make([]*StateSyncEvent, len(accounts))

	for i := 0; i < len(accounts); i++ {
		stateSyncEvents[i] = newStateSyncEvent(
			uint64(i),
			accounts[i].Ecdsa.Address(),
			accounts[0].Ecdsa.Address(),
			[]byte{},
		)

		bitmap.Set(uint64(i))
	}

	stateSyncsTrie, err := createMerkleTree(stateSyncEvents, stateSyncBundleSize)
	require.NoError(t, err)

	commitment := NewCommitmentMessage(stateSyncsTrie.Hash(), 0, uint64(len(stateSyncEvents)), stateSyncBundleSize)
	hash, err := commitment.Hash()
	require.NoError(t, err)

	var signatures bls.Signatures

	for _, a := range accounts {
		signature, err := a.Bls.Sign(hash.Bytes())
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
		Message:      commitment,
		AggSignature: signature,
	}
}

func newBlockBuilderMock(stateBlock *StateBlock) *blockBuilderMock {
	mBlockBuilder := new(blockBuilderMock)
	mBlockBuilder.On("Build", mock.Anything).Return(stateBlock).Once()
	mBlockBuilder.On("Fill", mock.Anything).Once()
	mBlockBuilder.On("Receipts", mock.Anything).Return([]*types.Receipt{}).Once()

	return mBlockBuilder
}

func createTestUptimeCounter(t *testing.T, validatorSet AccountSet, epochSize uint64) *CommitEpoch {
	t.Helper()

	if validatorSet == nil {
		validatorSet = newTestValidators(5).getPublicIdentities()
	}

	uptime := Uptime{EpochID: 0}
	commitEpoch := &CommitEpoch{
		EpochID: 0,
		Epoch: Epoch{
			StartBlock: 1,
			EndBlock:   1 + epochSize,
			EpochRoot:  types.Hash{},
		},
		Uptime: uptime,
	}
	indexToStart := 0

	for i := uint64(0); i < epochSize; i++ {
		validatorIndex := indexToStart
		for j := 0; j < validatorSet.Len()-1; j++ {
			validatorIndex = validatorIndex % validatorSet.Len()
			uptime.addValidatorUptime(validatorSet[validatorIndex].Address, 1)
			validatorIndex++
		}

		indexToStart = (indexToStart + 1) % validatorSet.Len()
	}

	return commitEpoch
}
*/
