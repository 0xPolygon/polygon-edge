package polybft

import (
	"errors"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// the test initializes polybft and chain mock (map of headers) after which a new header is verified
// firstly, two invalid situation of header verifications are triggered (missing Committed field and invalid validators for ParentCommitted)
// afterwards, valid inclusion into the block chain is checked
// and at the end there is a situation when header is already a part of blockchain
func TestPolybft_VerifyHeader(t *testing.T) {
	t.Parallel()

	const (
		allValidatorsSize = 6 // overall there are 6 validators
		validatorSetSize  = 5 // only 5 validators are active at the time
		fixedEpochSize    = uint64(10)
	)

	updateHeaderExtra := func(header *types.Header,
		validators *ValidatorSetDelta,
		parentSignature *Signature,
		checkpointData *CheckpointData,
		committedAccounts []*wallet.Account) *Signature {
		extra := &Extra{
			Validators: validators,
			Parent:     parentSignature,
			Checkpoint: checkpointData,
			Committed:  &Signature{},
		}

		if extra.Checkpoint == nil {
			extra.Checkpoint = &CheckpointData{}
		}

		header.ExtraData = extra.MarshalRLPTo(nil)
		header.ComputeHash()

		if len(committedAccounts) > 0 {
			checkpointHash, err := extra.Checkpoint.Hash(0, header.Number, header.Hash)
			require.NoError(t, err)

			extra.Committed = createSignature(t, committedAccounts, checkpointHash, bls.DomainCheckpointManager)
			header.ExtraData = extra.MarshalRLPTo(nil)
		}

		return extra.Committed
	}

	// create all validators
	validators := newTestValidators(t, allValidatorsSize)

	// create configuration
	polyBftConfig := PolyBFTConfig{
		InitialValidatorSet: validators.getParamValidators(),
		EpochSize:           fixedEpochSize,
		SprintSize:          5,
	}

	validatorSet := validators.getPublicIdentities()
	accounts := validators.getPrivateIdentities()

	// calculate validators before and after the end of the first epoch
	validatorSetParent, validatorSetCurrent := validatorSet[:len(validatorSet)-1], validatorSet[1:]
	accountSetParent, accountSetCurrent := accounts[:len(accounts)-1], accounts[1:]

	// create header map to simulate blockchain
	headersMap := &testHeadersMap{}

	// create genesis header
	genesisDelta, err := createValidatorSetDelta(nil, validatorSetParent)
	require.NoError(t, err)

	genesisHeader := &types.Header{Number: 0}
	updateHeaderExtra(genesisHeader, genesisDelta, nil, nil, nil)

	// add genesis header to map
	headersMap.addHeader(genesisHeader)

	// create headers from 1 to 9
	for i := uint64(1); i < polyBftConfig.EpochSize; i++ {
		delta, err := createValidatorSetDelta(validatorSetParent, validatorSetParent)
		require.NoError(t, err)

		header := &types.Header{Number: i}
		updateHeaderExtra(header, delta, nil, &CheckpointData{EpochNumber: 1}, nil)

		// add headers from 1 to 9 to map (blockchain imitation)
		headersMap.addHeader(header)
	}

	// mock blockchain
	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)
	blockchainMock.On("GetHeaderByHash", mock.Anything).Return(headersMap.getHeaderByHash)

	// create polybft with appropriate mocks
	polybft := &Polybft{
		closeCh:         make(chan struct{}),
		logger:          hclog.NewNullLogger(),
		consensusConfig: &polyBftConfig,
		blockchain:      blockchainMock,
		validatorsCache: newValidatorsSnapshotCache(
			hclog.NewNullLogger(),
			newTestState(t),
			blockchainMock,
		),
	}

	// create parent header (block 10)
	parentDelta, err := createValidatorSetDelta(validatorSetParent, validatorSetCurrent)
	require.NoError(t, err)

	parentHeader := &types.Header{
		Number:    polyBftConfig.EpochSize,
		Timestamp: uint64(time.Now().UTC().UnixMilli()),
	}
	parentCommitment := updateHeaderExtra(parentHeader, parentDelta, nil, &CheckpointData{EpochNumber: 1}, accountSetParent)

	// add parent header to map
	headersMap.addHeader(parentHeader)

	// create current header (block 11) with all appropriate fields required for validation
	currentDelta, err := createValidatorSetDelta(validatorSetCurrent, validatorSetCurrent)
	require.NoError(t, err)

	currentHeader := &types.Header{
		Number:     polyBftConfig.EpochSize + 1,
		ParentHash: parentHeader.Hash,
		Timestamp:  parentHeader.Timestamp + 1,
		MixHash:    PolyBFTMixDigest,
		Difficulty: 1,
	}
	updateHeaderExtra(currentHeader, currentDelta, nil,
		&CheckpointData{
			EpochNumber:           2,
			CurrentValidatorsHash: types.StringToHash("Foo"),
			NextValidatorsHash:    types.StringToHash("Bar"),
		}, nil)

	currentHeader.Hash[0] = currentHeader.Hash[0] + 1
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "invalid header hash")

	// omit Parent field (parent signature) intentionally
	updateHeaderExtra(currentHeader, currentDelta, nil,
		&CheckpointData{
			EpochNumber:           1,
			CurrentValidatorsHash: types.StringToHash("Foo"),
			NextValidatorsHash:    types.StringToHash("Bar")},
		accountSetCurrent)

	// since parent signature is intentionally disregarded the following error is expected
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	updateHeaderExtra(currentHeader, currentDelta, parentCommitment,
		&CheckpointData{
			EpochNumber:           1,
			CurrentValidatorsHash: types.StringToHash("Foo"),
			NextValidatorsHash:    types.StringToHash("Bar")},
		accountSetCurrent)

	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// clean validator snapshot cache (re-instantiate it), submit invalid validator set for parent signature and expect the following error
	polybft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock)
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(&validatorSnapshot{Epoch: 0, Snapshot: validatorSetCurrent})) // invalid validator set is submitted
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(&validatorSnapshot{Epoch: 1, Snapshot: validatorSetCurrent}))
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	// clean validators cache again and set valid snapshots
	polybft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock)
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(&validatorSnapshot{Epoch: 0, Snapshot: validatorSetParent}))
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(&validatorSnapshot{Epoch: 1, Snapshot: validatorSetCurrent}))
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// add current header to the blockchain (headersMap) and try validating again
	headersMap.addHeader(currentHeader)
	assert.NoError(t, polybft.VerifyHeader(currentHeader))
}

