package polybft

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestValidatorsSnapshotCache_GetSnapshot_Build(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 10
		validatorSetSize = 5
		epochSize        = uint64(10)
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()
	headersMap := &testHeadersMap{}

	var oddValidators, evenValidators AccountSet

	for i := 0; i < totalValidators; i++ {
		if i%2 == 0 {
			evenValidators = append(evenValidators, allValidators[i])
		} else {
			oddValidators = append(oddValidators, allValidators[i])
		}
	}
	headersMap.addHeader(createHeader(0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createHeader(1*epochSize, allValidators[:validatorSetSize], allValidators[validatorSetSize:]))
	headersMap.addHeader(createHeader(2*epochSize, allValidators[validatorSetSize:], oddValidators))
	headersMap.addHeader(createHeader(3*epochSize, oddValidators, evenValidators))

	var cases = []struct {
		blockNumber       uint64
		expectedSnapshot  AccountSet
		validatorsOverlap bool
		parents           []*types.Header
	}{
		{4, allValidators[:validatorSetSize], false, nil},
		{1 * epochSize, allValidators[validatorSetSize:], false, nil},
		{13, allValidators[validatorSetSize:], false, nil},
		{27, oddValidators, true, nil},
		{36, evenValidators, true, nil},
		{4, allValidators[:validatorSetSize], false, headersMap.getHeaders()},
		{13, allValidators[validatorSetSize:], false, headersMap.getHeaders()},
		{27, oddValidators, true, headersMap.getHeaders()},
		{36, evenValidators, true, headersMap.getHeaders()},
	}

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), epochSize, blockchainMock),
	}

	for _, c := range cases {
		snapshot, err := testValidatorsCache.GetSnapshot(c.blockNumber, c.parents)

		assertions.NoError(err)
		assertions.Len(snapshot, c.expectedSnapshot.Len())

		if c.validatorsOverlap {
			for _, validator := range c.expectedSnapshot {
				// Order of validators is not preserved, because there are overlapping between validators set.
				// In that case, at the beginning of the set are the ones preserved from the previous validator set.
				// Newly validators are added to the end after the one from previous validator set.
				assertions.True(snapshot.ContainsAddress(validator.Address))
			}
		} else {
			assertions.Equal(c.expectedSnapshot, snapshot)
		}

		assertions.NoError(testValidatorsCache.cleanValidatorsCache())

		if c.parents != nil {
			blockchainMock.AssertNotCalled(t, "GetHeaderByNumber")
		}
	}
}

func TestValidatorsSnapshotCache_GetSnapshot_FetchFromCache(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	const (
		totalValidators  = 10
		validatorSetSize = 5
		epochSize        = uint64(10)
	)

	validators := newTestValidators(totalValidators).getPublicIdentities()
	headersMap := &testHeadersMap{}
	epoch0Validators := AccountSet{validators[0], validators[len(validators)-1]}
	epoch1Validators := validators[1 : len(validators)-2]

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), epochSize, blockchainMock),
	}
	require.NoError(testValidatorsCache.storeSnapshot(0, epoch0Validators))
	require.NoError(testValidatorsCache.storeSnapshot(1, epoch1Validators))

	// Fetch snapshot from in memory cache
	snapshot, err := testValidatorsCache.GetSnapshot(7, nil)
	require.NoError(err)
	require.Equal(epoch0Validators, snapshot)

	// Invalidate in memory cache
	testValidatorsCache.snapshots = make(map[uint64]AccountSet)
	// Fetch snapshot from database
	snapshot, err = testValidatorsCache.GetSnapshot(8, nil)
	require.NoError(err)
	require.Equal(epoch0Validators, snapshot)

	snapshot, err = testValidatorsCache.GetSnapshot(epochSize, nil)
	require.NoError(err)
	require.Equal(epoch1Validators, snapshot)

	blockchainMock.AssertNotCalled(t, "GetHeaderByNumber")
}

func TestValidatorsSnapshotCache_Cleanup(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockchainMock := new(blockchainMock)
	cache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), 10, blockchainMock),
	}
	snapshot := newTestValidators(3).getPublicIdentities()
	maxEpoch := uint64(0)

	for i := 0; i < validatorSnapshotLimit; i++ {
		require.NoError(cache.storeSnapshot(uint64(i), snapshot))
		maxEpoch++
	}

	require.NoError(cache.cleanup())

	// assertions for remaining snapshots in the in memory cache
	require.Len(cache.snapshots, numberOfSnapshotsToLeaveInMemory)

	currentEpoch := maxEpoch

	for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
		currentEpoch--
		currentSnapshot, snapExists := cache.snapshots[currentEpoch]
		require.True(snapExists, fmt.Sprintf("failed to fetch in memory snapshot for epoch %d", currentEpoch))
		require.Equal(snapshot, currentSnapshot, fmt.Sprintf("snapshots for epoch %d are not equal", currentEpoch))
	}

	// assertions for remaining snapshots in database
	require.Equal(cache.state.validatorSnapshotsDBStats().KeyN, numberOfSnapshotsToLeaveInDB)

	currentEpoch = maxEpoch

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		currentEpoch--
		currentSnapshot, err := cache.state.getValidatorSnapshot(currentEpoch)
		require.NoError(err, fmt.Sprintf("failed to fetch database snapshot for epoch %d", currentEpoch))
		require.Equal(snapshot, currentSnapshot, fmt.Sprintf("snapshots for epoch %d are not equal", currentEpoch))
	}
}

func TestValidatorsSnapshotCache_ComputeSnapshot_UnknownEpochEndingBlock(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 15
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()
	headersMap := &testHeadersMap{}
	headersMap.addHeader(createHeader(0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createHeader(1*epochSize, allValidators[:validatorSetSize], allValidators[validatorSetSize:]))

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), epochSize, blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(nil, 5*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "unknown block. Block number=50")
}

func TestValidatorsSnapshotCache_ComputeSnapshot_IncorrectExtra(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 6
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()
	headersMap := &testHeadersMap{}
	invalidHeader := createHeader(1*epochSize, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
	invalidHeader.ExtraData = []byte{0x2, 0x7}
	headersMap.addHeader(invalidHeader)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), epochSize, blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(nil, 1*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "failed to decode extra from the block#10: wrong extra size: 2")
}

func TestValidatorsSnapshotCache_ComputeSnapshot_ApplyDeltaFail(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 6
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()
	headersMap := &testHeadersMap{}
	headersMap.addHeader(createHeader(0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createHeader(1*epochSize, nil, allValidators[:validatorSetSize]))

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), epochSize, blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(allValidators, 1*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "failed to apply delta to the validators snapshot, block#10")
}

func createHeader(number uint64, oldValidatorSet, newValidatorSet AccountSet) *types.Header {
	delta, _ := createValidatorSetDelta(hclog.NewNullLogger(), oldValidatorSet, newValidatorSet)
	extra := &Extra{Validators: delta}

	return &types.Header{
		Number:    number,
		ExtraData: append(make([]byte, 32), extra.MarshalRLPTo(nil)...),
	}
}

type testValidatorsCache struct {
	*validatorsSnapshotCache
}

func (c *testValidatorsCache) cleanValidatorsCache() error {
	c.snapshots = make(map[uint64]AccountSet)

	return c.state.removeAllValidatorSnapshots()
}
