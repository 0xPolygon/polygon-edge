package tracker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/tracker/store"
	bolt "go.etcd.io/bbolt"
)

var _ store.Store = (*EventTrackerStore)(nil)

var (
	dbLogs          = []byte("logs")
	dbConf          = []byte("conf")
	dbNextToProcess = []byte("nextToProcess")
)

// EventTrackerStore is a tracker store implementation.
type EventTrackerStore struct {
	conn               *bolt.DB
	finalizedThreshold uint64 // after how many blocks we consider block is finalized
	notifierCh         chan<- []*ethgo.Log
	logger             hcf.Logger
}

// NewEventTrackerStore creates a new EventTrackerStore
func NewEventTrackerStore(
	path string,
	finalizedTrashhold uint64,
	notifierCh chan<- []*ethgo.Log,
	logger hcf.Logger) (*EventTrackerStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	store := &EventTrackerStore{
		conn:               db,
		finalizedThreshold: finalizedTrashhold,
		notifierCh:         notifierCh,
		logger:             logger,
	}

	if err := store.setupDB(); err != nil {
		store.Close()

		return nil, err
	}

	return store, nil
}

func (b *EventTrackerStore) setupDB() error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(dbConf); err != nil {
			return err
		}

		return nil
	})
}

// Close implements the store interface
func (b *EventTrackerStore) Close() error {
	return b.conn.Close()
}

// Get implements the store interface
func (b *EventTrackerStore) Get(k string) (string, error) {
	var result []byte

	if err := b.conn.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(dbConf)
		result = bucket.Get([]byte(k))

		return nil
	}); err != nil {
		return "", err
	}

	return string(result), nil
}

