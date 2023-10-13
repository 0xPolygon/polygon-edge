package polybft

import (
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

/*
Bolt DB schema:

proposer snapshot/
|--> proposerSnapshotKey - only current one snapshot is preserved -> *ProposerSnapshot (json marshalled)
*/
var (
	// bucket to store proposer calculator snapshot
	proposerSnapshotBucket = []byte("proposerSnapshot")
	// proposerSnapshotKey is a static key which is used to save latest proposer snapshot.
	// (there will always be one object in bucket)
	proposerSnapshotKey = []byte("proposerSnapshotKey")
)

type ProposerSnapshotStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *ProposerSnapshotStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(proposerSnapshotBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(validatorSnapshotsBucket), err)
	}

	return nil
}

// getProposerSnapshot gets latest proposer snapshot
func (s *ProposerSnapshotStore) getProposerSnapshot(dbTx *bolt.Tx) (*ProposerSnapshot, error) {
	var (
		snapshot *ProposerSnapshot
		err      error
	)

	getFn := func(tx *bolt.Tx) error {
		value := tx.Bucket(proposerSnapshotBucket).Get(proposerSnapshotKey)
		if value == nil {
			return nil
		}

		return json.Unmarshal(value, &snapshot)
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

// writeProposerSnapshot writes proposer snapshot
func (s *ProposerSnapshotStore) writeProposerSnapshot(snapshot *ProposerSnapshot, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}

		return tx.Bucket(proposerSnapshotBucket).Put(proposerSnapshotKey, raw)
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}
