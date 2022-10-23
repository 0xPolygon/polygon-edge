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

func TestPolybft_VerifyHeader_Good(t *testing.T) {
	t.Parallel()

	const (
		allValidatorsSize = 6 // all together there are 6 validators
		validatorSetSize  = 5 // only 5 validators are active at the time
	)

	// create all valdators
	validators := newTestValidators(allValidatorsSize)
	polyBftConfig := PolyBFTConfig{
		InitialValidatorSet: validators.getParamValidators(),
		EpochSize:           10,
		SprintSize:          5,
		ValidatorSetSize:    validatorSetSize,
	}

	validatorSet := validators.getPublicIdentities()
	accounts := validators.getPrivateIdentities()

	// calculate validators before and after the end of the first epoch
	validatorSetParent := validatorSet[:len(validatorSet)-1]
	validatorSetCurrent := validatorSet[1:]
	accountSetParent := accounts[:len(accounts)-1]
	accountSetCurrent := accounts[1:]

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

	// now create new extra with commited and add it to parent header
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

	currentCommitted := createSignature(t, accountSetCurrent, currentHeader.Hash)
	currentExtra = &Extra{Validators: currentDelta, Committed: currentCommitted, Parent: parentCommitted}
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

	// check if current header is not added into the block chain
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// add current header  to block chain and try validating again
	headersMap.addHeader(currentHeader)
	assert.NoError(t, polybft.VerifyHeader(currentHeader))

	// cleant validators cache and set them manualy, afterwards try header verification again
	// polybft.validatorsCache.cleanup()
	// assert.NoError(t, polybft.validatorsCache.storeSnapshot(1, validatorSetParent))
	// assert.NoError(t, polybft.validatorsCache.storeSnapshot(2, validatorSetCurrent))
	// assert.NoError(t, polybft.VerifyHeader(currentHeader))
}
