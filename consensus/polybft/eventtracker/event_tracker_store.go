package eventtracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"
)

var (
	petLastProcessedBlockKey    = []byte("lastProcessedTrackerBlock")
	petLastProcessedBlockBucket = []byte("lastProcessedTrackerBucket")
	petLogsBucket               = []byte("logs")
)

// EventTrackerStore is an interface that defines the behavior of an event tracker store
type EventTrackerStore interface {
	GetLastProcessedBlock() (uint64, error)
	InsertLastProcessedBlock(blockNumber uint64) error
	InsertLogs(logs []*ethgo.Log) error
	GetLogsByBlockNumber(blockNumber uint64) ([]*ethgo.Log, error)
	GetLog(blockNumber, logIndex uint64) (*ethgo.Log, error)
	GetAllLogs() ([]*ethgo.Log, error)
}

var _ EventTrackerStore = (*BoltDBEventTrackerStore)(nil)

// BoltDBEventTrackerStore represents a store for event tracker events
type BoltDBEventTrackerStore struct {
	db *bolt.DB
}

// NewBoltDBEventTrackerStore is a constructor function that creates
// a new instance of the BoltDBEventTrackerStore struct.
//
// Example Usage:
//
//	t := NewBoltDBEventTrackerStore(/edge/polybft/consensus/deposit.db)
//
// Inputs:
//   - dbPath (string): Full path to the event tracker store db.
//
// Outputs:
//   - A new instance of the BoltDBEventTrackerStore struct with a connection to the event tracker store db.
func NewBoltDBEventTrackerStore(dbPath string) (*BoltDBEventTrackerStore, error) {
	db, err := bolt.Open(dbPath, 0666, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(petLastProcessedBlockBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(petLogsBucket)

		return err
	})
	if err != nil {
		return nil, err
	}

	return &BoltDBEventTrackerStore{db: db}, nil
}

// GetLastProcessedBlock retrieves the last processed block number from a BoltDB database.
//
// Example Usage:
//
//	store := NewBoltDBEventTrackerStore(db)
//	blockNumber, err := store.GetLastProcessedBlock()
//	if err != nil {
//	  fmt.Println("Error:", err)
//	} else {
//	  fmt.Println("Last processed block number:", blockNumber)
//	}
//
// Outputs:
//
//	blockNumber: The last processed block number retrieved from the database.
//	err: Any error that occurred during the database operation.
func (p *BoltDBEventTrackerStore) GetLastProcessedBlock() (uint64, error) {
	var blockNumber uint64

	err := p.db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket(petLastProcessedBlockBucket).Get(petLastProcessedBlockKey)
		if value != nil {
			blockNumber = common.EncodeBytesToUint64(value)
		}

		return nil
	})

	return blockNumber, err
}

// InsertLastProcessedBlock inserts the last processed block number into a BoltDB bucket.
//
// Inputs:
// - lastProcessedBlockNumber (uint64): The block number to be inserted into the bucket.
//
// Outputs:
// - error: An error indicating if there was a problem with the transaction or the insertion.
func (p *BoltDBEventTrackerStore) InsertLastProcessedBlock(lastProcessedBlockNumber uint64) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(petLastProcessedBlockBucket).Put(
			petLastProcessedBlockKey, common.EncodeUint64ToBytes(lastProcessedBlockNumber))
	})
}

