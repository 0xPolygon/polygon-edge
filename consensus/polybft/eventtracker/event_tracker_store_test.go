package eventtracker

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

// newTestState creates new instance of state used by tests.
func newTestTrackerStore(tb testing.TB) *BoltDBEventTrackerStore {
	tb.Helper()

	dir := fmt.Sprintf("/tmp/even-tracker-temp_%v", time.Now().UTC().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0775)

	if err != nil {
		tb.Fatal(err)
	}

	store, err := NewBoltDBEventTrackerStore(path.Join(dir, "tracker.db"))
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			tb.Fatal(err)
		}
	})

	return store
}

func TestEventTrackerStore_InsertAndGetLastProcessedBlock(t *testing.T) {
	t.Parallel()

	t.Run("No blocks inserted", func(t *testing.T) {
		t.Parallel()

		store := newTestTrackerStore(t)

		result, err := store.GetLastProcessedBlock()
		require.NoError(t, err)
		require.Equal(t, uint64(0), result)
	})

	t.Run("Has a block inserted", func(t *testing.T) {
		t.Parallel()

		store := newTestTrackerStore(t)

		require.NoError(t, store.InsertLastProcessedBlock(10))

		result, err := store.GetLastProcessedBlock()
		require.NoError(t, err)
		require.Equal(t, uint64(10), result)
	})

	t.Run("Insert a bunch of blocks - only the last one should be the result", func(t *testing.T) {
		t.Parallel()

		store := newTestTrackerStore(t)

		for i := uint64(0); i <= 20; i++ {
			require.NoError(t, store.InsertLastProcessedBlock(i))
		}

		result, err := store.GetLastProcessedBlock()
		require.NoError(t, err)
		require.Equal(t, uint64(20), result)
	})
}

func TestEventTrackerStore_InsertAndGetLogs(t *testing.T) {
	t.Parallel()

	t.Run("No logs inserted", func(t *testing.T) {
		t.Parallel()

		store := newTestTrackerStore(t)

		log, err := store.GetLog(1, 1)
		require.NoError(t, err)
		require.Nil(t, log)

		logs, err := store.GetLogsByBlockNumber(1)
		require.NoError(t, err)
		require.Nil(t, logs)
	})

	t.Run("Has some logs in store, but no desired log", func(t *testing.T) {
		t.Parallel()

		store := newTestTrackerStore(t)

		require.NoError(t, store.InsertLogs([]*ethgo.Log{createTestLogForStateSyncEvent(t, 1, 0)}))

		log, err := store.GetLog(1, 1)
		require.NoError(t, err)
		require.Nil(t, log)
	})

	t.Run("Has some logs in store, but no desired logs for specific block", func(t *testing.T) {
		t.Parallel()

		store := newTestTrackerStore(t)

		require.NoError(t, store.InsertLogs([]*ethgo.Log{createTestLogForStateSyncEvent(t, 1, 0)}))

		logs, err := store.GetLogsByBlockNumber(2)
		require.NoError(t, err)
		require.Nil(t, logs)
	})

	t.Run("Has bunch of logs", func(t *testing.T) {
		t.Parallel()

		numOfBlocks := 10
		numOfLogsPerBlock := 5

		store := newTestTrackerStore(t)

		for i := 1; i <= numOfBlocks; i++ {
			blockLogs := make([]*ethgo.Log, numOfLogsPerBlock)
			for j := 0; j < numOfLogsPerBlock; j++ {
				blockLogs[j] = createTestLogForStateSyncEvent(t, uint64(i), uint64(j))
			}

			require.NoError(t, store.InsertLogs(blockLogs))
		}

		// check if the num of blocks per each block matches expected values
		for i := 1; i <= numOfBlocks; i++ {
			logs, err := store.GetLogsByBlockNumber(uint64(i))
			require.NoError(t, err)
			require.Len(t, logs, numOfLogsPerBlock)
		}

		// get logs for non existing block
		logs, err := store.GetLogsByBlockNumber(uint64(numOfBlocks + 1))
		require.NoError(t, err)
		require.Nil(t, logs)

		// get specific logs
		for i := 1; i <= numOfBlocks; i++ {
			for j := 0; j < numOfLogsPerBlock; j++ {
				log, err := store.GetLog(uint64(i), uint64(j))
				require.NoError(t, err)
				require.NotNil(t, log)
				require.Equal(t, uint64(i), log.BlockNumber)
				require.Equal(t, uint64(j), log.LogIndex)
			}
		}

		// get some non existing logs
		log, err := store.GetLog(1, uint64(numOfLogsPerBlock+1))
		require.NoError(t, err)
		require.Nil(t, log)

		log, err = store.GetLog(uint64(numOfBlocks+1), uint64(numOfLogsPerBlock+1))
		require.NoError(t, err)
		require.Nil(t, log)
	})
}

