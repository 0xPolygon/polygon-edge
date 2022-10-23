package polybft

import (
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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
		ExtraData: append(make([]byte, 32), genesisExtra.MarshalRLPTo(nil)...),
	}
	genesisHeader.ComputeHash()

	// add genesis header to map
	headersMap.addHeader(genesisHeader)

	// create headers from 1 to 9
	for i := 1; i < int(polyBftConfig.EpochSize); i++ {
		delta, err := createValidatorSetDelta(hclog.NewNullLogger(), validatorSetParent, validatorSetParent)
		require.NoError(t, err)
		extra := &Extra{Validators: delta}
		header := &types.Header{
			Number:    uint64(i),
			ExtraData: append(make([]byte, 32), extra.MarshalRLPTo(nil)...),
		}
		header.ComputeHash()

		// add header from 1 to 9 to map
		headersMap.addHeader(header)
	}

	// create parent header (block 10)
	parentDelta, err := createValidatorSetDelta(hclog.NewNullLogger(), validatorSetParent, validatorSetCurrent)
	require.NoError(t, err)

	parentExtra := &Extra{Validators: parentDelta}
	parentHeader := &types.Header{
		Number:    polyBftConfig.EpochSize,
		ExtraData: append(make([]byte, 32), parentExtra.MarshalRLPTo(nil)...),
		Timestamp: uint64(time.Now().UTC().UnixMilli()),
	}
	parentHeader.ComputeHash()
	parentCommitted := createSignature(t, accountSetParent, parentHeader.Hash)

	// now create new extra with committed and add it to parent header
	parentExtra = &Extra{Validators: parentDelta, Committed: parentCommitted}
	parentHeader.ExtraData = append(make([]byte, 32), parentExtra.MarshalRLPTo(nil)...)

	// add parent header  to map
	headersMap.addHeader(parentHeader)

	// create current header (block 11) with all appropriate fields required for validation
	currentDelta, err := createValidatorSetDelta(hclog.NewNullLogger(), validatorSetCurrent, validatorSetCurrent)
	require.NoError(t, err)

	currentExtra := &Extra{Validators: currentDelta, Parent: parentCommitted}
	currentHeader := &types.Header{
		Number:     polyBftConfig.EpochSize + 1,
		ExtraData:  append(make([]byte, 32), currentExtra.MarshalRLPTo(nil)...),
		ParentHash: parentHeader.Hash,
		Timestamp:  parentHeader.Timestamp + 1,
		MixHash:    PolyMixDigest,
		Difficulty: 1,
	}
	currentHeader.ComputeHash()

	//
	currentCommitted := createSignature(t, accountSetCurrent, currentHeader.Hash)
	// forget Parent field (parent signature) intentionally
	currentExtra = &Extra{Validators: currentDelta, Committed: currentCommitted}
	currentHeader.ExtraData = append(make([]byte, 32), currentExtra.MarshalRLPTo(nil)...)

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
		validatorsCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), polyBftConfig.EpochSize, blockchainMock),
	}

	// sice parent signature is intentionally disregarded the following error is expected
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	// create valid extra filed for current header and check the header
	// this is the situation before a block (a valid header) is added to the blockchain
	currentExtra = &Extra{Validators: currentDelta, Committed: currentCommitted, Parent: parentCommitted}
	currentHeader.ExtraData = append(make([]byte, 32), currentExtra.MarshalRLPTo(nil)...)
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// clean validator snapshot cache (reinstantiate it), submit invalid validator set for parnet signature and expect the following error
	polybft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), polyBftConfig.EpochSize, blockchainMock)
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(0, validatorSetCurrent)) // invalid valdator set is submitted
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(1, validatorSetCurrent))
	assert.ErrorContains(t, polybft.VerifyHeader(currentHeader), "failed to verify signatures for parent of block")

	// cleant validators cache again and set valid snapsots
	polybft.validatorsCache = newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), polyBftConfig.EpochSize, blockchainMock)
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(0, validatorSetParent))
	assert.NoError(t, polybft.validatorsCache.storeSnapshot(1, validatorSetCurrent))
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// add current header to the blockchain (headersMap) and try validating again
	headersMap.addHeader(currentHeader)
	assert.NoError(t, polybft.VerifyHeader(currentHeader))
}
