package polybft

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/helper/common"
	bolt "go.etcd.io/bbolt"
)

const (
	// validatorSnapshotLimit defines a maximum number of validator snapshots
	// that can be stored in cache (both memory and db)
	validatorSnapshotLimit = 100
	// numberOfSnapshotsToLeaveInMemory defines a number of validator snapshots to leave in memory
	numberOfSnapshotsToLeaveInMemory = 12
	// numberOfSnapshotsToLeaveInDB defines a number of validator snapshots to leave in db
	numberOfSnapshotsToLeaveInDB = 20
)

var (
	// bucket to store epochs and all its nested buckets (message votes and message pool events)
	epochsBucket = []byte("epochs")

	// bucket to store validator snapshots
	validatorSnapshotsBucket = []byte("validatorSnapshots")
)

/*
Bolt DB schema:

epochs/
|--> epochNumber
	|--> hash -> []*MessageSignatures (json marshalled)

epochs/
|--> epochNumber -> []*TransferEvent (json marshalled)

validatorSnapshots/
|--> epochNumber -> *AccountSet (json marshalled)
*/

type EpochStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *EpochStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(epochsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(epochsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(validatorSnapshotsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(validatorSnapshotsBucket), err)
	}

	return nil
}

// insertValidatorSnapshot inserts a validator snapshot for the given block to its bucket in db
func (s *EpochStore) insertValidatorSnapshot(validatorSnapshot *validatorSnapshot, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := json.Marshal(validatorSnapshot)
		if err != nil {
			return err
		}

		return tx.Bucket(validatorSnapshotsBucket).Put(common.EncodeUint64ToBytes(validatorSnapshot.Epoch), raw)
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getValidatorSnapshot queries the validator snapshot for given block from db
func (s *EpochStore) getValidatorSnapshot(epoch uint64) (*validatorSnapshot, error) {
	var validatorSnapshot *validatorSnapshot

	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(validatorSnapshotsBucket).Get(common.EncodeUint64ToBytes(epoch))
		if v != nil {
			return json.Unmarshal(v, &validatorSnapshot)
		}

		return nil
	})

	return validatorSnapshot, err
}

// getLastSnapshot returns the last snapshot saved in db
// since they are stored by epoch number (uint64), they are sequentially stored,
// so the latest epoch will be the last snapshot in db
func (s *EpochStore) getLastSnapshot(dbTx *bolt.Tx) (*validatorSnapshot, error) {
	var (
		snapshot *validatorSnapshot
		err      error
	)

	getFn := func(tx *bolt.Tx) error {
		c := tx.Bucket(validatorSnapshotsBucket).Cursor()
		k, v := c.Last()

		if k == nil {
			// we have no snapshots in db
			return nil
		}

		return json.Unmarshal(v, &snapshot)
	}

	if dbTx == nil {
		err = s.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

	return snapshot, err
}

// insertEpoch inserts a new epoch to db with its meta data
func (s *EpochStore) insertEpoch(epoch uint64, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		epochBucket, err := tx.Bucket(epochsBucket).CreateBucketIfNotExists(common.EncodeUint64ToBytes(epoch))
		if err != nil {
			return err
		}

		_, err = epochBucket.CreateBucketIfNotExists(messageVotesBucket)
		if err != nil {
			return err
		}

		return err
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// isEpochInserted checks if given epoch is present in db
func (s *EpochStore) isEpochInserted(epoch uint64) bool {
	return s.db.View(func(tx *bolt.Tx) error {
		_, err := getEpochBucket(tx, epoch)

		return err
	}) == nil
}

// getEpochBucket returns bucket from db associated with given epoch
func getEpochBucket(tx *bolt.Tx, epoch uint64) (*bolt.Bucket, error) {
	epochBucket := tx.Bucket(epochsBucket).Bucket(common.EncodeUint64ToBytes(epoch))
	if epochBucket == nil {
		return nil, fmt.Errorf("could not find bucket for epoch: %v", epoch)
	}

	return epochBucket, nil
}

// cleanEpochsFromDB cleans epoch buckets from db
func (s *EpochStore) cleanEpochsFromDB(dbTx *bolt.Tx) error {
	cleanFn := func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(epochsBucket); err != nil {
			return err
		}

		_, err := tx.CreateBucket(epochsBucket)

		return err
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return cleanFn(tx)
		})
	}

	return cleanFn(dbTx)
}

// cleanValidatorSnapshotsFromDB cleans the validator snapshots bucket if a limit is reached,
// but it leaves the latest (n) number of snapshots
func (s *EpochStore) cleanValidatorSnapshotsFromDB(epoch uint64, dbTx *bolt.Tx) error {
	cleanFn := func(tx *bolt.Tx) error {
		bucket := tx.Bucket(validatorSnapshotsBucket)

		// paired list
		keys := make([][]byte, 0)
		values := make([][]byte, 0)

		for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ { // exclude the last inserted we already appended
			key := common.EncodeUint64ToBytes(epoch)
			value := bucket.Get(key)

			if value == nil {
				continue
			}

			keys = append(keys, key)
			values = append(values, value)
			epoch--
		}

		// removing an entire bucket is much faster than removing all keys
		// look at thread https://github.com/boltdb/bolt/issues/667
		err := tx.DeleteBucket(validatorSnapshotsBucket)
		if err != nil {
			return err
		}

		bucket, err = tx.CreateBucket(validatorSnapshotsBucket)
		if err != nil {
			return err
		}

		// we start the loop in reverse so that the oldest of snapshots get inserted first in db
		for i := len(keys) - 1; i >= 0; i-- {
			if err := bucket.Put(keys[i], values[i]); err != nil {
				return err
			}
		}

		return nil
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return cleanFn(tx)
		})
	}

	return cleanFn(dbTx)
}

// removeAllValidatorSnapshots drops a validator snapshot bucket and re-creates it in bolt database
func (s *EpochStore) removeAllValidatorSnapshots() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// removing an entire bucket is much faster than removing all keys
		// look at thread https://github.com/boltdb/bolt/issues/667
		err := tx.DeleteBucket(validatorSnapshotsBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucket(validatorSnapshotsBucket)
		if err != nil {
			return err
		}

		return nil
	})
}

// epochsDBStats returns stats of epochs bucket in db
func (s *EpochStore) epochsDBStats() (*bolt.BucketStats, error) {
	return bucketStats(epochsBucket, s.db)
}

// validatorSnapshotsDBStats returns stats of validators snapshot bucket in db
func (s *EpochStore) validatorSnapshotsDBStats() (*bolt.BucketStats, error) {
	return bucketStats(validatorSnapshotsBucket, s.db)
}

// getNestedBucketInEpoch returns a nested (child) bucket from db associated with given epoch
func getNestedBucketInEpoch(tx *bolt.Tx, epoch uint64, bucketKey []byte) (*bolt.Bucket, error) {
	epochBucket, err := getEpochBucket(tx, epoch)
	if err != nil {
		return nil, err
	}

	bucket := epochBucket.Bucket(bucketKey)
	if bucket == nil {
		return nil, fmt.Errorf("could not find %v bucket for epoch: %v", string(bucketKey), epoch)
	}

	return bucket, nil
}
