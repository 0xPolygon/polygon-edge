package polybft

import (
	"errors"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// the test initializes polybft and chain mock (map of headers) after wihch a new header is verified
// firstly, two invalid situation of header verifications are triggered (missing Committed field and invalid validators for ParentCommitted)
// afterwards, valid inclusion into the block chain is checked
// and at the end there is a situation when header is already a part of blockchain
func TestPolybft_VerifyHeader(t *testing.T) {
	t.Parallel()

	const (
		allValidatorsSize = 6 // overall there are 6 validators
		validatorSetSize  = 5 // only 5 validators are active at the time
	)

	// create all valdators
	validators := newTestValidators(allValidatorsSize)

	// create configuration
	polyBftConfig := PolyBFTConfig{
		InitialValidatorSet: validators.getParamValidators(),
		EpochSize:           10,
		SprintSize:          5,
		ValidatorSetSize:    validatorSetSize,
	}

	validatorSet := validators.getPublicIdentities()
	accounts := validators.getPrivateIdentities()

	// calculate validators before and after the end of the first epoch
	validatorSetParent, validatorSetCurrent := validatorSet[:len(validatorSet)-1], validatorSet[1:]
	accountSetParent, accountSetCurrent := accounts[:len(accounts)-1], accounts[1:]

	// create header map to simulate blockchain
	headersMap := &testHeadersMap{}

	// create genesis header
	genesisDelta, err := createValidatorSetDelta(hclog.NewNullLogger(), nil, validatorSetParent)
	require.NoError(t, err)

	genesisExtra := &Extra{Validators: genesisDelta}
	genesisHeader := &types.Header{
		Number:    0,
		ExtraData: append(make([]byte, signer.IstanbulExtraVanity), genesisExtra.MarshalRLPTo(nil)...),
	}
	genesisHeader.ComputeHash()

	// add genesis header to map
	headersMap.addHeader(genesisHeader)

	// create headers from 1 to 9
	for i := 1; i < int(polyBftConfig.EpochSize); i++ {
		delta, err := createValidatorSetDelta(hclog.NewNullLogger(), validatorSetParent, validatorSetParent)
		require.NoError(t, err)

		extra := &Extra{Validators: delta, Checkpoint: &CheckpointData{}}
		header := &types.Header{
			Number:    uint64(i),
			ExtraData: append(make([]byte, signer.IstanbulExtraVanity), extra.MarshalRLPTo(nil)...),
		}
		header.ComputeHash()

		// add headers from 1 to 9 to map (blockchain imitation)
		headersMap.addHeader(header)
	}

	// mock blockchain
	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)
	blockchainMock.On("GetHeaderByHash", mock.Anything).Return(headersMap.getHeaderByHash)

	// create parent header (block 10)
	parentDelta, err := createValidatorSetDelta(hclog.NewNullLogger(), validatorSetParent, validatorSetCurrent)
	require.NoError(t, err)

	parentExtra := &Extra{Validators: parentDelta, Checkpoint: &CheckpointData{}}
	parentHeader := &types.Header{
		Number:    polyBftConfig.EpochSize,
		ExtraData: append(make([]byte, signer.IstanbulExtraVanity), parentExtra.MarshalRLPTo(nil)...),
		Timestamp: uint64(time.Now().UnixMilli()),
	}
	parentHeader.ComputeHash()

	checkpointHash, err := parentExtra.Checkpoint.Hash(blockchainMock.GetChainID(), parentHeader.Number, parentHeader.Hash)
	require.NoError(t, err)

	parentCommitted := createSignature(t, accountSetParent, checkpointHash)

	// now create new extra with committed and add it to parent header
	parentExtra = &Extra{Validators: parentDelta, Committed: parentCommitted, Checkpoint: &CheckpointData{}}
	parentHeader.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), parentExtra.MarshalRLPTo(nil)...)

	// add parent header to map
	headersMap.addHeader(parentHeader)

	// create current header (block 11) with all appropriate fields required for validation
	currentDelta, err := createValidatorSetDelta(hclog.NewNullLogger(), validatorSetCurrent, validatorSetCurrent)
	require.NoError(t, err)

	currentExtra := &Extra{Validators: currentDelta, Parent: parentCommitted, Checkpoint: &CheckpointData{}}
	currentHeader := &types.Header{
		Number:     polyBftConfig.EpochSize + 1,
		ParentHash: parentHeader.Hash,
		Timestamp:  parentHeader.Timestamp + 1,
		MixHash:    PolyBFTMixDigest,
		Difficulty: 1,
	}
	currentHeader.ComputeHash()

	signingHash, err := new(CheckpointData).Hash(blockchainMock.GetChainID(), currentHeader.Number, currentHeader.Hash)
	require.NoError(t, err)

	currentCommitted := createSignature(t, accountSetCurrent, signingHash)
	// forget Parent field (parent signature) intentionally
	currentExtra = &Extra{Validators: currentDelta, Committed: currentCommitted, Checkpoint: &CheckpointData{}}
	currentHeader.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), currentExtra.MarshalRLPTo(nil)...)

	// create polybft with appropriate mocks
	polybft := &Polybft{
		closeCh:         make(chan struct{}),
		logger:          hclog.NewNullLogger(),
		consensusConfig: &polyBftConfig,
		blockchain:      blockchainMock,
		validatorsCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), polyBftConfig.EpochSize, blockchainMock),
	}

	// since parent signature is intentionally disregarded the following error is expected
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	// create valid extra filed for current header and check the header
	// this is the situation before a block (a valid header) is added to the blockchain
	currentExtra = &Extra{
		Validators: currentDelta,
		Committed:  currentCommitted,
		Parent:     parentCommitted,
		Checkpoint: &CheckpointData{},
	}
	currentHeader.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), currentExtra.MarshalRLPTo(nil)...)
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// clean validator snapshot cache (reinstantiate it), submit invalid validator set for parnet signature and expect the following error
	polybft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), polyBftConfig.EpochSize, blockchainMock)
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(0, validatorSetCurrent)) // invalid valdator set is submitted
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(1, validatorSetCurrent))
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	// clean validators cache again and set valid snapshots
	polybft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), polyBftConfig.EpochSize, blockchainMock)
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(0, validatorSetParent))
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(1, validatorSetCurrent))
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// add current header to the blockchain (headersMap) and try validating again
	headersMap.addHeader(currentHeader)
	assert.NoError(t, polybft.VerifyHeader(currentHeader))
}

func TestPolybft_Close(t *testing.T) {
	syncer := &syncerMock{}
	syncer.On("Close", mock.Anything).Return(error(nil)).Once()

	polybft := Polybft{
		closeCh: make(chan struct{}),
		syncer:  syncer,
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
	result := &progress.Progression{}

	syncer := &syncerMock{}
	syncer.On("GetSyncProgression", mock.Anything).Return(result).Once()

	polybft := Polybft{
		syncer: syncer,
	}

	assert.Equal(t, result, polybft.GetSyncProgression())
}

func Test_Factory(t *testing.T) {
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
