package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

// AATxState defines the interface for a stateful representation of Account Abstraction (AA) transactions
type AATxState interface {
	// Add adds a new AA transaction to the state (database) and returns a wrapper object
	Add(*AATransaction) (*AAStateTransaction, error)
	// Get retrieves the metadata for the AA transaction with the specified ID from the state
	Get(string) (*AAStateTransaction, error)
	// Get all pending transactions
	GetAllPending() ([]*AAStateTransaction, error)
	// Update modifies the metadata for the AA transaction with the specified ID in the state
	Update(string, func(tx *AAStateTransaction) error) error
}

var (
	pendingBucket  = []byte("pending")
	queuedBucket   = []byte("queued")
	finishedBucket = []byte("finished")
)

var _ AATxState = (*aaTxState)(nil)

type aaTxState struct {
	db *bolt.DB
}

func NewAATxState(dbFilePath string) (*aaTxState, error) {
	state := &aaTxState{}

	if err := state.init(dbFilePath); err != nil {
		return nil, err
	}

	return state, nil
}

func (s *aaTxState) Add(tx *AATransaction) (*AAStateTransaction, error) {
	ntx := &AAStateTransaction{
		ID:     uuid.NewString(),
		Tx:     tx,
		Status: StatusPending,
		Time:   time.Now().Unix(),
	}

	value, err := json.Marshal(ntx)
	if err != nil {
		return nil, err
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(pendingBucket).Put([]byte(ntx.ID), value)
	}); err != nil {
		return nil, err
	}

	return ntx, nil
}

func (s *aaTxState) Get(id string) (result *AAStateTransaction, err error) {
	if err := s.db.View(func(tx *bolt.Tx) error {
		value := s.getSerialized(tx, id)
		if value == nil {
			return nil
		}

		return json.Unmarshal(value, &result)
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *aaTxState) GetAllPending() ([]*AAStateTransaction, error) {
	return s.getAllFromBucket(pendingBucket)
}

func (s *aaTxState) Update(id string, fn func(tx *AAStateTransaction) error) error {
	statusToBucketMap := map[string][]byte{
		StatusPending:   pendingBucket,
		StatusQueued:    queuedBucket,
		StatusCompleted: finishedBucket,
		StatusFailed:    finishedBucket,
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		value := s.getSerialized(tx, id)
		if value == nil {
			return fmt.Errorf("tx with uuid = %s does not exist", id)
		}

		var stateTx AAStateTransaction

		if err := json.Unmarshal(value, &stateTx); err != nil {
			return err
		}

		oldStatusBucket := statusToBucketMap[stateTx.Status]

		if err := fn(&stateTx); err != nil {
			return err
		}

		newStatusBacket := statusToBucketMap[stateTx.Status]

		// if item changed bucket, remove it from old one
		if !bytes.Equal(newStatusBacket, oldStatusBucket) {
			tx.Bucket(oldStatusBucket).Delete([]byte(id))
		}

		bytesStateTx, err := json.Marshal(stateTx)
		if err != nil {
			return err
		}

		// put new value into new backet. Overwrite if already exists
		return tx.Bucket(newStatusBacket).Put([]byte(id), bytesStateTx)
	})
}

func (s *aaTxState) getSerialized(tx *bolt.Tx, id string) []byte {
	idb := []byte(id)

	for _, bucket := range [][]byte{pendingBucket, queuedBucket, finishedBucket} {
		value := tx.Bucket(bucket).Get(idb)
		if value != nil {
			return value
		}
	}

	return nil
}

func (s *aaTxState) init(dbFilePath string) (err error) {
	if s.db, err = bolt.Open(dbFilePath, 0666, nil); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{pendingBucket, queuedBucket, finishedBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *aaTxState) getAllFromBucket(bucketName []byte) ([]*AAStateTransaction, error) {
	var (
		result  []*AAStateTransaction
		stateTx AAStateTransaction
	)

	if err := s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bucketName).Cursor()

		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			if err := json.Unmarshal(value, &stateTx); err != nil {
				return err
			}

			result = append(result, &stateTx)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}