// InsertLogs inserts logs into a BoltDB database, where logs are stored by a composite key:
// - {log.BlockNumber,log.LogIndex}
//
// Example Usage:
//
//	store := &BoltDBEventTrackerStore{db: boltDB}
//	logs := []*ethgo.Log{log1, log2, log3}
//	err := store.InsertLogs(logs)
//	if err != nil {
//	  fmt.Println("Error inserting logs:", err)
//	}
//
// Inputs:
//   - logs: A slice of ethgo.Log structs representing the logs to be inserted into the database.
//
// Outputs:
//   - error: If an error occurs during the insertion process, it is returned. Otherwise, nil is returned.
func (p *BoltDBEventTrackerStore) InsertLogs(logs []*ethgo.Log) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(petLogsBucket)
		for _, log := range logs {
			raw, err := json.Marshal(log)
			if err != nil {
				return err
			}

			logKey := bytes.Join([][]byte{
				common.EncodeUint64ToBytes(log.BlockNumber),
				common.EncodeUint64ToBytes(log.LogIndex)}, nil)
			if err := bucket.Put(logKey, raw); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetLogsByBlockNumber retrieves all logs that happened in given block from a BoltDB database.
//
// Example Usage:
//
//		store := &BoltDBEventTrackerStore{db: boltDB}
//	 	block := uint64(10)
//		logs, err := store.GetLogsByBlockNumber(block)
//		if err != nil {
//		  fmt.Println("Error getting logs for block: %d. Err: %w", block, err)
//		}
//
// Inputs:
// - blockNumber (uint64): The block number for which the logs need to be retrieved.
//
// Outputs:
// - logs ([]*ethgo.Log): The logs retrieved from the database for the given block number.
// - err (error): Any error that occurred during the transaction or unmarshaling process.
func (p *BoltDBEventTrackerStore) GetLogsByBlockNumber(blockNumber uint64) ([]*ethgo.Log, error) {
	var logs []*ethgo.Log

	err := p.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(petLogsBucket).Cursor()
		prefix := common.EncodeUint64ToBytes(blockNumber)

		for k, v := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var log *ethgo.Log
			if err := json.Unmarshal(v, &log); err != nil {
				return err
			}

			logs = append(logs, log)
		}

		return nil
	})

	return logs, err
}

// GetLog retrieves a log from the BoltDB database based on the given block number and log index.
//
// Example Usage:
//
//		store := &BoltDBEventTrackerStore{db: boltDB}
//	 	block := uint64(10)
//		logIndex := uint64(1)
//		log, err := store.GetLog(block, logIndex)
//		if err != nil {
//		  fmt.Println("Error getting log of index: %d for block: %d. Err: %w", logIndex, block, err)
//		}
//
// Inputs:
//   - blockNumber (uint64): The block number of the desired log.
//   - logIndex (uint64): The index of the desired log within the block.
//
// Outputs:
//   - log (*ethgo.Log): The retrieved log from the BoltDB database. If the log does not exist, it will be nil.
//   - err (error): Any error that occurred during the database operation. If no error occurred, it will be nil.
func (p *BoltDBEventTrackerStore) GetLog(blockNumber, logIndex uint64) (*ethgo.Log, error) {
	var log *ethgo.Log

	err := p.db.View(func(tx *bolt.Tx) error {
		logKey := bytes.Join([][]byte{
			common.EncodeUint64ToBytes(blockNumber),
			common.EncodeUint64ToBytes(logIndex)}, nil)

		val := tx.Bucket(petLogsBucket).Get(logKey)
		if val == nil {
			return nil
		}

		return json.Unmarshal(val, &log)
	})

	return log, err
}

// GetAllLogs retrieves all logs from the logs bucket in the BoltDB database and
// returns them as a slice of ethgo.Log structs.
//
// Example Usage:
// store := NewBoltDBEventTrackerStore("path/to/db")
// logs, err := store.GetAllLogs()
//
//	if err != nil {
//	    fmt.Println("Error:", err)
//	    return
//	}
//
//	for _, log := range logs {
//	    fmt.Println(log)
//	}
//
// Outputs:
// The code snippet returns a slice of ethgo.Log structs (logs) and an error (err).
// The logs slice contains all the logs stored in the logs bucket in the BoltDB database.
// The error will be non-nil if there was an issue with the read transaction or unmarshaling the log structs.
func (p *BoltDBEventTrackerStore) GetAllLogs() ([]*ethgo.Log, error) {
	var logs []*ethgo.Log

	err := p.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(petLogsBucket).ForEach(func(k, v []byte) error {
			var log *ethgo.Log
			if err := json.Unmarshal(v, &log); err != nil {
				return err
			}

			logs = append(logs, log)

			return nil
		})
	})

	return logs, err
}