func TestTrackerBlockContainer_GetConfirmedBlocks(t *testing.T) {
	t.Parallel()

	tbc := NewTrackerBlockContainer(0)

	t.Run("Number of blocks is greater than numBlockConfirmations", func(t *testing.T) {
		t.Parallel()

		tbc.blocks = []uint64{1, 2, 3, 4, 5}

		numBlockConfirmations := uint64(2)
		expected := []uint64{1, 2, 3}

		result := tbc.GetConfirmedBlocks(numBlockConfirmations)

		require.Equal(t, expected, result)
	})

	t.Run("Number of blocks is less or equal than numBlockConfirmations", func(t *testing.T) {
		t.Parallel()

		tbc.blocks = []uint64{1, 2, 3}
		numBlockConfirmations := uint64(3)

		result := tbc.GetConfirmedBlocks(numBlockConfirmations)

		require.Nil(t, result)
	})

	t.Run("numBlockConfirmations is 0", func(t *testing.T) {
		t.Parallel()

		tbc.blocks = []uint64{1, 2, 3}
		numBlockConfirmations := uint64(0)

		result := tbc.GetConfirmedBlocks(numBlockConfirmations)

		require.Equal(t, tbc.blocks, result)
	})

	t.Run("numBlockConfirmations is 1", func(t *testing.T) {
		t.Parallel()

		tbc.blocks = []uint64{1, 2, 3}

		numBlockConfirmations := uint64(1)
		expected := []uint64{1, 2}

		result := tbc.GetConfirmedBlocks(numBlockConfirmations)

		require.Equal(t, expected, result)
	})

	t.Run("No blocks cached", func(t *testing.T) {
		t.Parallel()

		tbc.blocks = []uint64{}

		numBlockConfirmations := uint64(0)

		result := tbc.GetConfirmedBlocks(numBlockConfirmations)

		require.Nil(t, result)
	})
}

