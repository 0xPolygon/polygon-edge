package polybft

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/boltdb/bolt"
	"github.com/hashicorp/go-memdb"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

/*
The client has a boltDB backed state store. The schema as of looks as follows:

events/
|--> stateSyncEvent.Id -> *StateSyncEvent (json marshalled)

commitments/
|--> commitment.SignedCommitment.Message.ToIndex -> *CommitmentToExecute (json marshalled)

messageVotes/
|--> hash -> *MessageVotes (json marshalled)

validatorSnapshots/
|--> epochNumber -> *AccountSet (json marshalled)
*/

var (
	// ABI
	stateTransferEvent      = abi.MustNewEvent("event StateSync(uint256 indexed id, address indexed sender, address indexed target, bytes data)") //nolint:lll
	onStateReceiveMethod, _ = abi.NewMethod("function onStateReceive(uint64, address, bytes)")
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

	stateSyncTable         = "state_sync"
	commitmentTable        = "commitment"
	messageVoteTable       = "vote"
	validatorSnapshotTable = "snapshot"
)

var (
	// bucket to store rootchain bridge events
	syncStateEventsBucket = []byte(stateSyncTable)
	//bucket to store commitments
	commitmentsBucket = []byte(commitmentTable)
	// bucket to store message votes (signatures)
	messageVotesBucket = []byte(messageVoteTable)
	// bucket to store validator snapshots
	validatorSnapshotsBucket = []byte(validatorSnapshotTable)
	// array of all parent buckets
	parentBuckets = [][]byte{syncStateEventsBucket, commitmentsBucket, messageVotesBucket, validatorSnapshotsBucket}
	// errNotEnoughStateSyncs error message
	errNotEnoughStateSyncs = errors.New("there is not enough sync events")
	// errGapInStateSyncs error message
	errGapInStateSyncs = errors.New("there is a gap in state sync events")
	// errCommitmentNotBuilt error message
	errCommitmentNotBuilt = errors.New("there is no built commitment to register")
	// errItemNotMemDBRecord error message
	errItemNotMemDBRecord = errors.New("item not a MemDBRecord")
)

// MemDBRecord is an interface that all records in memdb implement
type MemDBRecord interface {
	// Key returns unique identification of a db record
	Key() uint64
}

// StateSyncEvent is a bridge event from the rootchain
type StateSyncEvent struct {
	// ID is the decoded 'index' field from the event
	ID uint64
	// Sender is the decoded 'sender' field from the event
	Sender ethgo.Address
	// Target is the decoded 'target' field from the event
	Target ethgo.Address
	// Data is the decoded 'data' field from the event
	Data []byte
	// Log contains raw data about smart contract event execution
	Log *ethgo.Log
}

func (s *StateSyncEvent) String() string {
	return fmt.Sprintf("Id=%d, Sender=%v, Target=%v", s.ID, s.Sender, s.Target)
}

var _ MemDBRecord = &StateSyncEvent{}

// Key returns the ID field of state sync event which is the unique key in memdb
func (s *StateSyncEvent) Key() uint64 {
	return s.ID
}

// newStateSyncEvent creates an instance of pending state sync event.
func newStateSyncEvent(
	id uint64,
	sender ethgo.Address,
	target ethgo.Address,
	data []byte, log *ethgo.Log,
) *StateSyncEvent {
	return &StateSyncEvent{
		ID:     id,
		Sender: sender,
		Target: target,
		Data:   data,
		Log:    log,
	}
}

