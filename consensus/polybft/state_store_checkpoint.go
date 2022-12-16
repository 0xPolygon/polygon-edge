package polybft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store exit contract events
	exitEventsBucket = []byte("exitEvent")
)

type exitEventNotFoundError struct {
	exitID uint64
	epoch  uint64
}

func (e *exitEventNotFoundError) Error() string {
	return fmt.Sprintf("could not find any exit event that has an id: %v and epoch: %v", e.exitID, e.epoch)
}

/*
exit events/
|--> (id+epoch+blockNumber) -> *ExitEvent (json marshalled)
*/
type CheckpointStore struct {
	db *bolt.DB
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

	return bucket.Put(bytes.Join([][]byte{itob(exitEvent.EpochNumber),
		itob(exitEvent.ID), itob(exitEvent.BlockNumber)}, nil), raw)
}

// getExitEvent returns exit event with given id, which happened in given epoch and given block number
func (s *CheckpointStore) getExitEvent(exitEventID, epoch uint64) (*ExitEvent, error) {
	var exitEvent *ExitEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(exitEventsBucket)

		key := bytes.Join([][]byte{itob(epoch), itob(exitEventID)}, nil)
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
