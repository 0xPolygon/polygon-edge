package polybft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/hashicorp/go-hclog"
	bolt "go.etcd.io/bbolt"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

/*
The client has a boltDB backed state store. The schema as of looks as follows:

state sync events/
|--> stateSyncEvent.Id -> *StateSyncEvent (json marshalled)

commitments/
|--> commitment.Message.ToIndex -> *CommitmentMessageSigned (json marshalled)

bundles/
|--> bundle.StateSyncs[0].Id -> *BundleProof (json marshalled)

epochs/
|--> epochNumber
	|--> hash -> []*MessageSignatures (json marshalled)

validatorSnapshots/
|--> epochNumber -> *AccountSet (json marshalled)

exit events/
|--> (id+epoch+blockNumber) -> *ExitEvent (json marshalled)
*/

var (
	// ABI
	stateTransferEventABI = abi.MustNewEvent("event StateSynced(uint256 indexed id, address indexed sender, address indexed receiver, bytes data)")   //nolint:lll
	exitEventABI          = abi.MustNewEvent("event L2StateSynced(uint256 indexed id, address indexed sender, address indexed receiver, bytes data)") //nolint:lll
	exitEventABIType      = abi.MustNewType("tuple(uint256 id, address sender, address receiver, bytes data)")
)

const (
	// validatorSnapshotLimit defines a maximum number of validator snapshots
	// that can be stored in cache (both memory and db)
	validatorSnapshotLimit = 100
	// numberOfSnapshotsToLeaveInMemory defines a number of validator snapshots to leave in memory
	numberOfSnapshotsToLeaveInMemory = 2
	// numberOfSnapshotsToLeaveInMemory defines a number of validator snapshots to leave in db
	numberOfSnapshotsToLeaveInDB = 10
	// number of stateSyncEvents to be processed before a commitment message can be created and gossiped
	stateSyncMainBundleSize = 10
	// number of stateSyncEvents to be grouped into one StateTransaction
	stateSyncBundleSize = 5
)

type ExitEventNotFoundError struct {
	exitID          uint64
	epoch           uint64
	checkpointBlock uint64
}

func (e *ExitEventNotFoundError) Error() string {
	return fmt.Sprintf("could not find any exit event that has an id: %v, added in block: %v and epoch: %v",
		e.exitID, e.checkpointBlock, e.epoch)
}

// StateSyncEvent is a bridge event from the rootchain
type StateSyncEvent struct {
	// ID is the decoded 'index' field from the event
	ID uint64
	// Sender is the decoded 'sender' field from the event
	Sender ethgo.Address
	// Receiver is the decoded 'receiver' field from the event
	Receiver ethgo.Address
	// Data is the decoded 'data' field from the event
	Data []byte
	// Skip is the decoded 'skip' field from the event
	Skip bool
}

// newStateSyncEvent creates an instance of pending state sync event.
func newStateSyncEvent(
	id uint64,
	sender ethgo.Address,
	target ethgo.Address,
	data []byte,
) *StateSyncEvent {
	return &StateSyncEvent{
		ID:       id,
		Sender:   sender,
		Receiver: target,
		Data:     data,
	}
}

func (s *StateSyncEvent) String() string {
	return fmt.Sprintf("Id=%d, Sender=%v, Target=%v", s.ID, s.Sender, s.Receiver)
}

func decodeStateSyncEvent(log *ethgo.Log) (*StateSyncEvent, error) {
	raw, err := stateTransferEventABI.ParseLog(log)
	if err != nil {
		return nil, err
	}

	id, ok := raw["id"].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to decode id field of log: %+v", log)
	}

	sender, ok := raw["sender"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode sender field of log: %+v", log)
	}

	target, ok := raw["receiver"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode target field of log: %+v", log)
	}

	data, ok := raw["data"].([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to decode data field of log: %+v", log)
	}

	return newStateSyncEvent(id.Uint64(), sender, target, data), nil
}

// TODO - maybe the two decodeEvent functions can be merged since data is pretty similar
func decodeExitEvent(log *ethgo.Log, epoch, block uint64) (*ExitEvent, error) {
	raw, err := exitEventABI.ParseLog(log)
	if err != nil {
		return nil, err
	}

	id, ok := raw["id"].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to decode id field of log: %+v", log)
	}

	sender, ok := raw["sender"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode sender field of log: %+v", log)
	}

	target, ok := raw["receiver"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode target field of log: %+v", log)
	}

	data, ok := raw["data"].([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to decode data field of log: %+v", log)
	}

	return &ExitEvent{id.Uint64(), sender, target, data, epoch, block}, nil
}

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
	// Epoch is the epoch number in which exit event was added
	Epoch uint64 `abi:"-"`
	// BlockNumber is the block in which exit event was added
	BlockNumber uint64 `abi:"-"`
}

