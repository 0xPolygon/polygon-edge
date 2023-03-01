package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store exit contract events
	exitEventsBucket = []byte("exitEvent")

	exitEventABI     = contractsapi.L2StateSender.Abi.Events["L2StateSynced"]
	ExitEventABIType = exitEventABI.Inputs
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
*/
type CheckpointStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *CheckpointStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(exitEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventsBucket), err)
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
		bucket := tx.Bucket(exitEventsBucket)
		for i := 0; i < len(exitEvents); i++ {
			if err := insertExitEventToBucket(bucket, exitEvents[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertExitEventToBucket inserts exit event to exit event bucket
func insertExitEventToBucket(bucket *bolt.Bucket, exitEvent *ExitEvent) error {
	raw, err := json.Marshal(exitEvent)
	if err != nil {
		return err
	}

	return bucket.Put(bytes.Join([][]byte{common.EncodeUint64ToBytes(exitEvent.EpochNumber),
		common.EncodeUint64ToBytes(exitEvent.ID), common.EncodeUint64ToBytes(exitEvent.BlockNumber)}, nil), raw)
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *CheckpointStore) getExitEvent(exitEventID, epoch uint64) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(exitEventsBucket)

		key := bytes.Join([][]byte{common.EncodeUint64ToBytes(epoch), common.EncodeUint64ToBytes(exitEventID)}, nil)
		k, v := bucket.Cursor().Seek(key)

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
	if !exitEventABI.Match(log) {
		// valid case, not an exit event
		return nil, nil
	}

	raw, err := exitEventABI.Inputs.ParseLog(log)
	if err != nil {
		return nil, err
	}

	eventGeneric, err := DecodeBridgeEventData(raw, log,
		func(id *big.Int, sender, receiver ethgo.Address, data []byte) interface{} {
			return &ExitEvent{
				ID:          id.Uint64(),
				Sender:      sender,
				Receiver:    receiver,
				Data:        data,
				EpochNumber: epoch,
				BlockNumber: block,
			}
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

// DecodeBridgeEventData decodes provided map of event metadata and
// creates a generic instance which is returned by eventCreator callback
func DecodeBridgeEventData(eventDataMap map[string]interface{}, log *ethgo.Log,
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