func decodeEvent(log *ethgo.Log) (*StateSyncEvent, error) {
	raw, err := stateTransferEvent.ParseLog(log)
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

	target, ok := raw["target"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode target field of log: %+v", log)
	}

	data, ok := raw["data"].([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to decode data field of log: %+v", log)
	}

	return newStateSyncEvent(id.Uint64(), sender, target, data, log), nil
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
	Hash string
	// Message signature
	Signature []byte
	// Node identifier
	NodeID pbft.NodeID
	// Number of epoch
	EpochNumber uint64
}

var _ MemDBRecord = &CommitmentToExecute{}

// CommitmentToExecute represents a finalized commitment
type CommitmentToExecute struct {
	SignedCommitment *CommitmentMessageSigned
	Proofs           []*BundleProof
	ToIndex          uint64
}

// Key returns the ToIndex of commitment which is the unique key in memdb
func (c *CommitmentToExecute) Key() uint64 {
	return c.ToIndex
}

// getBundles returns bundles from commitment that can be executed in current sprint
func (c *CommitmentToExecute) getBundles(maxNumberOfBundles, processedBundles int) ([]*BundleProof, int) {
	commitmentBundlesNum := len(c.Proofs)
	if commitmentBundlesNum == 0 {
		return nil, commitmentBundlesNum
	}

	if (processedBundles + commitmentBundlesNum) < maxNumberOfBundles {
		return c.Proofs, commitmentBundlesNum
	}

	return c.Proofs[0 : maxNumberOfBundles-processedBundles], commitmentBundlesNum
}

var _ MemDBRecord = &MessageVotes{}

// MessageVotes represents accumulated votes for a hash in a single epoch
type MessageVotes struct {
	Epoch      uint64
	Hash       string
	Signatures []*MessageSignature
}

// Key returns the Epoch of message vote which is the unique key in memdb
func (mv *MessageVotes) Key() uint64 {
	return mv.Epoch
}

var _ MemDBRecord = &ValidatorSnapshot{}

// ValidatorSnapshot represents a snapshot of validators for given epoch
type ValidatorSnapshot struct {
	Epoch      uint64
	AccountSet AccountSet
}

// Key returns the Epoch of validator snapshot which is the unique key in memdb
func (vs *ValidatorSnapshot) Key() uint64 {
	return vs.Epoch
}

// MemDBIterator is an interface implemented by every custom memdb iterator
type MemDBIterator interface {
	Next() (MemDBRecord, error)
}

// memStateSchema represents the schema of the in-memory db
var memStateSchema = &memdb.DBSchema{
	Tables: map[string]*memdb.TableSchema{
		// list of state syncs
		stateSyncTable: {
			Name: stateSyncTable,
			Indexes: map[string]*memdb.IndexSchema{
				"id": {
					Name:    "id",
					Unique:  true,
					Indexer: &memdb.UintFieldIndex{Field: "ID"},
				},
			},
		},
		commitmentTable: {
			Name: commitmentTable,
			Indexes: map[string]*memdb.IndexSchema{
				"id": {
					Name:    "id",
					Unique:  true,
					Indexer: &memdb.UintFieldIndex{Field: "ToIndex"},
				},
			},
		},
		messageVoteTable: {
			Name: messageVoteTable,
			Indexes: map[string]*memdb.IndexSchema{
				"id": {
					Name:    "id",
					Unique:  true,
					Indexer: &memdb.UintFieldIndex{Field: "Epoch"},
				},
				"hash": {
					Name:    "hash",
					Unique:  true,
					Indexer: &memdb.StringFieldIndex{Field: "Hash"},
				},
			},
		},
		validatorSnapshotTable: {
			Name: validatorSnapshotTable,
			Indexes: map[string]*memdb.IndexSchema{
				"id": {
					Name:    "id",
					Unique:  true,
					Indexer: &memdb.UintFieldIndex{Field: "Epoch"},
				},
			},
		},
	},
}

func getUpperBoundUint64(upperBound interface{}) uint64 {
	if upperBound == nil {
		// if there is no upper bound provided, then we return everything
		return math.MaxUint64
	}

	upper, ok := upperBound.(uint64)

	if !ok {
		return math.MaxUint64
	}

	return upper
}

var iterators = map[string]func(it memdb.ResultIterator, upperBound interface{}, first MemDBRecord) MemDBIterator{
	stateSyncTable: func(it memdb.ResultIterator, upperBound interface{}, first MemDBRecord) MemDBIterator {
		event, isOk := first.(*StateSyncEvent)
		if !isOk {
			panic("failed to create state sync events table iterator, because element is not of type StateSyncEvent")
		}

		return &StateSyncIterator{iter: it, next: event, upperBound: getUpperBoundUint64(upperBound)}
	},
	commitmentTable: func(it memdb.ResultIterator, upperBound interface{}, first MemDBRecord) MemDBIterator {
		return &MemDBRecordIterator{iter: it, next: first, upperBound: getUpperBoundUint64(upperBound)}
	},
	messageVoteTable: func(it memdb.ResultIterator, upperBound interface{}, first MemDBRecord) MemDBIterator {
		return &MemDBRecordIterator{iter: it, next: first, upperBound: getUpperBoundUint64(upperBound)}
	},
	validatorSnapshotTable: func(it memdb.ResultIterator, upperBound interface{}, first MemDBRecord) MemDBIterator {
		return &MemDBRecordIterator{iter: it, next: first, upperBound: getUpperBoundUint64(upperBound)}
	},
}

// StateSyncIterator is a custom memdb iterator that ensures
// sequential order of state sync events that are returned from db
type StateSyncIterator struct {
	iter         memdb.ResultIterator
	upperBound   uint64
	next         MemDBRecord
	lastGottenID uint64
}

// Next returns the next item of the iterator. It is **not** safe to call
// Next after the last call returned nil.
func (s *StateSyncIterator) Next() (MemDBRecord, error) {
	res := s.next
	s.next = nil

	if res != nil {
		s.lastGottenID = res.Key()
	}

	nextItem := s.iter.Next()
	if nextItem == nil {
		// for the case when there is not enough state sync events
		if s.lastGottenID < s.upperBound && s.upperBound != math.MaxUint64 {
			return nil, errNotEnoughStateSyncs
		}

		return res, nil
	}

	obj, isOk := nextItem.(MemDBRecord)
	if !isOk {
		return nil, errItemNotMemDBRecord
	}

	if obj.Key() > s.upperBound {
		return res, nil
	}

	// ensure linearity
	if res != nil && res.Key()+1 != obj.Key() {
		// for the case when there is a gap in state syncs
		return nil, errGapInStateSyncs
	}

	s.next = obj

	return res, nil
}

// MemDBRecordIterator is a custom iterator for all MemDBRecords
type MemDBRecordIterator struct {
	iter       memdb.ResultIterator
	upperBound uint64
	next       MemDBRecord
}

// Next returns the next item of the iterator. It is **not** safe to call
// Next after the last call returned nil.
func (s *MemDBRecordIterator) Next() (MemDBRecord, error) {
	res := s.next
	s.next = nil

	nextItem := s.iter.Next()

	if nextItem == nil {
		return res, nil
	}

	obj, isOk := nextItem.(MemDBRecord)
	if !isOk {
		return nil, errItemNotMemDBRecord
	}

	if obj.Key() > s.upperBound {
		return res, nil
	}

	s.next = obj

	return res, nil
}

// insertToMemDB inserts a new record in provided table
func insertToMemDB[V MemDBRecord](memdb *memdb.MemDB, table string, record V) error {
	txn := memdb.Txn(true)
	if err := txn.Insert(table, record); err != nil {
		txn.Abort()

		return err
	}

	txn.Commit()

	return nil
}

type Number interface {
	int64 | uint64
}

// getFilteredFromMemDB returns a filtered collection of desired memdb records
func getFilteredFromMemDB[V MemDBRecord, BV Number](memdb *memdb.MemDB, table string,
	lowerBound, upperBound BV) ([]V, error) {
	txn := memdb.Txn(false)
	defer txn.Abort()

	memdbIterator, err := txn.LowerBound(table, "id", lowerBound)
	if err != nil {
		return nil, err
	}

	var slice []V

	iteratorCreator, exists := iterators[table]

	if !exists {
		panic(fmt.Sprintf("no iterator found for table: %v", table))
	}

	elem := memdbIterator.Next()
	if elem == nil {
		return slice, nil
	}

	firstElement, isOk := elem.(V)
	if !isOk {
		return nil, errors.New("first element not of provided type")
	}

	iterator := iteratorCreator(memdbIterator, upperBound, firstElement)

	for {
		record, err := iterator.Next()
		if err != nil {
			return nil, err
		}

		if record == nil {
			break
		}

		r, isOk := record.(V)
		if !isOk {
			return nil, errors.New("element not of provided type")
		}

		slice = append(slice, r)
	}

	return slice, nil
}

func getFromMemDB[V MemDBRecord](memdb *memdb.MemDB, table, indexName string, id interface{}) (V, error) {
	txn := memdb.Txn(false)
	defer txn.Abort()

	var record V

	result, err := txn.First(table, indexName, id)

	if err != nil {
		return record, err
	}

	if result != nil {
		r, isOk := result.(V)
		if !isOk {
			return record, errors.New("record not of given type")
		}

		record = r
	}

	return record, nil
}

func deleteFilteredFromMemDB(memdb *memdb.MemDB, table string, upperBound interface{}) error {
	txn := memdb.Txn(true)

	it, err := txn.ReverseLowerBound(table, "id", upperBound)
	if err != nil {
		txn.Abort()

		return err
	}

	// Put them into a slice so there are no safety concerns while actually
	// performing the deletes
	var records []interface{}

	for {
		obj := it.Next()
		if obj == nil {
			break
		}

		records = append(records, obj)
	}

	for _, r := range records {
		if err := txn.Delete(table, r); err != nil {
			return err
		}
	}

	txn.Commit()

	return nil
}

// State represents a persistence layer which persists consensus data off-chain
type State struct {
	db    *bolt.DB
	memdb *memdb.MemDB
	// mu is a lock used for parallel writing to memdb
	// since it does not support multiple writers at one time
	// see the link: https://pkg.go.dev/github.com/hashicorp/go-memdb#MemDB.Txn
	// only used for message votes
	mu sync.Mutex
}

// newState creates a new instance of State
func newState(path string) (*State, error) {
	db, err := bolt.Open(path, 0666, nil)
	if err != nil {
		return nil, err
	}

	err = initMainDBBuckets(db)
	if err != nil {
		return nil, err
	}

	memdb, err := memdb.NewMemDB(memStateSchema)
	if err != nil {
		return nil, err
	}

	state := &State{
		db:    db,
		memdb: memdb,
	}

	if err := state.populateMemdb(); err != nil {
		return nil, err
	}

	return state, nil
}

// populateMemdb populates memdb with data from boltDb
func (s *State) populateMemdb() error {
	if err := populateMemdbTable[*StateSyncEvent](s, stateSyncTable, syncStateEventsBucket); err != nil {
		return err
	}

	if err := populateMemdbTable[*ValidatorSnapshot](s, validatorSnapshotTable,
		validatorSnapshotsBucket); err != nil {
		return err
	}

	if err := populateMemdbTable[*CommitmentToExecute](s, commitmentTable, commitmentsBucket); err != nil {
		return err
	}

	return populateMemdbTable[*MessageVotes](s, messageVoteTable, messageVotesBucket)
}

// populateMemdbTable populates provided memdb table with data from boltDb
func populateMemdbTable[V MemDBRecord](s *State, table string, bucket []byte) error {
	records, err := list[V](s, bucket)
	if err != nil {
		return err
	}

	for _, r := range records {
		if err := insertToMemDB(s.memdb, table, r); err != nil {
			return err
		}
	}

	return nil
}

// list returns all records of given type from boltDb
func list[V MemDBRecord](s *State, bucket []byte) ([]V, error) {
	records := []V{}

	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucket).ForEach(func(k, v []byte) error {
			var record V
			if err := json.Unmarshal(v, &record); err != nil {
				return err
			}
			records = append(records, record)

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return records, nil
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
	snapshot := &ValidatorSnapshot{epoch, validatorSnapshot}
	if err := insertToMemDB(s.memdb, validatorSnapshotTable, snapshot); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}

		bucket := tx.Bucket(validatorSnapshotsBucket)

		return bucket.Put(convertToBytes(epoch), raw)
	})
}

