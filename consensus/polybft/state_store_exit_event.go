package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	exitEventsBucket                  = []byte("exitEvent")
	slashingExitEventsBucket          = []byte("slashingExitEvent")
	exitEventToEpochLookupBucket      = []byte("exitIdToEpochLookup")
	exitEventLastProcessedBlockBucket = []byte("lastProcessedBlock")

	lastProcessedBlockKey = []byte("lastProcessedBlock")
	errNoLastSavedEntry   = errors.New("there is no last saved block in last saved bucket")

	slashSignature = crypto.Keccak256([]byte("SLASH"))
)

type exitEventNotFoundError struct {
	exitID uint64
	epoch  uint64
}

func (e *exitEventNotFoundError) Error() string {
	return fmt.Sprintf("could not find any exit event that has an id: %v and epoch: %v", e.exitID, e.epoch)
}

// ExitEvent is an event emitted by L2StateSender contract
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
|--> (epoch+exit event id+blockNumber) -> *ExitEvent (json marshalled)
|--> (exit event id) -> nil (slashing exit events)
|--> (exitEventID) -> epochNumber
|--> (lastProcessedBlockKey) -> block number
*/
type ExitEventStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *ExitEventStore) initialize(tx *bolt.Tx, blockNumber uint64) error {
	if _, err := tx.CreateBucketIfNotExists(exitEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventsBucket), err)
	}

	if forkmanager.GetInstance().IsForkEnabled(chain.DoubleSignSlashing, blockNumber) {
		if _, err := tx.CreateBucketIfNotExists(slashingExitEventsBucket); err != nil {
			return fmt.Errorf("failed to create bucket=%s: %w", string(slashingExitEventsBucket), err)
		}
	}

	if _, err := tx.CreateBucketIfNotExists(exitEventToEpochLookupBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventToEpochLookupBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(exitEventLastProcessedBlockBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(exitEventLastProcessedBlockBucket), err)
	}

	return tx.Bucket(exitEventLastProcessedBlockBucket).Put(lastProcessedBlockKey, common.EncodeUint64ToBytes(0))
}

// insertExitEvents inserts a slice of exit events to exit event bucket in bolt db
func (s *ExitEventStore) insertExitEvents(exitEvents []*ExitEvent, blockNumber uint64) error {
	if len(exitEvents) == 0 {
		// small optimization
		return nil
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		exitEventBucket := tx.Bucket(exitEventsBucket)
		lookupBucket := tx.Bucket(exitEventToEpochLookupBucket)

		doubleSlashingEnabled := forkmanager.GetInstance().IsForkEnabled(chain.DoubleSignSlashing, blockNumber)

		var slashExitEventBucket *bolt.Bucket
		for _, exitEvent := range exitEvents {
			exitIDRaw := common.EncodeUint64ToBytes(exitEvent.ID.Uint64())

			if doubleSlashingEnabled {
				signature := exitEvent.Data[:types.HashLength]
				if bytes.Equal(signature, slashSignature) {
					// create slash exit events bucket lazily
					if slashExitEventBucket == nil {
						slashExitEventBucket = tx.Bucket(slashingExitEventsBucket)
					}

					if err := slashExitEventBucket.Put(exitIDRaw, nil); err != nil {
						return err
					}
				}
			}

			if err := insertExitEvent(exitEventBucket, lookupBucket, exitIDRaw, exitEvent); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertExitEvent inserts exit event to exit event bucket
func insertExitEvent(exitEventBucket, lookupBucket *bolt.Bucket, exitIDRaw []byte, exitEvent *ExitEvent) error {
	raw, err := json.Marshal(exitEvent)
	if err != nil {
		return err
	}

	epochRaw := common.EncodeUint64ToBytes(exitEvent.EpochNumber)
	blockNumRaw := common.EncodeUint64ToBytes(exitEvent.BlockNumber)

	if err = exitEventBucket.Put(bytes.Join([][]byte{epochRaw, exitIDRaw, blockNumRaw}, nil), raw); err != nil {
		return err
	}

	return lookupBucket.Put(exitIDRaw, epochRaw)
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *ExitEventStore) getExitEvent(exitEventID uint64) (*ExitEvent, error) {
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
func (s *ExitEventStore) getExitEventsByEpoch(epoch uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch
	})
}

// getExitEventsForProof returns all exit events that happened in and prior to the given checkpoint block number
// with respect to the epoch in which block is added
func (s *ExitEventStore) getExitEventsForProof(epoch, checkpointBlock uint64) ([]*ExitEvent, error) {
	return s.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
		return exitEvent.EpochNumber == epoch && exitEvent.BlockNumber <= checkpointBlock
	})
}

// getExitEvents returns exit events for given epoch and provided filter
func (s *ExitEventStore) getExitEvents(epoch uint64, filter func(exitEvent *ExitEvent) bool) ([]*ExitEvent, error) {
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

// getPendingSlashExitIDs returns pending slash exit event ids
func (s *ExitEventStore) getPendingSlashExitIDs() ([]uint64, error) {
	var exitIds []uint64

	if err := s.db.View(func(tx *bolt.Tx) error {
		slashingEventsBucket := tx.Bucket(slashingExitEventsBucket)

		return slashingEventsBucket.ForEach(func(k, _ []byte) error {
			exitIds = append(exitIds, common.EncodeBytesToUint64(k))

			return nil
		})
	}); err != nil {
		return nil, err
	}

	sort.Slice(exitIds, func(i, j int) bool {
		return exitIds[i] < exitIds[j]
	})

	return exitIds, nil
}

// getExitEventsByIds returns exit events by its ids
func (s *ExitEventStore) getExitEventsByIds(exitEventIDs ...uint64) ([]*ExitEvent, error) {
	events := make([]*ExitEvent, 0, len(exitEventIDs))

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(exitEventsBucket).Cursor()
		for _, id := range exitEventIDs {
			prefix := common.EncodeUint64ToBytes(id)

			for k, v := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, v = c.Next() {
				var event *ExitEvent
				if err := json.Unmarshal(v, &event); err != nil {
					return err
				}

				events = append(events, event)
			}
		}

		return nil
	})

	return events, err
}

// updateLastSaved saves the last block processed for exit events
func (s *ExitEventStore) updateLastSaved(blockNumber uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(exitEventLastProcessedBlockBucket).Put(lastProcessedBlockKey,
			common.EncodeUint64ToBytes(blockNumber))
	})
}

// updateLastSaved saves the last block processed for exit events
func (s *ExitEventStore) getLastSaved() (uint64, error) {
	var lastSavedBlock uint64

	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(exitEventLastProcessedBlockBucket).Get(lastProcessedBlockKey)
		if v == nil {
			return errNoLastSavedEntry
		}

		lastSavedBlock = common.EncodeBytesToUint64(v)

		return nil
	})

	return lastSavedBlock, err
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
