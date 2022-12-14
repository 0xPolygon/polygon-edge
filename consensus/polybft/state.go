package polybft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/0xPolygon/polygon-edge/types"
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
	ExitEventABIType      = abi.MustNewType("tuple(uint256 id, address sender, address receiver, bytes data)")
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
	stateSyncBundleSize = 1
)

type exitEventNotFoundError struct {
	exitID uint64
	epoch  uint64
}

func (e *exitEventNotFoundError) Error() string {
	return fmt.Sprintf("could not find any exit event that has an id: %v and epoch: %v", e.exitID, e.epoch)
}

// TODO: remove and refactor to use types.StateSyncEvent
// StateSyncEvent is a bridge event from the rootchain
type StateSyncEvent types.StateSyncEvent

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

	eventGeneric, err := decodeEventData(raw, log,
		func(id *big.Int, sender, receiver ethgo.Address, data []byte) interface{} {
			return newStateSyncEvent(id.Uint64(), sender, receiver, data)
		})
	if err != nil {
		return nil, err
	}

	stateSyncEvent, ok := eventGeneric.(*StateSyncEvent)
	if !ok {
		return nil, errors.New("failed to convert event to StateSyncEvent instance")
	}

	return stateSyncEvent, nil
}

func decodeExitEvent(log *ethgo.Log, epoch, block uint64) (*ExitEvent, error) {
	if !exitEventABI.Match(log) {
		// valid case, not an exit event
		return nil, nil
	}

	raw, err := exitEventABI.Inputs.ParseLog(log)
	if err != nil {
		return nil, err
	}

	eventGeneric, err := decodeEventData(raw, log,
		func(id *big.Int, sender, receiver ethgo.Address, data []byte) interface{} {
			return &ExitEvent{ID: id.Uint64(),
				Sender:      sender,
				Receiver:    receiver,
				Data:        data,
				EpochNumber: epoch,
				BlockNumber: block}
		})
	if err != nil {
		return nil, err
	}

	exitEvent, ok := eventGeneric.(*ExitEvent)
	if !ok {
		return nil, errors.New("failed to convert event to ExitEvent instance")
	}

	return exitEvent, err
}

// decodeEventData decodes provided map of event metadata and
// creates a generic instance which is returned by eventCreator callback
func decodeEventData(eventDataMap map[string]interface{}, log *ethgo.Log,
	eventCreator func(*big.Int, ethgo.Address, ethgo.Address, []byte) interface{}) (interface{}, error) {
	id, ok := eventDataMap["id"].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to decode id field of log: %+v", log)
	}

	sender, ok := eventDataMap["sender"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode sender field of log: %+v", log)
	}

	receiver, ok := eventDataMap["receiver"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode receiver field of log: %+v", log)
	}

	data, ok := eventDataMap["data"].([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to decode data field of log: %+v", log)
	}

	return eventCreator(id, sender, receiver, data), nil
}

// convertLog converts types.Log to ethgo.Log
func convertLog(log *types.Log) *ethgo.Log {
	l := &ethgo.Log{
		Address: ethgo.Address(log.Address),
		Data:    log.Data,
		Topics:  make([]ethgo.Hash, len(log.Topics)),
	}

	for i, topic := range log.Topics {
		l.Topics[i] = ethgo.Hash(topic)
	}

	return l
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
	// errNotEnoughStateSyncs error message
	errNotEnoughStateSyncs = errors.New("there is either a gap or not enough sync events")
	// errCommitmentNotBuilt error message
	errCommitmentNotBuilt = errors.New("there is no built commitment to register")
	// errNotAnExitEvent error message
	errNotAnExitEvent = errors.New("log not an exit event")
)

// State represents a persistence layer which persists consensus data off-chain
type State struct {
	db     *bolt.DB
	logger hclog.Logger
	close  chan struct{}
}

func newState(path string, logger hclog.Logger, closeCh chan struct{}) (*State, error) {
	db, err := bolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	if err = initMainDBBuckets(db); err != nil {
		return nil, err
	}

	state := &State{
		db:     db,
		logger: logger.Named("state"),
		close:  closeCh,
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

// insertExitEvents inserts a slice of exit events to exit event bucket in bolt db
func (s *State) insertExitEvents(exitEvents []*ExitEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(exitEventsBucket)
		for i := 0; i < len(exitEvents); i++ {
			if err := insertExitEventToBucket(bucket, exitEvents[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertExitEvent inserts a new exit event to exit event bucket in bolt db
func (s *State) insertExitEvent(event *ExitEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return insertExitEventToBucket(tx.Bucket(exitEventsBucket), event)
	})
}

// insertExitEventToBucket inserts exit event to exit event bucket
func insertExitEventToBucket(bucket *bolt.Bucket, exitEvent *ExitEvent) error {
	raw, err := json.Marshal(exitEvent)
	if err != nil {
		return err
	}

	return bucket.Put(bytes.Join([][]byte{itob(exitEvent.EpochNumber),
		itob(exitEvent.ID), itob(exitEvent.BlockNumber)}, nil), raw)
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *State) getExitEvent(exitEventID, epoch uint64) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(exitEventsBucket)

		key := bytes.Join([][]byte{itob(epoch), itob(exitEventID)}, nil)
		k, v := bucket.Cursor().Seek(key)
		fmt.Println(k, "k")
		fmt.Println(key, "key")
		fmt.Println(v, "v")

		if bytes.HasPrefix(k, key) == false || v == nil {
			return &exitEventNotFoundError{
				exitID: exitEventID,
				epoch:  epoch,
			}
		}

		return json.Unmarshal(v, &exitEvent)
	})

	return exitEvent, err
}

// getExitEventsByEpoch returns all exit events that happened in the given epoch
func (s *State) getExitEventsByEpoch(epoch uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch
	})
}

// getExitEventsForProof returns all exit events that happened in and prior to the given checkpoint block number
// with respect to the epoch in which block is added
func (s *State) getExitEventsForProof(epoch, checkpointBlock uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch && exitEvent.BlockNumber <= checkpointBlock
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

	// enforce sequential order
	sort.Slice(events, func(i, j int) bool {
		return events[i].ID < events[j].ID
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
				return errNotEnoughStateSyncs
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