// getValidatorSnapshot queries the validator snapshot for given epoch from db
func (s *State) getValidatorSnapshot(epoch uint64) (AccountSet, error) {
	memdbRecord, err := getFromMemDB[*ValidatorSnapshot](s.memdb, validatorSnapshotTable, "id", epoch)
	if err != nil {
		return nil, err
	}

	if memdbRecord != nil {
		return memdbRecord.AccountSet, nil
	}

	return nil, nil
}

// insertStateSyncEvent inserts a new state sync event to state event bucket in db
func (s *State) insertStateSyncEvent(event *StateSyncEvent) error {
	if err := insertToMemDB(s.memdb, stateSyncTable, event); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}

		bucket := tx.Bucket(syncStateEventsBucket)

		return bucket.Put(convertToBytes(event.ID), raw)
	})
}

// getStateSyncEventsForCommitment returns state sync events for commitment
// if there is an event with index that can not be found in db in given range, an error is returned
func (s *State) getStateSyncEventsForCommitment(fromIndex, toIndex uint64) ([]*StateSyncEvent, error) {
	return getFilteredFromMemDB[*StateSyncEvent](s.memdb, stateSyncTable, fromIndex, toIndex)
}

// insertCommitmentMessage inserts signed commitment to db
func (s *State) insertCommitmentMessage(commitment *CommitmentToExecute) error {
	if err := insertToMemDB(s.memdb, commitmentTable, commitment); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(commitment)
		if err != nil {
			return err
		}

		return tx.Bucket(commitmentsBucket).Put(convertToBytes(commitment.ToIndex), raw)
	})
}