func TestPolybft_Close(t *testing.T) {
	t.Parallel()

	syncer := &syncerMock{}
	syncer.On("Close", mock.Anything).Return(error(nil)).Once()

	polybft := Polybft{
		closeCh: make(chan struct{}),
		syncer:  syncer,
		runtime: &consensusRuntime{stateSyncManager: &dummyStateSyncManager{}},
	}

	assert.NoError(t, polybft.Close())

	<-polybft.closeCh

	syncer.AssertExpectations(t)

	errExpected := errors.New("something")
	syncer.On("Close", mock.Anything).Return(errExpected).Once()

	polybft.closeCh = make(chan struct{})

	assert.Error(t, errExpected, polybft.Close())

	select {
	case <-polybft.closeCh:
		assert.Fail(t, "channel closing is invoked")
	case <-time.After(time.Millisecond * 100):
	}

	syncer.AssertExpectations(t)
}

func TestPolybft_GetSyncProgression(t *testing.T) {
	t.Parallel()

	result := &progress.Progression{}

	syncer := &syncerMock{}
	syncer.On("GetSyncProgression", mock.Anything).Return(result).Once()

	polybft := Polybft{
		syncer: syncer,
	}

	assert.Equal(t, result, polybft.GetSyncProgression())
}

func Test_Factory(t *testing.T) {
	t.Parallel()

	const epochSize = uint64(141)

	txPool := &txpool.TxPool{}

	params := &consensus.Params{
		TxPool: txPool,
		Logger: hclog.Default(),
		Config: &consensus.Config{
			Config: map[string]interface{}{
				"EpochSize": epochSize,
			},
		},
	}

	r, err := Factory(params)

	require.NoError(t, err)
	require.NotNil(t, r)

	polybft, ok := r.(*Polybft)
	require.True(t, ok)

	assert.Equal(t, txPool, polybft.txPool)
	assert.Equal(t, epochSize, polybft.consensusConfig.EpochSize)
	assert.Equal(t, params, polybft.config)
}
