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

	var oddValidators, evenValidators AccountSet

	for i := 0; i < totalValidators; i++ {
		if i%2 == 0 {
			evenValidators = append(evenValidators, allValidators[i])
		} else {
			oddValidators = append(oddValidators, allValidators[i])
		}
	}

	headersMap := &testHeadersMap{headersByNumber: make(map[uint64]*types.Header)}
	createHeaders := func(fromBlock, toBlock uint64, oldValidators, newValidators AccountSet) {
		headersMap.addHeader(createValidatorDeltaHeader(fromBlock, oldValidators, newValidators))

		for i := fromBlock + 1; i <= toBlock; i++ {
			headersMap.addHeader(createValidatorDeltaHeader(i, nil, newValidators))
		}
	}

	createHeaders(0, epochSize-1, nil, allValidators[:validatorSetSize])
	createHeaders(epochSize, 2*epochSize-1, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
	createHeaders(2*epochSize, 3*epochSize-1, allValidators[validatorSetSize:], oddValidators)
	createHeaders(3*epochSize, 4*epochSize-1, oddValidators, evenValidators)

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
	blockOneValidators := AccountSet{allValidators[0], allValidators[len(allValidators)-1]}
	blockTwoValidators := allValidators[1 : len(allValidators)-2]

	blockchainMock := new(blockchainMock)

	testValidatorsCache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock),
	}
	require.NoError(testValidatorsCache.storeSnapshot(10, blockOneValidators))
	require.NoError(testValidatorsCache.storeSnapshot(20, blockTwoValidators))

	// Fetch snapshot from in memory cache
	snapshot, err := testValidatorsCache.GetSnapshot(10, nil)
	require.NoError(err)
	require.Equal(blockOneValidators, snapshot)

	// Invalidate in memory cache
	testValidatorsCache.snapshots = make(map[uint64]AccountSet)
	// Fetch snapshot from database
	snapshot, err = testValidatorsCache.GetSnapshot(10, nil)
	require.NoError(err)
	require.Equal(blockOneValidators, snapshot)

	snapshot, err = testValidatorsCache.GetSnapshot(20, nil)
	require.NoError(err)
	require.Equal(blockTwoValidators, snapshot)

	blockchainMock.AssertNotCalled(t, "GetHeaderByNumber")
}

func TestValidatorsSnapshotCache_Cleanup(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockchainMock := new(blockchainMock)
	cache := &testValidatorsCache{
		validatorsSnapshotCache: newValidatorsSnapshotCache(hclog.NewNullLogger(), newTestState(t), blockchainMock)}
	snapshot := newTestValidators(3).getPublicIdentities()
	maxBlock := uint64(0)

	for i := 0; i < validatorSnapshotLimit; i++ {
		require.NoError(cache.storeSnapshot(uint64(i), snapshot))
		maxBlock++
	}

	require.NoError(cache.cleanup())

	// assertions for remaining snapshots in the in memory cache
	require.Len(cache.snapshots, numberOfSnapshotsToLeaveInMemory)

	currentBlock := maxBlock

	for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
		currentBlock--
		currentSnapshot, snapExists := cache.snapshots[currentBlock]
		require.True(snapExists, fmt.Sprintf("failed to fetch in memory snapshot for block %d", currentBlock))
		require.Equal(snapshot, currentSnapshot, fmt.Sprintf("snapshots for block %d are not equal", currentBlock))
	}

	// assertions for remaining snapshots in database
	require.Equal(cache.state.validatorSnapshotsDBStats().KeyN, numberOfSnapshotsToLeaveInDB)

	currentBlock = maxBlock

	for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ {
		currentBlock--
		currentSnapshot, err := cache.state.getValidatorSnapshot(currentBlock)
		require.NoError(err, fmt.Sprintf("failed to fetch database snapshot for block %d", currentBlock))
		require.Equal(snapshot, currentSnapshot, fmt.Sprintf("snapshots for block %d are not equal", currentBlock))
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
	headersMap.addHeader(createValidatorDeltaHeader(t, 0, nil, allValidators[:validatorSetSize]))
	headersMap.addHeader(createValidatorDeltaHeader(t, 1*epochSize, allValidators[:validatorSetSize], allValidators[validatorSetSize:]))

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
	invalidHeader := createValidatorDeltaHeader(t, 1*epochSize, allValidators[:validatorSetSize], allValidators[validatorSetSize:])
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

func createValidatorDeltaHeader(number uint64, oldValidatorSet, newValidatorSet AccountSet) *types.Header {
	delta, _ := createValidatorSetDelta(oldValidatorSet, newValidatorSet)
	extra := &Extra{Validators: delta}

	return &types.Header{
		Number:    number,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}
}

type testValidatorsCache struct {
	*validatorsSnapshotCache
}

func (c *testValidatorsCache) cleanValidatorsCache() error {
	c.snapshots = make(map[uint64]AccountSet)

	return c.state.removeAllValidatorSnapshots()
}