// getNonExecutedCommitments gets non executed commitments
// (commitments whose toIndex is greater than or equal to startIndex)
func (s *State) getNonExecutedCommitments(startIndex uint64) ([]*CommitmentToExecute, error) {
	commitments, err := getFilteredFromMemDB[*CommitmentToExecute](s.memdb, commitmentTable, startIndex, math.MaxUint64)
	if err != nil {
		return nil, err
	}

	sort.Slice(commitments, func(i, j int) bool {
		return commitments[i].SignedCommitment.Message.FromIndex < commitments[j].SignedCommitment.Message.FromIndex
	})

	return commitments, err
}

// cleanCommitments cleans all commitments that are older than the provided fromIndex, alongside their proofs
func (s *State) cleanCommitments(stateSyncExecutionIndex uint64) error {
	if stateSyncExecutionIndex <= 1 {
		// small optimization, there is nothing to clean
		return nil
	}

	if err := deleteFilteredFromMemDB(s.memdb, commitmentTable, stateSyncExecutionIndex-1); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		commitmentsBucket := tx.Bucket(commitmentsBucket)
		commitmentsCursor := commitmentsBucket.Cursor()
		for k, _ := commitmentsCursor.First(); k != nil; k, _ = commitmentsCursor.Next() {
			if convertToUint64(k) >= stateSyncExecutionIndex {
				// reached a commitment that is not executed
				break
			}

			if err := commitmentsBucket.Delete(k); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertMessageVote inserts given vote to signatures bucket of given epoch
func (s *State) insertMessageVote(epoch uint64, hash string, vote *MessageSignature) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	votes, err := s.getMessageVotes(hash)

	if err != nil {
		return 0, err
	}

	if votes == nil {
		votes = &MessageVotes{
			Epoch:      epoch,
			Hash:       hash,
			Signatures: make([]*MessageSignature, 0),
		}
	}

	// check if the signature has already being included
	for _, sigs := range votes.Signatures {
		if sigs.From == vote.From {
			return len(votes.Signatures), nil
		}
	}

	votes.Signatures = append(votes.Signatures, vote)

	err = insertToMemDB(s.memdb, messageVoteTable, votes)
	if err != nil {
		return 0, err
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(votes)
		if err != nil {
			return err
		}

		bucket := tx.Bucket(messageVotesBucket)

		return bucket.Put([]byte(hash), raw)
	})

	if err != nil {
		return 0, err
	}

	return len(votes.Signatures), nil
}

