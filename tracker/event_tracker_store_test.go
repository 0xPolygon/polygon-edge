package tracker

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/tracker/store"
)

func createSetupDB(subscriber eventSubscription, numBlockConfirmations uint64) store.SetupDB {
	return func(t *testing.T) (store.Store, func()) {
		t.Helper()

		dir, err := os.MkdirTemp("/tmp", "boltdb-test")
		require.NoError(t, err)

		path := filepath.Join(dir, "test.db")
		store, err := NewEventTrackerStore(path, numBlockConfirmations, subscriber, hclog.Default())
		require.NoError(t, err)

		closeFn := func() {
			require.NoError(t, os.RemoveAll(dir))
		}

		return store, closeFn
	}
}

func TestBoltDBStore(t *testing.T) {
	store.TestStore(t, createSetupDB(nil, 2))
}

func TestEntry_getFinalizedLogs(t *testing.T) {
	const someFilterHash = "test"

	tstore, closeFn := createSetupDB(nil, 3)(t)
	defer closeFn()

	entry, err := tstore.(*EventTrackerStore).getImplEntry(someFilterHash)
	require.NoError(t, err)

	require.NoError(t, entry.StoreLogs([]*ethgo.Log{
		{BlockNumber: 1}, {BlockNumber: 5}, {BlockNumber: 8}, {BlockNumber: 11}, {BlockNumber: 12}, {BlockNumber: 15},
	}))

	logs, key, err := entry.getFinalizedLogs(10)

	assert.NoError(t, err)
	assert.Len(t, logs, 3)
	assert.Equal(t, common.EncodeUint64ToBytes(2), key)

	err = entry.saveNextToProcessIndx(1) // next time should start from the second one
	require.NoError(t, err)

	logs, key, err = entry.getFinalizedLogs(14)

	assert.NoError(t, err)
	assert.Len(t, logs, 4)
	assert.Equal(t, common.EncodeUint64ToBytes(4), key)
}

func TestEntry_saveNextToProcessIndx(t *testing.T) {
	const someFilterHash = "test"

	tstore, closeFn := createSetupDB(nil, 2)(t)
	defer closeFn()

	entry, err := tstore.(*EventTrackerStore).getImplEntry(someFilterHash)
	require.NoError(t, err)

	require.NoError(t, entry.StoreLogs([]*ethgo.Log{
		{BlockNumber: 1}, {BlockNumber: 5}, {BlockNumber: 8}, {BlockNumber: 11}, {BlockNumber: 12}, {BlockNumber: 15},
	}))

	for i := 0; i < 10; i++ {
		require.NoError(t, entry.saveNextToProcessIndx(uint64(i)))
	}
}

func TestEventTrackerStore_SetNotLastBlock(t *testing.T) {
	subs := &mockEventSubscriber{}

	tstore, closeFn := createSetupDB(subs, 2)(t)
	defer closeFn()

	assert.NoError(t, tstore.(*EventTrackerStore).Set("dummy", "dummy")) //nolint
	assert.Len(t, subs.logs, 0)
}

func TestEventTrackerStore_onNewBlockBad(t *testing.T) {
	tstore, closeFn := createSetupDB(nil, 0)(t)
	defer closeFn()

	assert.Error(t, tstore.(*EventTrackerStore).onNewBlock("dummy", "dummy"))                          //nolint
	assert.Error(t, tstore.(*EventTrackerStore).onNewBlock("dummy", hex.EncodeToString([]byte{0, 1}))) //nolint
}

func TestEventTrackerStore_OnNewBlockNothingToProcess(t *testing.T) {
	subs := &mockEventSubscriber{}

	tstore, closeFn := createSetupDB(subs, 10)(t)
	defer closeFn()

	block := ethgo.Block{Number: 8}

	bytes, err := block.MarshalJSON()
	require.NoError(t, err)

	value := hex.EncodeToString(bytes)

	// block less than numBlockConfirmations
	assert.NoError(t, tstore.(*EventTrackerStore).onNewBlock("dummy", value)) //nolint
	assert.Len(t, subs.logs, 0)

	block = ethgo.Block{Number: 12}

	bytes, err = block.MarshalJSON()
	require.NoError(t, err)

	value = hex.EncodeToString(bytes)

	// no logs
	assert.NoError(t, tstore.(*EventTrackerStore).onNewBlock("dummy", value)) //nolint
	assert.Len(t, subs.logs, 0)
}

func TestEventTrackerStore_SetLastBlockSubscriberNotified(t *testing.T) {
	const hash = "dummy_hash"

	subs := &mockEventSubscriber{}

	tstore, closeFn := createSetupDB(subs, 2)(t)
	defer closeFn()

	entry, err := tstore.GetEntry(hash)
	require.NoError(t, err)

	require.NoError(t, entry.StoreLogs([]*ethgo.Log{
		{BlockNumber: 1}, {BlockNumber: 2}, {BlockNumber: 3},
	}))

	// There are 3 logs in store (one for block 1, one for block 2, one for block 3) and numBlockConfirmations is 2
	// If block 2 arrives (`tstore.Set(dbLastBlockPrefix+hash, value)`) subscriber should be notified with 0 logs
	// If block 3 arrives subscriber should be notified with 1 log
	// If block 4 arrives subscriber should be notified with 2 logs
	// If block 5 arrives subscriber should be notified with all 3 logs
	for i := 0; i < 4; i++ {
		block := ethgo.Block{Number: uint64(i + 2)}

		bytes, err := block.MarshalJSON()
		require.NoError(t, err)

		value := hex.EncodeToString(bytes)

		assert.NoError(t, tstore.Set(dbLastBlockPrefix+hash, value))
		assert.Len(t, subs.logs, i)

		subs.logs = nil

		require.NoError(t, entry.(*Entry).saveNextToProcessIndx(0)) //nolint
	}
}
