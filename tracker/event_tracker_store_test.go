package tracker

import (
	"encoding/hex"
	"io/ioutil"
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

func createSetupDB(subscriber eventSubscription, finalityDepth uint64) store.SetupDB {
	return func(t *testing.T) (store.Store, func()) {
		t.Helper()

		dir, err := ioutil.TempDir("/tmp", "boltdb-test")
		require.NoError(t, err)

		path := filepath.Join(dir, "test.db")
		store, err := NewEventTrackerStore(path, finalityDepth, subscriber, hclog.Default())
		require.NoError(t, err)

		closeFn := func() {
			require.NoError(t, os.RemoveAll(dir))
		}

		return store, closeFn
	}
}

func TestBoltDBStore(t *testing.T) {
	t.Parallel()

	store.TestStore(t, createSetupDB(nil, 2))
}

func Test_Entry_getFinalizedLogs(t *testing.T) {
	t.Parallel()

	const someFilterHash = "test"

	tstore, fn := createSetupDB(nil, 3)(t)
	defer fn()

	entry, err := tstore.(*EventTrackerStore).getImplEntry(someFilterHash)
	require.NoError(t, err)

	entry.StoreLogs([]*ethgo.Log{
		{BlockNumber: 1}, {BlockNumber: 5}, {BlockNumber: 8}, {BlockNumber: 11}, {BlockNumber: 12}, {BlockNumber: 15},
	})

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

func Test_Entry_saveNextToProcessIndx(t *testing.T) {
	const someFilterHash = "test"

	tstore, fn := createSetupDB(nil, 2)(t)
	defer fn()

	entry, err := tstore.(*EventTrackerStore).getImplEntry(someFilterHash)
	require.NoError(t, err)

	entry.StoreLogs([]*ethgo.Log{
		{BlockNumber: 1}, {BlockNumber: 5}, {BlockNumber: 8}, {BlockNumber: 11}, {BlockNumber: 12}, {BlockNumber: 15},
	})

	for i := 0; i < 10; i++ {
		require.NoError(t, entry.saveNextToProcessIndx(uint64(i)))
	}
}

func Test_EventTrackerStore_SetNotLastBlock(t *testing.T) {
	t.Parallel()

	subs := &mockEventSubscriber{}

	tstore, fn := createSetupDB(subs, 2)(t)
	defer fn()

	assert.NoError(t, tstore.(*EventTrackerStore).Set("dummy", "dummy")) //nolint
	assert.Len(t, subs.logs, 0)
}

func Test_EventTrackerStore_onNewBlockBad(t *testing.T) {
	t.Parallel()

	tstore, fn := createSetupDB(nil, 0)(t)
	defer fn()

	assert.Error(t, tstore.(*EventTrackerStore).onNewBlock("dummy", "dummy"))                          //nolint
	assert.Error(t, tstore.(*EventTrackerStore).onNewBlock("dummy", hex.EncodeToString([]byte{0, 1}))) //nolint
}

func Test_EventTrackerStore_OnNewBlockNothingToProcess(t *testing.T) {
	t.Parallel()

	subs := &mockEventSubscriber{}

	tstore, fn := createSetupDB(subs, 10)(t)
	defer fn()

	block := ethgo.Block{Number: 8}

	bytes, err := block.MarshalJSON()
	require.NoError(t, err)

	value := hex.EncodeToString(bytes)

	// block less than finality depth
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

func Test_EventTrackerStore_SetLastBlockSubscriberNotified(t *testing.T) {
	t.Parallel()

	const hash = "dummy_hash"

	subs := &mockEventSubscriber{}

	tstore, fn := createSetupDB(subs, 2)(t)
	defer fn()

	entry, err := tstore.GetEntry(hash)
	require.NoError(t, err)

	require.NoError(t, entry.StoreLogs([]*ethgo.Log{
		{BlockNumber: 1}, {BlockNumber: 2}, {BlockNumber: 3},
	}))

	for i := 0; i < 3; i++ {
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