func TestTrackerBlockContainer_IsOutOfSync(t *testing.T) {
	t.Parallel()

	t.Run("Block number greater than last cached block and parent hash matches", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		cachedBlock := &ethgo.Block{
			Number:     2,
			Hash:       ethgo.Hash{2},
			ParentHash: ethgo.Hash{1},
		}
		require.NoError(t, tbc.AddBlock(cachedBlock))

		latestBlock := &ethgo.Block{
			Number:     3,
			Hash:       ethgo.Hash{3},
			ParentHash: cachedBlock.Hash,
		}

		require.False(t, tbc.IsOutOfSync(latestBlock))
	})

	t.Run("Latest block number equal to 1 (start of chain)", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		require.False(t, tbc.IsOutOfSync(&ethgo.Block{Number: 1}))
	})

	t.Run("Block number greater than last cached block and parent hash does not match", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		cachedBlock := &ethgo.Block{
			Number:     2,
			Hash:       ethgo.Hash{2},
			ParentHash: ethgo.Hash{1},
		}
		require.NoError(t, tbc.AddBlock(cachedBlock))

		latestBlock := &ethgo.Block{
			Number:     3,
			Hash:       ethgo.Hash{3},
			ParentHash: ethgo.Hash{22}, // some other parent
		}

		require.True(t, tbc.IsOutOfSync(latestBlock))
	})

	t.Run("Block number less or equal to the last cached block", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		cachedBlock := &ethgo.Block{
			Number:     2,
			Hash:       ethgo.Hash{2},
			ParentHash: ethgo.Hash{1},
		}
		require.NoError(t, tbc.AddBlock(cachedBlock))

		require.False(t, tbc.IsOutOfSync(cachedBlock))
	})

	t.Run("Block number greater than the last cached block, and parent does not exist", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		cachedBlock := &ethgo.Block{
			Number:     12,
			Hash:       ethgo.Hash{2},
			ParentHash: ethgo.Hash{1},
		}
		require.NoError(t, tbc.AddBlock(cachedBlock))

		require.False(t, tbc.IsOutOfSync(cachedBlock))
	})
}

func TestTrackerBlockContainer_RemoveBlocks(t *testing.T) {
	t.Parallel()

	t.Run("Remove blocks from 'from' to 'last' index, and update lastProcessedConfirmedBlock", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 1, Hash: ethgo.Hash{1}, ParentHash: ethgo.Hash{0}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 2, Hash: ethgo.Hash{2}, ParentHash: ethgo.Hash{1}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 3, Hash: ethgo.Hash{3}, ParentHash: ethgo.Hash{2}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 4, Hash: ethgo.Hash{4}, ParentHash: ethgo.Hash{3}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 5, Hash: ethgo.Hash{5}, ParentHash: ethgo.Hash{4}}))

		require.NoError(t, tbc.RemoveBlocks(1, 3))

		// Check if the blocks and lastProcessedConfirmedBlock are updated correctly
		require.Equal(t, []uint64{4, 5}, tbc.blocks)
		require.Equal(t, uint64(3), tbc.lastProcessedConfirmedBlock)
	})

	t.Run("Remove blocks from 'from' to 'last' index, where 'from' is greater than 'last'", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 1, Hash: ethgo.Hash{1}, ParentHash: ethgo.Hash{0}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 2, Hash: ethgo.Hash{2}, ParentHash: ethgo.Hash{1}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 3, Hash: ethgo.Hash{3}, ParentHash: ethgo.Hash{2}}))

		require.ErrorContains(t, tbc.RemoveBlocks(3, 1), "greater than last block")
	})

	t.Run("Remove blocks from 'from' to 'last' index, where given already removed", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(30)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 21, Hash: ethgo.Hash{21}, ParentHash: ethgo.Hash{20}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 22, Hash: ethgo.Hash{22}, ParentHash: ethgo.Hash{21}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 23, Hash: ethgo.Hash{23}, ParentHash: ethgo.Hash{22}}))

		require.ErrorContains(t, tbc.RemoveBlocks(18, 19), "are already processed and removed")
	})

	t.Run("Remove blocks from 'from' to 'last' index, where last does not exist in cached blocks", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(10)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 11, Hash: ethgo.Hash{11}, ParentHash: ethgo.Hash{10}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 12, Hash: ethgo.Hash{12}, ParentHash: ethgo.Hash{11}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 13, Hash: ethgo.Hash{13}, ParentHash: ethgo.Hash{12}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 14, Hash: ethgo.Hash{14}, ParentHash: ethgo.Hash{13}}))

		require.ErrorContains(t, tbc.RemoveBlocks(11, 15), "could not find last block")
	})

	t.Run("Remove all blocks", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 1, Hash: ethgo.Hash{1}, ParentHash: ethgo.Hash{0}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 2, Hash: ethgo.Hash{2}, ParentHash: ethgo.Hash{1}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 3, Hash: ethgo.Hash{3}, ParentHash: ethgo.Hash{2}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 4, Hash: ethgo.Hash{4}, ParentHash: ethgo.Hash{3}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 5, Hash: ethgo.Hash{5}, ParentHash: ethgo.Hash{4}}))

		require.NoError(t, tbc.RemoveBlocks(1, 5))

		require.Empty(t, tbc.blocks)
		require.Equal(t, uint64(5), tbc.lastProcessedConfirmedBlock)
	})

	t.Run("Remove single block", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(0)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 1, Hash: ethgo.Hash{1}, ParentHash: ethgo.Hash{0}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 2, Hash: ethgo.Hash{2}, ParentHash: ethgo.Hash{1}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 3, Hash: ethgo.Hash{3}, ParentHash: ethgo.Hash{2}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 4, Hash: ethgo.Hash{4}, ParentHash: ethgo.Hash{3}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 5, Hash: ethgo.Hash{5}, ParentHash: ethgo.Hash{4}}))

		require.NoError(t, tbc.RemoveBlocks(1, 1))

		assert.Equal(t, []uint64{2, 3, 4, 5}, tbc.blocks)
		assert.Equal(t, uint64(1), tbc.lastProcessedConfirmedBlock)
	})

	t.Run("Try to do non-sequential removal of blocks", func(t *testing.T) {
		t.Parallel()

		tbc := NewTrackerBlockContainer(110)

		// Add some blocks to the container
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 111, Hash: ethgo.Hash{111}, ParentHash: ethgo.Hash{110}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 112, Hash: ethgo.Hash{112}, ParentHash: ethgo.Hash{111}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 113, Hash: ethgo.Hash{113}, ParentHash: ethgo.Hash{112}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 114, Hash: ethgo.Hash{114}, ParentHash: ethgo.Hash{113}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 115, Hash: ethgo.Hash{115}, ParentHash: ethgo.Hash{114}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 116, Hash: ethgo.Hash{116}, ParentHash: ethgo.Hash{115}}))
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: 117, Hash: ethgo.Hash{117}, ParentHash: ethgo.Hash{116}}))

		// Try to remove last 2 blocks without first removing first 3
		require.ErrorContains(t, tbc.RemoveBlocks(113, 115), "trying to do non-sequential removal")
	})
}