// ListPrefix implements the store interface
func (b *EventTrackerStore) ListPrefix(prefix string) ([]string, error) {
	var result []string

	if err := b.conn.View(func(tx *bolt.Tx) error {
		pfx := []byte(prefix)
		c := tx.Bucket(dbConf).Cursor()

		for k, v := c.Seek(pfx); k != nil && bytes.HasPrefix(k, pfx); k, v = c.Next() {
			result = append(result, string(v))
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// Set implements the store interface
func (b *EventTrackerStore) Set(k, v string) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(dbConf)

		return bucket.Put([]byte(k), []byte(v))
	})
}

// GetEntry implements the store interface
func (b *EventTrackerStore) GetEntry(hash string) (store.Entry, error) {
	var result store.Entry

	if err := b.conn.Update(func(tx *bolt.Tx) error {
		logsBucketName := append(dbLogs, []byte(hash)...)
		if _, err := tx.CreateBucketIfNotExists(logsBucketName); err != nil {
			return err
		}

		nextToProcessBucketName := append(dbNextToProcess, []byte(hash)...)
		if _, err := tx.CreateBucketIfNotExists(nextToProcessBucketName); err != nil {
			return err
		}

		result = &Entry{
			conn:                b.conn,
			bucketLogs:          logsBucketName,
			bucketNextToProcess: nextToProcessBucketName,
			finalizedThreshold:  b.finalizedThreshold,
			notifierCh:          b.notifierCh,
			logger:              b.logger,
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// Entry is an store.Entry implementation
type Entry struct {
	conn                *bolt.DB
	bucketLogs          []byte
	bucketNextToProcess []byte
	finalizedThreshold  uint64 // after how many blocks we consider block is finalized
	notifierCh          chan<- []*ethgo.Log
	logger              hcf.Logger
}

// LastIndex implements the store interface
func (e *Entry) LastIndex() (uint64, error) {
	var result uint64

	if err := e.conn.View(func(tx *bolt.Tx) error {
		result = getLastIndex(tx.Bucket(e.bucketLogs))

		return nil
	}); err != nil {
		return 0, err
	}

	return result, nil
}

// StoreLog implements the store interface
func (e *Entry) StoreLog(log *ethgo.Log) error {
	return e.StoreLogs([]*ethgo.Log{log})
}

// StoreLogs implements the store interface
// logs are added in sequentional order
func (e *Entry) StoreLogs(logs []*ethgo.Log) error {
	if len(logs) == 0 {
		return nil
	}

	var lastBlockNumber uint64

	if err := e.conn.Update(func(tx *bolt.Tx) error {
		bucketLogs := tx.Bucket(e.bucketLogs)
		lastLogIndx := getLastIndex(bucketLogs)

		for idx, log := range logs {
			logIdx := lastLogIndx + uint64(idx)

			val, err := log.MarshalJSON()
			if err != nil {
				return err
			}

			if err := bucketLogs.Put(itob(logIdx), val); err != nil {
				return err
			}

			lastBlockNumber = log.BlockNumber
		}

		e.logger.Info("write event logs",
			"from", lastLogIndx, "to", lastLogIndx+uint64(len(logs))-1, "block", lastBlockNumber)

		return nil
	}); err != nil {
		return err
	}

	notifyLogs, lastProcessedIdx, err := e.getFinalizedLogs(lastBlockNumber - e.finalizedThreshold)
	if err != nil {
		return err
	}

	return e.notifyFinalizedLogs(notifyLogs, lastProcessedIdx)
}

// RemoveLogs implements the store interface
func (e *Entry) RemoveLogs(indx uint64) error {
	return e.conn.Update(func(tx *bolt.Tx) error {
		cursorLogs := tx.Bucket(e.bucketLogs).Cursor()

		// remove logs
		for k, _ := cursorLogs.Seek(itob(indx)); k != nil; k, _ = cursorLogs.Next() {
			if err := cursorLogs.Delete(); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetLog implements the store interface
func (e *Entry) GetLog(idx uint64, log *ethgo.Log) error {
	return e.conn.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(e.bucketLogs).Get(itob(idx))
		if val == nil {
			return fmt.Errorf("log not found: %d", idx)
		}

		return log.UnmarshalJSON(val)
	})
}

func (e *Entry) getFinalizedLogs(untilBlockNumber uint64) ([]*ethgo.Log, uint64, error) {
	var (
		logs             []*ethgo.Log
		lastProcessedIdx uint64 = 0
	)

	if err := e.conn.View(func(tx *bolt.Tx) error {
		bucketLastProcessedBlock := tx.Bucket(e.bucketNextToProcess)
		cursorLogs := tx.Bucket(e.bucketLogs).Cursor()

		var key, value []byte

		// pick first unprocessed block
		if _, val := bucketLastProcessedBlock.Cursor().First(); val != nil {
			key, value = cursorLogs.Seek(val)
		} else {
			key, value = cursorLogs.First()
		}

		for ; key != nil; key, value = cursorLogs.Next() {
			log := &ethgo.Log{}
			if err := json.Unmarshal(value, log); err != nil {
				return err
			}

			if log.BlockNumber > untilBlockNumber {
				break
			}

			logs = append(logs, log)
			lastProcessedIdx = btoi(key)
		}

		return nil
	}); err != nil {
		return nil, 0, err
	}

	return logs, lastProcessedIdx, nil
}

func (e *Entry) notifyFinalizedLogs(logs []*ethgo.Log, lastProcessedIdx uint64) error {
	if len(logs) == 0 {
		return nil
	}

	e.logger.Info("notify event logs", "len", len(logs), "last processed", lastProcessedIdx)

	if err := e.conn.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(e.bucketNextToProcess).Put([]byte("0"), itob(lastProcessedIdx+1))
	}); err != nil {
		return err
	}

	// notify logs - for testing purpose chan can be nil
	if e.notifierCh != nil {
		e.notifierCh <- logs
	}

	return nil
}

func getLastIndex(bucket *bolt.Bucket) uint64 {
	if last, _ := bucket.Cursor().Last(); last != nil {
		return btoi(last) + 1
	}

	return 0
}

func btoi(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func itob(u uint64) []byte {
	buf := [8]byte{}
	binary.BigEndian.PutUint64(buf[:], u)

	return buf[:]
}
