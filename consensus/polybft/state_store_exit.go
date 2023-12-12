package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	exitEventsBucket             = []byte("exitEvent")
	exitEventToEpochLookupBucket = []byte("exitIdToEpochLookup")
	exitRelayerEventsBucket      = []byte("exitRelayerEvents")

	errNoLastSavedEntry = errors.New("there is no last saved block in last saved bucket")
)

type exitEventNotFoundError struct {
	exitID uint64
	epoch  uint64
}

func (e *exitEventNotFoundError) Error() string {
	return fmt.Sprintf("could not find any exit event that has an id: %v and epoch: %v", e.exitID, e.epoch)
}

// ExitEvent is an event emitted by Exit contract
type ExitEvent struct {
	*contractsapi.L2StateSyncedEvent
	// EpochNumber is the epoch number in which exit event was added
	EpochNumber uint64 `abi:"-"`
	// BlockNumber is the block in which exit event was added
	BlockNumber uint64 `abi:"-"`
}

/*
Bolt DB schema:

exit events/
|--> (id+epoch+blockNumber) -> *ExitEvent (json marshalled)
|--> (exitEventID) -> epochNumber
relayerEvents/
|--> RelayerEventData.EventID -> *RelayerEventData (json marshalled)
*/
type ExitStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *ExitStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(exitEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(exitEventToEpochLookupBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventToEpochLookupBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(exitRelayerEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitRelayerEventsBucket), err)
	}

	return nil
}

// insertExitEventWithTx inserts an exit event to db
func (s *ExitStore) insertExitEvent(exitEvent *ExitEvent, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := json.Marshal(exitEvent)
		if err != nil {
			return err
		}

		exitEventKey := generateExitEventKey(exitEvent.ID.Uint64(), exitEvent.EpochNumber, exitEvent.BlockNumber)

		err = tx.Bucket(exitEventsBucket).Put(exitEventKey, raw)
		if err != nil {
			return err
		}

		return tx.Bucket(exitEventToEpochLookupBucket).Put(
			common.EncodeUint64ToBytes(exitEvent.ID.Uint64()),
			common.EncodeUint64ToBytes(exitEvent.EpochNumber))
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *ExitStore) getExitEvent(exitEventID uint64) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		e, err := getExitEventSingle(exitEventID, tx)
		if err != nil {
			return err
		}

		exitEvent = e

		return nil
	})

	return exitEvent, err
}

// getExitEventSingle returns an exit event from db based on its ID
func getExitEventSingle(exitEventID uint64, tx *bolt.Tx) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	exitEventBucket := tx.Bucket(exitEventsBucket)
	lookupBucket := tx.Bucket(exitEventToEpochLookupBucket)

	exitIDBytes := common.EncodeUint64ToBytes(exitEventID)

	epochBytes := lookupBucket.Get(exitIDBytes)
	if epochBytes == nil {
		return nil, fmt.Errorf("could not find any exit event that has an id: %v. Its epoch was not found in lookup table",
			exitEventID)
	}

	key := bytes.Join([][]byte{epochBytes, exitIDBytes}, nil)
	k, v := exitEventBucket.Cursor().Seek(key)

	if bytes.HasPrefix(k, key) == false || v == nil {
		return nil, &exitEventNotFoundError{
			exitID: exitEventID,
			epoch:  common.EncodeBytesToUint64(epochBytes),
		}
	}

	if err := json.Unmarshal(v, &exitEvent); err != nil {
		return nil, err
	}

	return exitEvent, nil
}

// getExitEventsByEpoch returns all exit events that happened in the given epoch
func (s *ExitStore) getExitEventsByEpoch(epoch uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch
	})
}

// getExitEventsForProof returns all exit events that happened in and prior to the given checkpoint block number
// with respect to the epoch in which block is added
func (s *ExitStore) getExitEventsForProof(epoch, checkpointBlock uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch && exitEvent.BlockNumber <= checkpointBlock
	})
}

// getExitEvents returns exit events for given epoch and provided filter
func (s *ExitStore) getExitEvents(epoch uint64, filter func(exitEvent *ExitEvent) bool) ([]*ExitEvent, error) {
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
		return events[i].ID.Cmp(events[j].ID) < 0
	})

	return events, err
}

// getAllAvailableRelayerEvents retrieves all Exit RelayerEventData that should be sent as a transactions
func (s *ExitStore) GetAllAvailableRelayerEvents(limit int) (result []*RelayerEventMetaData, err error) {
	if err = s.db.View(func(tx *bolt.Tx) error {
		result, err = getAvailableRelayerEvents(limit, exitRelayerEventsBucket, tx)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// updateRelayerEvents updates/remove desired exit relayer events
func (s *ExitStore) UpdateRelayerEvents(
	events []*RelayerEventMetaData, removeIDs []uint64, dbTx *bolt.Tx) error {
	return updateRelayerEvents(exitRelayerEventsBucket, events, removeIDs, s.db, dbTx)
}

func generateExitEventKey(exitEventID, epoch, blockNumber uint64) []byte {
	return bytes.Join([][]byte{
		common.EncodeUint64ToBytes(epoch),
		common.EncodeUint64ToBytes(exitEventID),
		common.EncodeUint64ToBytes(blockNumber)}, nil)
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
		L2StateSyncedEvent: &l2StateSyncedEvent,
		EpochNumber:        epoch,
		BlockNumber:        block,
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