func TestTrackerBlockContainer_AddBlockAndLastCachedBlock(t *testing.T) {
	t.Parallel()

	numOfBlocks := 30
	tbc := NewTrackerBlockContainer(0)

	for i := uint64(1); i <= uint64(numOfBlocks); i++ {
		require.NoError(t, tbc.AddBlock(&ethgo.Block{Number: i, Hash: ethgo.Hash{byte(i)}, ParentHash: ethgo.Hash{byte(i - 1)}}))
		require.Equal(t, i, tbc.LastCachedBlock())
	}

	require.Len(t, tbc.blocks, numOfBlocks)
	require.Len(t, tbc.numToHashMap, numOfBlocks)

	for i := 0; i < numOfBlocks; i++ {
		require.Equal(t, tbc.blocks[i], uint64(i+1))
	}
}

func createTestLogForStateSyncEvent(t *testing.T, blockNumber, logIndex uint64) *ethgo.Log {
	t.Helper()

	var transferEvent contractsapi.StateSyncedEvent

	topics := make([]ethgo.Hash, 3)
	topics[0] = transferEvent.Sig()
	topics[1] = ethgo.BytesToHash(types.ZeroAddress.Bytes())
	topics[2] = ethgo.BytesToHash(types.ZeroAddress.Bytes())
	encodedData, err := abi.MustNewType("tuple(string a)").Encode([]string{"data"})
	require.NoError(t, err)

	return &ethgo.Log{
		BlockNumber: blockNumber,
		LogIndex:    logIndex,
		Address:     ethgo.ZeroAddress,
		Topics:      topics,
		Data:        encodedData,
	}
}