// MessageSignature encapsulates sender identifier and its signature
type MessageSignature struct {
	// Signer of the vote
	From pbft.NodeID
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
	NodeID pbft.NodeID
	// Number of epoch
	EpochNumber uint64
}

var (
	// bucket to store rootchain bridge events
	syncStateEventsBucket = []byte("stateSyncEvents")
	// bucket to store exit contract events
	exitEventsBucket = []byte("exitEvent")
	// bucket to store commitments
	commitmentsBucket = []byte("commitments")
	// bucket to store bundles
	bundlesBucket = []byte("bundles")
	// bucket to store epochs and all its nested buckets (message votes and message pool events)
	epochsBucket = []byte("epochs")
	// bucket to store message votes (signatures)
	messageVotesBucket = []byte("votes")
	// bucket to store validator snapshots
	validatorSnapshotsBucket = []byte("validatorSnapshots")
	// array of all parent buckets
	parentBuckets = [][]byte{syncStateEventsBucket, exitEventsBucket, commitmentsBucket, bundlesBucket,
		epochsBucket, validatorSnapshotsBucket}
	// ErrNotEnoughStateSyncs error message
	ErrNotEnoughStateSyncs = errors.New("there is either a gap or not enough sync events")
	// ErrCommitmentNotBuilt error message
	ErrCommitmentNotBuilt = errors.New("there is no built commitment to register")
)

// State represents a persistence layer which persists consensus data off-chain
type State struct {
	db     *bolt.DB
	logger hclog.Logger
}

func newState(path string, logger hclog.Logger) (*State, error) {
	db, err := bolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	err = initMainDBBuckets(db)
	if err != nil {
		return nil, err
	}

	state := &State{
		db:     db,
		logger: logger.Named("state"),
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

// insertValidatorSnapshot inserts a validator snapshot for the given epoch to its bucket in db
func (s *State) insertValidatorSnapshot(epoch uint64, validatorSnapshot AccountSet) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := validatorSnapshot.Marshal()
		if err != nil {
			return err
		}

		bucket := tx.Bucket(validatorSnapshotsBucket)

		return bucket.Put(itob(epoch), raw)
	})
}

// getValidatorSnapshot queries the validator snapshot for given epoch from db
func (s *State) getValidatorSnapshot(epoch uint64) (AccountSet, error) {
	var validatorSnapshot AccountSet

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(validatorSnapshotsBucket)
		v := bucket.Get(itob(epoch))
		if v != nil {
			return validatorSnapshot.Unmarshal(v)
		}

		return nil
	})

	return validatorSnapshot, err
}

// list iterates through all events in events bucket in db, unmarshals them, and returns as array
func (s *State) list() ([]*StateSyncEvent, error) {
	events := []*StateSyncEvent{}

	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(syncStateEventsBucket).ForEach(func(k, v []byte) error {
			var event *StateSyncEvent
			if err := json.Unmarshal(v, &event); err != nil {
				return err
			}
			events = append(events, event)

			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return events, nil
}

func compose(slices [][]byte) []byte {
	var totalLen, i int

	for _, s := range slices {
		totalLen += len(s)
	}

	tmp := make([]byte, totalLen)

	for _, s := range slices {
		i += copy(tmp[i:], s)
	}

	return tmp
}

// insertStateSyncEvent inserts a new state sync event to state event bucket in db
func (s *State) insertExitEvent(event *ExitEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}

		bucket := tx.Bucket(exitEventsBucket)

		return bucket.Put(compose([][]byte{itob(event.Epoch), itob(event.ID), itob(event.BlockNumber)}), raw)
	})
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *State) getExitEvent(exitEventID, epoch, checkpointBlockNumber uint64) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(exitEventsBucket)

		key := compose([][]byte{itob(epoch), itob(exitEventID), itob(checkpointBlockNumber)})
		v := bucket.Get(key)
		if v == nil {
			return &ExitEventNotFoundError{exitEventID, checkpointBlockNumber, epoch}
		}

		return json.Unmarshal(v, &exitEvent)
	})

	return exitEvent, err
}

// getExitEventsByEpoch returns all exit events that happened in the given epoch
func (s *State) getExitEventsByEpoch(epoch uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.Epoch == epoch
	})
}

// getExitEventsForProof returns all exit events that happened in and prior to the given checkpoint block number
// with respect to the epoch in which block is added
func (s *State) getExitEventsForProof(epoch, checkpointBlock uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.Epoch == epoch && exitEvent.BlockNumber <= checkpointBlock
	})
}

