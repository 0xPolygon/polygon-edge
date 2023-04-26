package polybft

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store stake changed events
	stakeChangeBucket = []byte("stakeChange")
)

type StakeStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *StakeStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(stakeChangeBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(epochsBucket), err)
	}

	return nil
}

// insertTransferEvents inserts provided transfer events to stake change bucket for given epoch
func (s *StakeStore) insertTransferEvents(epoch uint64, transferEvents []*contractsapi.TransferEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		eventsInDB, err := s.getTransferEventsLocked(tx, epoch)
		if err != nil {
			return err
		}

		eventsInDB = append(eventsInDB, transferEvents...)

		raw, err := json.Marshal(eventsInDB)
		if err != nil {
			return err
		}

		return tx.Bucket(stakeChangeBucket).Put(common.EncodeUint64ToBytes(epoch), raw)
	})
}

// getTransferEvents returns a slice of transfer events (stake changed events) that happened in given epoch
func (s *StakeStore) getTransferEvents(epoch uint64) ([]*contractsapi.TransferEvent, error) {
	var transferEvents []*contractsapi.TransferEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		res, err := s.getTransferEventsLocked(tx, epoch)
		if err != nil {
			return err
		}
		transferEvents = res

		return nil
	})

	return transferEvents, err
}

// getTransferEventsLocked gets all transfer events from db associated with given epoch
func (s *StakeStore) getTransferEventsLocked(tx *bolt.Tx, epoch uint64) ([]*contractsapi.TransferEvent, error) {
	bucket := tx.Bucket(stakeChangeBucket)

	v := bucket.Get(common.EncodeUint64ToBytes(epoch))
	if v == nil {
		return nil, nil
	}

	var transferEvents []*contractsapi.TransferEvent
	if err := json.Unmarshal(v, &transferEvents); err != nil {
		return nil, err
	}

	return transferEvents, nil
}