// TrackerBlockContainer is a struct used to cache and manage tracked blocks from tracked chain.
// It keeps a map of block numbers to hashes, a slice of block numbers to process,
// and the last processed confirmed block number.
// It also uses a mutex to handle concurrent access to the struct
type TrackerBlockContainer struct {
	numToHashMap                map[uint64]ethgo.Hash
	blocks                      []uint64
	lastProcessedConfirmedBlock uint64
	mux                         sync.RWMutex
}

// NewTrackerBlockContainer is a constructor function that creates a
// new instance of the TrackerBlockContainer struct.
//
// Example Usage:
//
//	t := NewTrackerBlockContainer(1)
//
// Inputs:
//   - lastProcessed (uint64): The last processed block number.
//
// Outputs:
//   - A new instance of the TrackerBlockContainer struct with the lastProcessedConfirmedBlock
//     field set to the input lastProcessed block number and an empty numToHashMap map.
func NewTrackerBlockContainer(lastProcessed uint64) *TrackerBlockContainer {
	return &TrackerBlockContainer{
		lastProcessedConfirmedBlock: lastProcessed,
		numToHashMap:                make(map[uint64]ethgo.Hash),
	}
}

// AcquireWriteLock acquires the write lock on the TrackerBlockContainer
func (t *TrackerBlockContainer) AcquireWriteLock() {
	t.mux.Lock()
}

// ReleaseWriteLock releases the write lock on the TrackerBlockContainer
func (t *TrackerBlockContainer) ReleaseWriteLock() {
	t.mux.Unlock()
}

// LastProcessedBlockLocked returns number of last processed block for logs
// Function acquires the read lock before accessing the lastProcessedConfirmedBlock field
func (t *TrackerBlockContainer) LastProcessedBlock() uint64 {
	t.mux.RLock()
	defer t.mux.RUnlock()

	return t.LastProcessedBlockLocked()
}

// LastProcessedBlockLocked returns number of last processed block for logs
// Function assumes that the read or write lock is already acquired before accessing
// the lastProcessedConfirmedBlock field
func (t *TrackerBlockContainer) LastProcessedBlockLocked() uint64 {
	return t.lastProcessedConfirmedBlock
}

// LastCachedBlock returns the block number of the last cached block for processing
//
// Example Usage:
//
//	t := NewTrackerBlockContainer(1)
//	t.AddBlock(&ethgo.Block{Number: 1, Hash: "hash1"})
//	t.AddBlock(&ethgo.Block{Number: 2, Hash: "hash2"})
//	t.AddBlock(&ethgo.Block{Number: 3, Hash: "hash3"})
//
//	lastCachedBlock := t.LastCachedBlock()
//	fmt.Println(lastCachedBlock) // Output: 3
//
// Outputs:
//
//	The output is a uint64 value representing the block number of the last cached block.
func (t *TrackerBlockContainer) LastCachedBlock() uint64 {
	if len(t.blocks) > 0 {
		return t.blocks[len(t.blocks)-1]
	}

	return 0
}

// AddBlock adds a new block to the tracker by storing its number and hash in the numToHashMap map
// and appending the block number to the blocks slice.
//
// Inputs:
//   - block (ethgo.Block): The block to be added to the tracker cache for later processing,
//     once it hits confirmation number.
func (t *TrackerBlockContainer) AddBlock(block *ethgo.Block) error {
	if hash, exists := t.numToHashMap[block.Number-1]; len(t.blocks) > 0 && (!exists || block.ParentHash != hash) {
		return fmt.Errorf("no parent for block %d, or a reorg happened", block.Number)
	}

	t.numToHashMap[block.Number] = block.Hash
	t.blocks = append(t.blocks, block.Number)

	return nil
}

