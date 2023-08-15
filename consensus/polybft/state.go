package polybft

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	bolt "go.etcd.io/bbolt"
)

// MessageSignature encapsulates sender identifier and its signature
type MessageSignature struct {
	// Signer of the vote
	From string
	// Signature of the message
	Signature []byte
}

// TransportMessage represents the payload which is gossiped across the network
type TransportMessage struct {
	// Hash is encoded data
	Hash []byte
	// Message signature
	Signature []byte
	// From is the address of the message signer
	From string
	// Number of epoch
	EpochNumber uint64
}

// State represents a persistence layer which persists consensus data off-chain
type State struct {
	db    *bolt.DB
	close chan struct{}

	StateSyncStore        *StateSyncStore
	CheckpointStore       *CheckpointStore
	EpochStore            *EpochStore
	ProposerSnapshotStore *ProposerSnapshotStore
	StakeStore            *StakeStore
}

// newState creates new instance of State
func newState(path string, logger hclog.Logger, closeCh chan struct{}) (*State, error) {
	db, err := bolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	s := &State{
		db:                    db,
		close:                 closeCh,
		StateSyncStore:        &StateSyncStore{db: db},
		CheckpointStore:       &CheckpointStore{db: db},
		EpochStore:            &EpochStore{db: db},
		ProposerSnapshotStore: &ProposerSnapshotStore{db: db},
		StakeStore:            &StakeStore{db: db},
	}

	if err = s.initStorages(); err != nil {
		return nil, err
	}

	return s, nil
}

// initStorages initializes data storages
func (s *State) initStorages() error {
	// init the buckets
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.StateSyncStore.initialize(tx); err != nil {
			return err
		}
		if err := s.CheckpointStore.initialize(tx); err != nil {
			return err
		}
		if err := s.EpochStore.initialize(tx); err != nil {
			return err
		}
		if err := s.ProposerSnapshotStore.initialize(tx); err != nil {
			return err
		}

		return s.StakeStore.initialize(tx)
	})
}

// bucketStats returns stats for the given bucket in db
func bucketStats(bucketName []byte, db *bolt.DB) (*bolt.BucketStats, error) {
	var stats *bolt.BucketStats

	err := db.View(func(tx *bolt.Tx) error {
		s := tx.Bucket(bucketName).Stats()
		stats = &s

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("cannot check bucket stats. Bucket name=%s: %w", string(bucketName), err)
	}

	return stats, nil
}