// getExitEvents returns exit events for given epoch and provided filter
func (s *State) getExitEvents(epoch uint64, filter func(exitEvent *ExitEvent) bool) ([]*ExitEvent, error) {
	var events []*ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(exitEventsBucket).Cursor()
		prefix := itob(epoch)

		for k, v := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var event *ExitEvent
			if err := json.Unmarshal(v, &event); err != nil {
				return err
			}

			if filter(event) {
				events = append(events, event)
			}
		}

		return nil
	})

	return events, err
}

// insertStateSyncEvent inserts a new state sync event to state event bucket in db
func (s *State) insertStateSyncEvent(event *StateSyncEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}

		bucket := tx.Bucket(syncStateEventsBucket)

		return bucket.Put(itob(event.ID), raw)
	})
}

// getStateSyncEventsForCommitment returns state sync events for commitment
// if there is an event with index that can not be found in db in given range, an error is returned
func (s *State) getStateSyncEventsForCommitment(fromIndex, toIndex uint64) ([]*StateSyncEvent, error) {
	var events []*StateSyncEvent

	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncStateEventsBucket)
		for i := fromIndex; i <= toIndex; i++ {
			v := bucket.Get(itob(i))
			if v == nil {
				return ErrNotEnoughStateSyncs
			}

			var event *StateSyncEvent
			if err := json.Unmarshal(v, &event); err != nil {
				return err
			}

			events = append(events, event)
		}

		return nil
	})

	return events, err
}

// insertEpoch inserts a new epoch to db with its meta data
func (s *State) insertEpoch(epoch uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		epochBucket, err := tx.Bucket(epochsBucket).CreateBucketIfNotExists(itob(epoch))
		if err != nil {
			return err
		}
		_, err = epochBucket.CreateBucketIfNotExists(messageVotesBucket)

		return err
	})
}

// isEpochInserted checks if given epoch is present in db
func (s *State) isEpochInserted(epoch uint64) bool {
	return s.db.View(func(tx *bolt.Tx) error {
		_, err := getEpochBucket(tx, epoch)

		return err
	}) == nil
}

// insertCommitmentMessage inserts signed commitment to db
func (s *State) insertCommitmentMessage(commitment *CommitmentMessageSigned) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(commitment)
		if err != nil {
			return err
		}

		if err := tx.Bucket(commitmentsBucket).Put(itob(commitment.Message.ToIndex), raw); err != nil {
			return err
		}

		return nil
	})
}

// getCommitmentMessage queries the signed commitment from the db
func (s *State) getCommitmentMessage(toIndex uint64) (*CommitmentMessageSigned, error) {
	var commitment *CommitmentMessageSigned

	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(commitmentsBucket).Get(itob(toIndex))
		if raw == nil {
			return nil
		}

		return json.Unmarshal(raw, &commitment)
	})

	return commitment, err
}

// getNonExecutedCommitments gets non executed commitments
// (commitments whose toIndex is greater than or equal to startIndex)
func (s *State) getNonExecutedCommitments(startIndex uint64) ([]*CommitmentMessageSigned, error) {
	var commitments []*CommitmentMessageSigned

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(commitmentsBucket).Cursor()

		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			if itou(k) < startIndex {
				// reached a commitment that was executed
				break
			}

			var commitment *CommitmentMessageSigned
			if err := json.Unmarshal(v, &commitment); err != nil {
				return err
			}

			commitments = append(commitments, commitment)
		}

		return nil
	})

	sort.Slice(commitments, func(i, j int) bool {
		return commitments[i].Message.FromIndex < commitments[j].Message.FromIndex
	})

	return commitments, err
}

// cleanCommitments cleans all commitments that are older than the provided fromIndex, alongside their proofs
func (s *State) cleanCommitments(stateSyncExecutionIndex uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		commitmentsBucket := tx.Bucket(commitmentsBucket)
		commitmentsCursor := commitmentsBucket.Cursor()
		for k, _ := commitmentsCursor.First(); k != nil; k, _ = commitmentsCursor.Next() {
			if itou(k) >= stateSyncExecutionIndex {
				// reached a commitment that is not executed
				break
			}

			if err := commitmentsBucket.Delete(k); err != nil {
				return err
			}
		}

		bundlesBucket := tx.Bucket(bundlesBucket)
		bundlesCursor := bundlesBucket.Cursor()
		for k, _ := bundlesCursor.First(); k != nil; k, _ = bundlesCursor.Next() {
			if itou(k) >= stateSyncExecutionIndex {
				// reached a bundle that is not executed
				break
			}

			if err := bundlesBucket.Delete(k); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertBundles inserts the provided bundles to db
func (s *State) insertBundles(bundles []*BundleProof) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bundlesBucket := tx.Bucket(bundlesBucket)
		for _, b := range bundles {
			raw, err := json.Marshal(b)
			if err != nil {
				return err
			}

			if err := bundlesBucket.Put(itob(b.ID()), raw); err != nil {
				return err
			}
		}

		return nil
	})
}

