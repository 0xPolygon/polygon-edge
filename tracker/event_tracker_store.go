package tracker

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/common"
	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/tracker/store"
	bolt "go.etcd.io/bbolt"
)

const dbLastBlockPrefix = "lastBlock_"

var (
	_ store.Store = (*EventTrackerStore)(nil)

	dbLogs           = []byte("logs")
	dbConf           = []byte("conf")
	dbNextToProcess  = []byte("nextToProcess")
	nextToProcessKey = []byte("0")
)

// EventTrackerStore is a tracker store implementation.
type EventTrackerStore struct {
	conn                  *bolt.DB
	numBlockConfirmations uint64
	subscriber            eventSubscription
	logger                hcf.Logger
}

// NewEventTrackerStore creates a new EventTrackerStore
func NewEventTrackerStore(
	path string,
	numBlockConfirmations uint64,
	subscriber eventSubscription,
	logger hcf.Logger) (*EventTrackerStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	store := &EventTrackerStore{
		conn:                  db,
		numBlockConfirmations: numBlockConfirmations,
		subscriber:            subscriber,
		logger:                logger,
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
	if err := b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(dbConf)

		return bucket.Put([]byte(k), []byte(v))
	}); err != nil {
		return err
	}

	if strings.HasPrefix(k, dbLastBlockPrefix) {
		if err := b.onNewBlock(k[len(dbLastBlockPrefix):], v); err != nil {
			b.logger.Warn("new block error", "err", err)

			return err
		}
	}

	return nil
}

func (b *EventTrackerStore) onNewBlock(filterHash, blockData string) error {
	var block ethgo.Block

	bytes, err := hex.DecodeString(blockData)
	if err != nil {
		return err
	}

	if err := block.UnmarshalJSON(bytes); err != nil {
		return err
	}

	if block.Number <= b.numBlockConfirmations {
		return nil // there is nothing to process yet
	}

	entry, err := b.getImplEntry(filterHash)
	if err != nil {
		return nil
	}

	logs, lastProcessedKey, err := entry.getFinalizedLogs(block.Number - b.numBlockConfirmations)
	if err != nil {
		return err
	}

	if len(logs) == 0 {
		return nil // nothing to process
	}

	// notify subscriber with logs
	for _, log := range logs {
		if err := b.subscriber.AddLog(log); err != nil {
			return err
		}
	}

	// save next to process only if every AddLog finished successfully
	nextToProcessIdx := common.EncodeBytesToUint64(lastProcessedKey) + 1
	if err := entry.saveNextToProcessIndx(nextToProcessIdx); err != nil {
		return err
	}

	b.logger.Debug("Event logs have been notified to a subscriber", "len", len(logs), "next", nextToProcessIdx)

	return nil
}

// GetEntry implements the store interface
func (b *EventTrackerStore) GetEntry(hash string) (store.Entry, error) {
	return b.getImplEntry(hash)
}

func (b *EventTrackerStore) getImplEntry(hash string) (*Entry, error) {
	logsBucketName := append(dbLogs, []byte(hash)...)
	nextToProcessBucketName := append(dbNextToProcess, []byte(hash)...)

	if err := b.conn.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(logsBucketName); err != nil {
			return err
		}

		if _, err := tx.CreateBucketIfNotExists(nextToProcessBucketName); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &Entry{
		conn:                b.conn,
		bucketLogs:          logsBucketName,
		bucketNextToProcess: nextToProcessBucketName,
	}, nil
}

// Entry is an store.Entry implementation
type Entry struct {
	conn                *bolt.DB
	bucketLogs          []byte
	bucketNextToProcess []byte
}

// LastIndex implements the store.Entry interface
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

// StoreLog implements the store.Entry interface
func (e *Entry) StoreLog(log *ethgo.Log) error {
	return e.StoreLogs([]*ethgo.Log{log})
}

// StoreLogs implements the store.Entry interface
// logs are added in sequentional order
func (e *Entry) StoreLogs(logs []*ethgo.Log) error {
	if len(logs) == 0 { // dont start tx if there is nothing to add
		return nil
	}

	return e.conn.Update(func(tx *bolt.Tx) error {
		bucketLogs := tx.Bucket(e.bucketLogs)
		logFirstIndx := getLastIndex(bucketLogs)

		for idx, log := range logs {
			logIdx := logFirstIndx + uint64(idx)

			val, err := log.MarshalJSON()
			if err != nil {
				return err
			}

			if err := bucketLogs.Put(common.EncodeUint64ToBytes(logIdx), val); err != nil {
				return err
			}
		}

		return nil
	})
}

// RemoveLogs implements the store.Entry interface
func (e *Entry) RemoveLogs(indx uint64) error {
	return e.conn.Update(func(tx *bolt.Tx) error {
		cursorLogs := tx.Bucket(e.bucketLogs).Cursor()

		// remove logs
		for k, _ := cursorLogs.Seek(common.EncodeUint64ToBytes(indx)); k != nil; k, _ = cursorLogs.Next() {
			if err := cursorLogs.Delete(); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetLog implements the store.Entry interface
func (e *Entry) GetLog(idx uint64, log *ethgo.Log) error {
	return e.conn.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(e.bucketLogs).Get(common.EncodeUint64ToBytes(idx))
		if val == nil {
			return fmt.Errorf("log not found: %d", idx)
		}

		return log.UnmarshalJSON(val)
	})
}

func (e *Entry) getFinalizedLogs(untilBlockNumber uint64) ([]*ethgo.Log, []byte, error) {
	var (
		logs             []*ethgo.Log
		lastProcessedKey []byte
		key, value       []byte
	)

	if err := e.conn.View(func(tx *bolt.Tx) error {
		bucketLastProcessedBlock := tx.Bucket(e.bucketNextToProcess)
		cursorLogs := tx.Bucket(e.bucketLogs).Cursor()

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
			lastProcessedKey = key
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	return logs, lastProcessedKey, nil
}

func (e *Entry) saveNextToProcessIndx(nextToProcessIdx uint64) error {
	return e.conn.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(e.bucketNextToProcess).Put(nextToProcessKey, common.EncodeUint64ToBytes(nextToProcessIdx))
	})
}

func getLastIndex(bucket *bolt.Bucket) uint64 {
	if last, _ := bucket.Cursor().Last(); last != nil {
		return common.EncodeBytesToUint64(last) + 1
	}

	return 0
}
