package polybft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store exit contract events
	exitEventsBucket             = []byte("exitEvent")
	exitEventToEpochLookupBucket = []byte("exitIdToEpochLookup")
)

type exitEventNotFoundError struct {
	exitID uint64
	epoch  uint64
}

func (e *exitEventNotFoundError) Error() string {
	return fmt.Sprintf("could not find any exit event that has an id: %v and epoch: %v", e.exitID, e.epoch)
}

/*
Bolt DB schema:

exit events/
|--> (id+epoch+blockNumber) -> *ExitEvent (json marshalled)
|--> (exitEventID) -> epochNumber
*/
type CheckpointStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *CheckpointStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(exitEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(exitEventToEpochLookupBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventToEpochLookupBucket), err)
	}

	return nil
}

// insertExitEvents inserts a slice of exit events to exit event bucket in bolt db
func (s *CheckpointStore) insertExitEvents(exitEvents []*ExitEvent) error {
	if len(exitEvents) == 0 {
		// small optimization
		return nil
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		exitEventBucket := tx.Bucket(exitEventsBucket)
		lookupBucket := tx.Bucket(exitEventToEpochLookupBucket)
		for i := 0; i < len(exitEvents); i++ {
			if err := insertExitEventToBucket(exitEventBucket, lookupBucket, exitEvents[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertExitEventToBucket inserts exit event to exit event bucket
func insertExitEventToBucket(exitEventBucket, lookupBucket *bolt.Bucket, exitEvent *ExitEvent) error {
	raw, err := json.Marshal(exitEvent)
	if err != nil {
		return err
	}

	epochBytes := common.EncodeUint64ToBytes(exitEvent.EpochNumber)
	exitIDBytes := common.EncodeUint64ToBytes(exitEvent.ID)

	err = exitEventBucket.Put(bytes.Join([][]byte{epochBytes,
		exitIDBytes, common.EncodeUint64ToBytes(exitEvent.BlockNumber)}, nil), raw)
	if err != nil {
		return err
	}

	return lookupBucket.Put(exitIDBytes, epochBytes)
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *CheckpointStore) getExitEvent(exitEventID uint64) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		exitEventBucket := tx.Bucket(exitEventsBucket)
		lookupBucket := tx.Bucket(exitEventToEpochLookupBucket)

		exitIDBytes := common.EncodeUint64ToBytes(exitEventID)

		epochBytes := lookupBucket.Get(exitIDBytes)
		if epochBytes == nil {
			return fmt.Errorf("could not find any exit event that has an id: %v. Its epoch was not found in lookup table",
				exitEventID)
		}

		key := bytes.Join([][]byte{epochBytes, exitIDBytes}, nil)
		k, v := exitEventBucket.Cursor().Seek(key)

		if bytes.HasPrefix(k, key) == false || v == nil {
			return &exitEventNotFoundError{
				exitID: exitEventID,
				epoch:  common.EncodeBytesToUint64(epochBytes),
			}
		}

		return json.Unmarshal(v, &exitEvent)
	})

	return exitEvent, err
}

// getExitEventsByEpoch returns all exit events that happened in the given epoch
func (s *CheckpointStore) getExitEventsByEpoch(epoch uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch
	})
}

// getExitEventsForProof returns all exit events that happened in and prior to the given checkpoint block number
// with respect to the epoch in which block is added
func (s *CheckpointStore) getExitEventsForProof(epoch, checkpointBlock uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch && exitEvent.BlockNumber <= checkpointBlock
	})
}

// getExitEvents returns exit events for given epoch and provided filter
func (s *CheckpointStore) getExitEvents(epoch uint64, filter func(exitEvent *ExitEvent) bool) ([]*ExitEvent, error) {
	var events []*ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(exitEventsBucket).Cursor()
		prefix := common.EncodeUint64ToBytes(epoch)

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

// decodeExitEvent tries to decode exit event from the provided log
func decodeExitEvent(log *ethgo.Log, epoch, block uint64) (*ExitEvent, error) {
	var l2StateSyncedEvent contractsapi.L2StateSyncedEvent

	doesMatch, err := l2StateSyncedEvent.ParseLog(log)
	if err != nil {
		return nil, err
	}

	if !doesMatch {
		return nil, nil
	}

	return &ExitEvent{
		ID:          l2StateSyncedEvent.ID.Uint64(),
		Sender:      ethgo.Address(l2StateSyncedEvent.Sender),
		Receiver:    ethgo.Address(l2StateSyncedEvent.Receiver),
		Data:        l2StateSyncedEvent.Data,
		EpochNumber: epoch,
		BlockNumber: block,
	}, nil
}

// convertLog converts types.Log to ethgo.Log
func convertLog(log *types.Log) *ethgo.Log {
	l := &ethgo.Log{
		Address: ethgo.Address(log.Address),
		Data:    make([]byte, len(log.Data)),
		Topics:  make([]ethgo.Hash, len(log.Topics)),
	}

	copy(l.Data, log.Data)

	for i, topic := range log.Topics {
		l.Topics[i] = ethgo.Hash(topic)
	}

	return l
}
