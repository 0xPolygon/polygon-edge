package polybft

import (
	"encoding/binary"
	"encoding/json"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/hashicorp/go-hclog"
	bolt "go.etcd.io/bbolt"

	"github.com/umbracle/ethgo"
)

/*
The client has a boltDB backed state store. The schema as of looks as follows:

proposer snapshot/
|--> staticKey - only current one snapshot is preserved -> *ProposerSnapshot (json marshalled)
*/

var (
	// ABI
	stateTransferEventABI = contractsapi.StateSender.Abi.Events["StateSynced"]
	exitEventABI          = contractsapi.L2StateSender.Abi.Events["L2StateSynced"]
	ExitEventABIType      = exitEventABI.Inputs

	// proposerSnapshotKey is a static key which is used to save latest proposer snapshot.
	// (there will always be one object in bucket)
	proposerSnapshotKey = []byte("proposerSnapshotKey")
)

// ExitEvent is an event emitted by Exit contract
type ExitEvent struct {
	// ID is the decoded 'index' field from the event
	ID uint64 `abi:"id"`
	// Sender is the decoded 'sender' field from the event
	Sender ethgo.Address `abi:"sender"`
	// Receiver is the decoded 'receiver' field from the event
	Receiver ethgo.Address `abi:"receiver"`
	// Data is the decoded 'data' field from the event
	Data []byte `abi:"data"`
	// EpochNumber is the epoch number in which exit event was added
	EpochNumber uint64 `abi:"-"`
	// BlockNumber is the block in which exit event was added
	BlockNumber uint64 `abi:"-"`
}

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
	// Node identifier
	NodeID string
	// Number of epoch
	EpochNumber uint64
}

var (
	// bucket to store proposer calculator snapshot
	proposerCalcSnapshotBucket = []byte("proposerCalculatorSnapshot")
	// array of all parent buckets
	parentBuckets = [][]byte{syncStateEventsBucket, exitEventsBucket, commitmentsBucket, stateSyncProofsBucket,
		epochsBucket, validatorSnapshotsBucket, proposerCalcSnapshotBucket}
)

// State represents a persistence layer which persists consensus data off-chain
type State struct {
	db     *bolt.DB
	logger hclog.Logger
	close  chan struct{}

	StateSyncStore  *StateSyncStore
	CheckpointStore *CheckpointStore
	EpochStore      *EpochStore
}

// newState creates new instance of State
func newState(path string, logger hclog.Logger, closeCh chan struct{}) (*State, error) {
	db, err := bolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	if err = initMainDBBuckets(db); err != nil {
		return nil, err
	}

	state := &State{
		db:              db,
		logger:          logger.Named("state"),
		close:           closeCh,
		StateSyncStore:  &StateSyncStore{db: db},
		CheckpointStore: &CheckpointStore{db: db},
		EpochStore:      &EpochStore{db: db},
	}

	return state, nil
}

// initMainDBBuckets creates predefined buckets in bolt database if they don't exist already.
func initMainDBBuckets(db *bolt.DB) error {
	// init the buckets
	err := db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range parentBuckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

// epochsDBStats returns stats of epochs bucket in db
func (s *State) epochsDBStats() *bolt.BucketStats {
	return s.bucketStats(epochsBucket)
}

// validatorSnapshotsDBStats returns stats of validators snapshot bucket in db
func (s *State) validatorSnapshotsDBStats() *bolt.BucketStats {
	return s.bucketStats(validatorSnapshotsBucket)
}

// bucketStats returns stats for the given bucket in db
func (s *State) bucketStats(bucketName []byte) *bolt.BucketStats {
	var stats *bolt.BucketStats

	err := s.db.View(func(tx *bolt.Tx) error {
		s := tx.Bucket(bucketName).Stats()
		stats = &s

		return nil
	})

	if err != nil {
		s.logger.Error("Cannot check bucket stats", "Bucket name", string(bucketName), "Error", err)
	}

	return stats
}

// getProposerSnapshot gets latest proposer snapshot
func (s *State) getProposerSnapshot() (*ProposerSnapshot, error) {
	var snapshot *ProposerSnapshot

	err := s.db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket(proposerCalcSnapshotBucket).Get(proposerSnapshotKey)
		if value == nil {
			return nil
		}

		return json.Unmarshal(value, &snapshot)
	})

	return snapshot, err
}

// writeProposerSnapshot writes proposer snapshot
func (s *State) writeProposerSnapshot(snapshot *ProposerSnapshot) error {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(proposerCalcSnapshotBucket).Put(proposerSnapshotKey, raw)
	})
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)

	return b
}