// getBundles gets bundles that are not executed
func (s *State) getBundles(stateSyncExecutionIndex, maxNumberOfBundles uint64) ([]*BundleProof, error) {
	var bundles []*BundleProof

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bundlesBucket).Cursor()
		processed := uint64(0)
		for k, v := c.First(); k != nil && processed < maxNumberOfBundles; k, v = c.Next() {
			if itou(k) >= stateSyncExecutionIndex {
				var bundle *BundleProof
				if err := json.Unmarshal(v, &bundle); err != nil {
					return err
				}

				bundles = append(bundles, bundle)
				processed++
			}
		}

		return nil
	})

	return bundles, err
}

// insertMessageVote inserts given vote to signatures bucket of given epoch
func (s *State) insertMessageVote(epoch uint64, key []byte, vote *MessageSignature) (int, error) {
	var numSignatures int

	err := s.db.Update(func(tx *bolt.Tx) error {
		signatures, err := s.getMessageVotesLocked(tx, epoch, key)
		if err != nil {
			return err
		}

		// check if the signature has already being included
		for _, sigs := range signatures {
			if sigs.From == vote.From {
				numSignatures = len(signatures)

				return nil
			}
		}

		if signatures == nil {
			signatures = []*MessageSignature{vote}
		} else {
			signatures = append(signatures, vote)
		}
		numSignatures = len(signatures)

		raw, err := json.Marshal(signatures)
		if err != nil {
			return err
		}

		bucket, err := getNestedBucketInEpoch(tx, epoch, messageVotesBucket)
		if err != nil {
			return err
		}

		if err := bucket.Put(key, raw); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return numSignatures, nil
}

// getMessageVotes gets all signatures from db associated with given epoch and hash
func (s *State) getMessageVotes(epoch uint64, hash []byte) ([]*MessageSignature, error) {
	var signatures []*MessageSignature

	err := s.db.View(func(tx *bolt.Tx) error {
		res, err := s.getMessageVotesLocked(tx, epoch, hash)
		if err != nil {
			return err
		}
		signatures = res

		return nil
	})

	if err != nil {
		return nil, err
	}

	return signatures, nil
}

// getMessageVotesLocked gets all signatures from db associated with given epoch and hash
func (s *State) getMessageVotesLocked(tx *bolt.Tx, epoch uint64, hash []byte) ([]*MessageSignature, error) {
	bucket, err := getNestedBucketInEpoch(tx, epoch, messageVotesBucket)
	if err != nil {
		return nil, err
	}

	v := bucket.Get(hash)
	if v == nil {
		return nil, nil
	}

	var signatures []*MessageSignature
	if err := json.Unmarshal(v, &signatures); err != nil {
		return nil, err
	}

	return signatures, nil
}

// getNestedBucketInEpoch returns a nested (child) bucket from db associated with given epoch
func getNestedBucketInEpoch(tx *bolt.Tx, epoch uint64, bucketKey []byte) (*bolt.Bucket, error) {
	epochBucket, err := getEpochBucket(tx, epoch)
	if err != nil {
		return nil, err
	}

	bucket := epochBucket.Bucket(bucketKey)

	if epochBucket == nil {
		return nil, fmt.Errorf("could not find %v bucket for epoch: %v", string(bucketKey), epoch)
	}

	return bucket, nil
}

// getEpochBucket returns bucket from db associated with given epoch
func getEpochBucket(tx *bolt.Tx, epoch uint64) (*bolt.Bucket, error) {
	epochBucket := tx.Bucket(epochsBucket).Bucket(itob(epoch))
	if epochBucket == nil {
		return nil, fmt.Errorf("could not find bucket for epoch: %v", epoch)
	}

	return epochBucket, nil
}

// cleanEpochsFromDB cleans epoch buckets from db
func (s *State) cleanEpochsFromDB() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(epochsBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(epochsBucket)

		return err
	})
}

// cleanValidatorSnapshotsFromDB cleans the validator snapshots bucket if a limit is reached,
// but it leaves the latest (n) number of snapshots
func (s *State) cleanValidatorSnapshotsFromDB(epoch uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(validatorSnapshotsBucket)

		// paired list
		keys := make([][]byte, 0)
		values := make([][]byte, 0)
		if numberOfSnapshotsToLeaveInDB > 0 { // TODO this is always true?!
			for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ { // exclude the last inserted we already appended
				key := itob(epoch)
				value := bucket.Get(key)
				if value == nil {
					continue
				}
				keys = append(keys, key)
				values = append(values, value)
				epoch--
			}
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
	})
}

// removeAllValidatorSnapshots drops a validator snapshot bucket and re-creates it in bolt database
func (s *State) removeAllValidatorSnapshots() error {
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

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)

	return b
}

func itou(v []byte) uint64 {
	return binary.BigEndian.Uint64(v)
}