// getMessageVotes gets all signatures from db associated with given epoch and hash
func (s *State) getMessageVotes(hash string) (*MessageVotes, error) {
	return getFromMemDB[*MessageVotes](s.memdb, messageVoteTable, "hash", hash)
}

// cleanPreviousEpochsDataFromDB cleans data from previous epochs from memdb and boltDb
func (s *State) cleanPreviousEpochsDataFromDB(currentEpoch uint64) error {
	if err := deleteFilteredFromMemDB(s.memdb, messageVoteTable, currentEpoch-1); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(messageVotesBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(messageVotesBucket)

		return err
	})
}

// cleanValidatorSnapshotsFromDB cleans the validator snapshots bucket if a limit is reached,
// but it leaves the latest (n) number of snapshots
func (s *State) cleanValidatorSnapshotsFromDB(epoch uint64) error {
	if numberOfSnapshotsToLeaveInDB < epoch {
		if err := deleteFilteredFromMemDB(s.memdb, messageVoteTable, epoch-numberOfSnapshotsToLeaveInDB); err != nil {
			return err
		}
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(validatorSnapshotsBucket)

		// paired list
		keys := make([][]byte, 0)
		values := make([][]byte, 0)
		for i := 0; i < numberOfSnapshotsToLeaveInDB; i++ { // exclude the last inserted we already appended
			key := convertToBytes(epoch)
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

// validatorSnapshotsDBStats returns stats of validators snapshot bucket in db
func (s *State) validatorSnapshotsDBStats() *bolt.BucketStats {
	return s.bucketStats(validatorSnapshotsBucket)
}

// bucketStats returns stats for the given bucket in db
func (s *State) bucketStats(bucketName []byte) *bolt.BucketStats {
	var stats *bolt.BucketStats

	s.db.View(func(tx *bolt.Tx) error {
		s := tx.Bucket(bucketName).Stats()
		stats = &s

		return nil
	})

	return stats
}

// convertToBytes converts uint64 to bytes array
func convertToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)

	return b
}

// convertToUint64 converts bytes array to uint64
func convertToUint64(v []byte) uint64 {
	return binary.BigEndian.Uint64(v)
}
