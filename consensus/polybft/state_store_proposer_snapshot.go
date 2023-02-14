package polybft

import (
	"encoding/json"

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

// getProposerSnapshot gets latest proposer snapshot
func (s *ProposerSnapshotStore) getProposerSnapshot() (*ProposerSnapshot, error) {
	var snapshot *ProposerSnapshot

	err := s.db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket(proposerSnapshotBucket).Get(proposerSnapshotKey)
		if value == nil {
			return nil
		}

		return json.Unmarshal(value, &snapshot)
	})

	return snapshot, err
}

// writeProposerSnapshot writes proposer snapshot
func (s *ProposerSnapshotStore) writeProposerSnapshot(snapshot *ProposerSnapshot) error {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(proposerSnapshotBucket).Put(proposerSnapshotKey, raw)
	})
}
