package polybft

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/hashicorp/go-hclog"
	bolt "go.etcd.io/bbolt"
)

var (
	edgeEventsLastProcessedBlockBucket = []byte("EdgeEventsLastProcessedBlock")
	edgeEventsLastProcessedBlockKey    = []byte("EdgeEventsLastProcessedBlockKey")
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
	ExitStore             *ExitStore
	EpochStore            *EpochStore
	ProposerSnapshotStore *ProposerSnapshotStore
	StakeStore            *StakeStore
	GovernanceStore       *GovernanceStore
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
		ExitStore:             &ExitStore{db: db},
		EpochStore:            &EpochStore{db: db},
		ProposerSnapshotStore: &ProposerSnapshotStore{db: db},
		StakeStore:            &StakeStore{db: db},
		GovernanceStore:       &GovernanceStore{db: db},
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
		if err := s.ExitStore.initialize(tx); err != nil {
			return err
		}
		if err := s.EpochStore.initialize(tx); err != nil {
			return err
		}
		if err := s.ProposerSnapshotStore.initialize(tx); err != nil {
			return err
		}
		if err := s.StakeStore.initialize(tx); err != nil {
			return err
		}

		_, err := tx.CreateBucketIfNotExists(edgeEventsLastProcessedBlockBucket)
		if err != nil {
			return fmt.Errorf("failed to create bucket=%s: %w", string(edgeEventsLastProcessedBlockBucket), err)
		}

		return s.GovernanceStore.initialize(tx)
	})
}

// insertLastProcessedEventsBlock inserts the last processed block for events on Edge
func (s *State) insertLastProcessedEventsBlock(block uint64, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		return tx.Bucket(edgeEventsLastProcessedBlockBucket).Put(
			edgeEventsLastProcessedBlockKey, common.EncodeUint64ToBytes(block))
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getLastProcessedEventsBlock gets the last processed block for events on Edge
func (s *State) getLastProcessedEventsBlock(dbTx *bolt.Tx) (uint64, error) {
	var (
		lastProcessed uint64
		err           error
	)

	getFn := func(tx *bolt.Tx) {
		value := tx.Bucket(edgeEventsLastProcessedBlockBucket).Get(edgeEventsLastProcessedBlockKey)
		if value != nil {
			lastProcessed = common.EncodeBytesToUint64(value)
		}
	}

	if dbTx == nil {
		err = s.db.View(func(tx *bolt.Tx) error {
			getFn(tx)

			return nil
		})
	} else {
		getFn(dbTx)
	}

	return lastProcessed, err
}

// beginDBTransaction creates and begins a transaction on BoltDB
// Note that transaction needs to be manually rollback or committed
func (s *State) beginDBTransaction(isWriteTx bool) (*bolt.Tx, error) {
	return s.db.Begin(isWriteTx)
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
