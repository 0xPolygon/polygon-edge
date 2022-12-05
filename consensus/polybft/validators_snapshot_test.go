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
	assertions := require.New(t)

	const (
		totalValidators  = 10
		validatorSetSize = 5
		epochSize        = uint64(10)
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()

	var oddValidators, evenValidators AccountSet

	for i := 0; i < totalValidators; i++ {
		if i%2 == 0 {
			evenValidators = append(evenValidators, allValidators[i])
		} else {
			oddValidators = append(oddValidators, allValidators[i])
		}
	}

	headersMap := &testHeadersMap{headersByNumber: make(map[uint64]*types.Header)}

	createHeaders(t, headersMap, 0, epochSize-1, 1, nil, allValidators[:validatorSetSize])
	createHeaders(t, headersMap, epochSize, 2*epochSize-1, 2, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
	createHeaders(t, headersMap, 2*epochSize, 3*epochSize-1, 3, allValidators[validatorSetSize:], oddValidators)
	createHeaders(t, headersMap, 3*epochSize, 4*epochSize-1, 4, oddValidators, evenValidators)

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
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
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
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()
	epochOneValidators := AccountSet{allValidators[0], allValidators[len(allValidators)-1]}
	epochTwoValidators := allValidators[1 : len(allValidators)-2]

	headersMap := &testHeadersMap{headersByNumber: make(map[uint64]*types.Header)}
	createHeaders(t, headersMap, 0, 9, 1, nil, allValidators)
	createHeaders(t, headersMap, 10, 19, 2, allValidators, epochOneValidators)
	createHeaders(t, headersMap, 20, 29, 3, epochOneValidators, epochTwoValidators)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	require.NoError(testValidatorsCache.storeSnapshot(&validatorSnapshot{1, 10, epochOneValidators}))
	require.NoError(testValidatorsCache.storeSnapshot(&validatorSnapshot{2, 20, epochTwoValidators}))

	// Fetch snapshot from in memory cache
	snapshot, err := testValidatorsCache.GetSnapshot(10, nil)
	require.NoError(err)
	require.Equal(epochOneValidators, snapshot)

	// Invalidate in memory cache
	testValidatorsCache.snapshots = map[uint64]*validatorSnapshot{}
	require.NoError(testValidatorsCache.state.removeAllValidatorSnapshots())
	// Fetch snapshot from database
	snapshot, err = testValidatorsCache.GetSnapshot(10, nil)
	require.NoError(err)
	require.Equal(epochOneValidators, snapshot)

	snapshot, err = testValidatorsCache.GetSnapshot(20, nil)
	require.NoError(err)
	require.Equal(epochTwoValidators, snapshot)
}

func TestValidatorsSnapshotCache_Cleanup(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockchainMock := new(blockchainMock)
	cache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}
	snapshot := newTestValidators(3).getPublicIdentities()
	maxEpoch := uint64(0)

	for i := uint64(0); i < validatorSnapshotLimit; i++ {
		require.NoError(cache.storeSnapshot(&validatorSnapshot{i, i * 10, snapshot}))
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
		require.Equal(snapshot, currentSnapshot.Snapshot, fmt.Sprintf("snapshots for epoch %d are not equal", currentEpoch))
	}

	// assertions for remaining snapshots in database
	require.Equal(cache.state.validatorSnapshotsDBStats().KeyN, numberOfSnapshotsToLeaveInDB)

	currentEpoch = maxEpoch

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		currentEpoch--
		currentSnapshot, err := cache.state.getValidatorSnapshot(currentEpoch)
		require.NoError(err, fmt.Sprintf("failed to fetch database snapshot for epoch %d", currentEpoch))
		require.Equal(snapshot, currentSnapshot.Snapshot, fmt.Sprintf("snapshots for epoch %d are not equal", currentEpoch))
	}
}

func TestValidatorsSnapshotCache_ComputeSnapshot_UnknownBlock(t *testing.T) {
	t.Parallel()
	assertions := assert.New(t)

	const (
		totalValidators  = 15
		validatorSetSize = totalValidators / 2
		epochSize        = uint64(10)
	)

	allValidators := newTestValidators(totalValidators).getPublicIdentities()
	headersMap := &testHeadersMap{}
	headersMap.addHeader(createValidatorDeltaHeader(t, 0, 0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createValidatorDeltaHeader(t, 1*epochSize, 1, allValidators[:validatorSetSize], allValidators[validatorSetSize:]))

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
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
	invalidHeader := createValidatorDeltaHeader(t, 1*epochSize, 1, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
	invalidHeader.ExtraData = []byte{0x2, 0x7}
	headersMap.addHeader(invalidHeader)

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
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
	headersMap.addHeader(createValidatorDeltaHeader(t, 0, 0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createValidatorDeltaHeader(t, 1*epochSize, 1, nil, allValidators[:validatorSetSize]))

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}

	snapshot, err := testValidatorsCache.computeSnapshot(&validatorSnapshot{0, 0, allValidators}, 1*epochSize, nil)
	assertions.Nil(snapshot)
	assertions.ErrorContains(err, "failed to apply delta to the validators snapshot, block#10")
}

func createHeaders(t *testing.T, headersMap *testHeadersMap,
	fromBlock, toBlock, epoch uint64, oldValidators, newValidators AccountSet) {
	t.Helper()

	headersMap.addHeader(createValidatorDeltaHeader(t, fromBlock, epoch-1, oldValidators, newValidators))

	for i := fromBlock + 1; i <= toBlock; i++ {
		headersMap.addHeader(createValidatorDeltaHeader(t, i, epoch, nil, nil))
	}
}

func createValidatorDeltaHeader(t *testing.T, blockNumber, epoch uint64, oldValidatorSet, newValidatorSet AccountSet) *types.Header {
	t.Helper()

	delta, _ := createValidatorSetDelta(oldValidatorSet, newValidatorSet)
	extra := &Extra{Validators: delta, Checkpoint: &CheckpointData{EpochNumber: epoch}}

	return &types.Header{
		Number:    blockNumber,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}
}

type testValidatorsCache struct {
	*validatorsSnapshotCache
}

func (c *testValidatorsCache) cleanValidatorsCache() error {
	c.snapshots = make(map[uint64]*validatorSnapshot)

	return c.state.removeAllValidatorSnapshots()
}