// RemoveBlocks removes processed blocks from cached maps,
// and updates the lastProcessedConfirmedBlock variable, to the last processed block.
//
// Inputs:
// - from (uint64): The starting block number to remove.
// - last (uint64): The ending block number to remove.
//
// Returns:
//   - nil if removal is successful
//   - An error if from block is greater than the last, if given range of blocks was already processed and removed,
//     if the last block could not be found in cached blocks, or if we are trying to do a non sequential removal
func (t *TrackerBlockContainer) RemoveBlocks(from, last uint64) error {
	if from > last {
		return fmt.Errorf("from block: %d, greater than last block: %d", from, last)
	}

	if last < t.lastProcessedConfirmedBlock {
		return fmt.Errorf("blocks until block: %d are already processed and removed", last)
	}

	lastIndex := t.indexOf(last)
	if lastIndex == -1 {
		return fmt.Errorf("could not find last block: %d in cached blocks", last)
	}

	removedBlocks := t.blocks[:lastIndex+1]
	remainingBlocks := t.blocks[lastIndex+1:]

	if removedBlocks[0] != from {
		return fmt.Errorf("trying to do non-sequential removal of blocks. from: %d, last: %d", from, last)
	}

	for i := from; i <= last; i++ {
		delete(t.numToHashMap, i)
	}

	t.blocks = remainingBlocks
	t.lastProcessedConfirmedBlock = last

	return nil
}

// CleanState resets the state of the TrackerBlockContainer
// by clearing the numToHashMap map and setting the blocks slice to an empty slice
// Called when a reorg happened or we are completely out of sync
func (t *TrackerBlockContainer) CleanState() {
	t.numToHashMap = make(map[uint64]ethgo.Hash)
	t.blocks = make([]uint64, 0)
}

// IsOutOfSync checks if tracker is out of sync with the tracked chain.
// Tracker is out of sync with the tracked chain if these conditions are met:
//   - latest block from chain has higher number than the last tracked block
//   - its parent doesn't exist in numToHash map
//   - its parent hash doesn't match with the hash of the given parent block we tracked,
//     meaning, a reorg happened
//
// Inputs:
// - block (ethgo.Block): The latest block of the tracked chain.
//
// Outputs:
// - outOfSync (bool): A boolean value indicating if the tracker is out of sync (true) or not (false).
func (t *TrackerBlockContainer) IsOutOfSync(block *ethgo.Block) bool {
	t.mux.RLock()
	defer t.mux.RUnlock()

	if block.Number == 1 {
		// if the chain we are tracking just started
		return false
	}

	parentHash, parentExists := t.numToHashMap[block.Number-1]

	return block.Number > t.LastCachedBlock() && (!parentExists || parentHash != block.ParentHash)
}

// GetConfirmedBlocks returns a slice of uint64 representing the block numbers of confirmed blocks.
//
// Example Usage:
//
//	t := NewTrackerBlockContainer(2)
//	t.AddBlock(&ethgo.Block{Number: 1, Hash: "hash1"})
//	t.AddBlock(&ethgo.Block{Number: 2, Hash: "hash2"})
//	t.AddBlock(&ethgo.Block{Number: 3, Hash: "hash3"})
//
//	confirmedBlocks := t.GetConfirmedBlocks(2)
//	fmt.Println(confirmedBlocks) // Output: [1]
//
// Inputs:
//   - numBlockConfirmations (uint64): The number of block confirmations to consider.
//
// Flow:
//  1. Convert numBlockConfirmations to an integer numBlockConfirmationsInt.
//  2. Check if the length of t.blocks (slice of block numbers) is greater than numBlockConfirmationsInt.
//  3. If it is, return a sub-slice of t.blocks from the beginning to the length of
//     t.blocks minus numBlockConfirmationsInt.
//  4. If it is not, return nil.
//
// Outputs:
//   - A slice of uint64 representing the block numbers of confirmed blocks.
func (t *TrackerBlockContainer) GetConfirmedBlocks(numBlockConfirmations uint64) []uint64 {
	numBlockConfirmationsInt := int(numBlockConfirmations)
	if len(t.blocks) > numBlockConfirmationsInt {
		return t.blocks[:len(t.blocks)-numBlockConfirmationsInt]
	}

	return nil
}

// indexOf returns the index of a given block number in the blocks slice.
// If the block number is not found, it returns -1
func (t *TrackerBlockContainer) indexOf(block uint64) int {
	index := -1

	for i, b := range t.blocks {
		if b == block {
			index = i

			break
		}
	}

	return index
}
