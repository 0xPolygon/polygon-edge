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
func (s *ProposerSnapshotStore) getProposerSnapshot() (*ProposerSnapshot, error) {
	var snapshot *ProposerSnapshot

	err := s.db.View(func(tx *bolt.Tx) error {
		s, err := s.getProposerSnapshotWithTx(tx)
		if err != nil {
			return err
		}

		snapshot = s

		return nil
	})

	return snapshot, err
}

func (s *ProposerSnapshotStore) getProposerSnapshotWithTx(dbTx DBTransaction) (*ProposerSnapshot, error) {
	value := dbTx.Bucket(proposerSnapshotBucket).Get(proposerSnapshotKey)
	if value == nil {
		return nil, nil
	}

	var snapshot *ProposerSnapshot
	if err := json.Unmarshal(value, &snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

// writeProposerSnapshot writes proposer snapshot
func (s *ProposerSnapshotStore) writeProposerSnapshot(snapshot *ProposerSnapshot) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.writeProposerSnapshotWithTx(snapshot, tx)
	})
}

// writeProposerSnapshot writes proposer snapshot
// Note that function assumes that db tx is already open
func (s *ProposerSnapshotStore) writeProposerSnapshotWithTx(snapshot *ProposerSnapshot, dbTx DBTransaction) error {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return dbTx.Bucket(proposerSnapshotBucket).Put(proposerSnapshotKey, raw)
}
