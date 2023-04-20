package service

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
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
	// Get all queued transactions
	GetAllQueued() ([]*AAStateTransaction, error)
	// Get all sent transactions
	GetAllSent() ([]*AAStateTransaction, error)
	// Update modifies the metadata for the AA transaction
	Update(stateTx *AAStateTransaction) error
	// GetNonce get latest saved nonce
	GetNonce() (uint64, error)
	// UpdateNonce updates latest nonce
	UpdateNonce(nonce uint64) error
}

var (
	pendingBucket  = []byte("pending")
	queuedBucket   = []byte("queued")
	sentBucket     = []byte("sent")
	finishedBucket = []byte("finished")
	nonceBucket    = []byte("nonce")

	allBuckets        = [][]byte{pendingBucket, queuedBucket, sentBucket, finishedBucket}
	statusToBucketMap = map[string][]byte{
		StatusPending:   pendingBucket,
		StatusQueued:    queuedBucket,
		StatusSent:      sentBucket,
		StatusCompleted: finishedBucket,
		StatusFailed:    finishedBucket,
	}
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
		Time:   time.Now().UTC().Unix(),
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
		idb := []byte(id)

		for _, bucket := range allBuckets {
			value := tx.Bucket(bucket).Get(idb)
			if value != nil {
				return json.Unmarshal(value, &result)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *aaTxState) GetAllPending() ([]*AAStateTransaction, error) {
	return s.getAllFromBucket(pendingBucket)
}

func (s *aaTxState) GetAllQueued() ([]*AAStateTransaction, error) {
	return s.getAllFromBucket(queuedBucket)
}

func (s *aaTxState) GetAllSent() ([]*AAStateTransaction, error) {
	return s.getAllFromBucket(sentBucket)
}

func (s *aaTxState) Update(stateTx *AAStateTransaction) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		idb := []byte(stateTx.ID)
		newStatusBacket := statusToBucketMap[stateTx.Status]

		// check if item exist in another bucket, and if it is, delete item from the old bucket
		for _, bucket := range allBuckets {
			if !bytes.Equal(newStatusBacket, bucket) {
				if value := tx.Bucket(bucket).Get(idb); value != nil {
					// delete item from old bucket
					if err := tx.Bucket(bucket).Delete(idb); err != nil {
						return err
					}

					// update time
					switch stateTx.Status {
					case StatusQueued:
						stateTx.TimeQueued = time.Now().UTC().Unix()
					case StatusCompleted, StatusFailed:
						stateTx.TimeFinished = time.Now().UTC().Unix()
					}

					break
				}
			}
		}

		bytesStateTx, err := json.Marshal(stateTx)
		if err != nil {
			return err
		}

		// put new value into new backet. Overwrite if already exists
		return tx.Bucket(newStatusBacket).Put(idb, bytesStateTx)
	})
}

func (s *aaTxState) GetNonce() (nonce uint64, err error) {
	if err = s.db.View(func(tx *bolt.Tx) error {
		bytes := tx.Bucket(nonceBucket).Get([]byte{0})
		if bytes == nil {
			nonce = 0
		} else {
			nonce = common.EncodeBytesToUint64(bytes)
		}

		return nil
	}); err != nil {
		return 0, err
	}

	return nonce, nil
}

func (s *aaTxState) UpdateNonce(nonce uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(nonceBucket).Put([]byte{0}, common.EncodeUint64ToBytes(nonce))
	})
}

func (s *aaTxState) init(dbFilePath string) (err error) {
	if s.db, err = bolt.Open(dbFilePath, 0666, nil); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}

		if _, err := tx.CreateBucketIfNotExists(nonceBucket); err != nil {
			return err
		}

		return nil
	})
}

func (s *aaTxState) getAllFromBucket(bucketName []byte) ([]*AAStateTransaction, error) {
	var result []*AAStateTransaction

	if err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).ForEach(func(key, value []byte) error {
			stateTx := &AAStateTransaction{}

			if err := json.Unmarshal(value, stateTx); err != nil {
				return err
			}

			result = append(result, stateTx)

			return nil
		})
	}); err != nil {
		return nil, err
	}

	return result, nil
}
