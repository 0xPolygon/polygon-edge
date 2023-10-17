package polybft

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store rootchain bridge events
	stateSyncEventsBucket = []byte("stateSyncEvents")
	// bucket to store commitments
	commitmentsBucket = []byte("commitments")
	// bucket to store state sync proofs
	stateSyncProofsBucket = []byte("stateSyncProofs")
	// bucket to store message votes (signatures)
	messageVotesBucket = []byte("votes")
	// bucket to store all state sync relayer events
	stateSyncRelayerEventsBucket = []byte("relayerEvents")

	// errNotEnoughStateSyncs error message
	errNotEnoughStateSyncs = errors.New("there is either a gap or not enough sync events")
	// errCommitmentNotBuilt error message
	errCommitmentNotBuilt = errors.New("there is no built commitment to register")
	// errNoCommitmentForStateSync error message
	errNoCommitmentForStateSync = errors.New("no commitment found for given state sync event")
)

/*
Bolt DB schema:

state sync events/
|--> stateSyncEvent.Id -> *StateSyncEvent (json marshalled)

commitments/
|--> commitment.Message.ToIndex -> *CommitmentMessageSigned (json marshalled)

stateSyncProofs/
|--> stateSyncProof.StateSync.Id -> *StateSyncProof (json marshalled)

relayerEvents/
|--> StateSyncRelayerEventData.EventID -> *StateSyncRelayerEventData (json marshalled)
*/

type StateSyncStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *StateSyncStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(stateSyncEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(stateSyncEventsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(commitmentsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(commitmentsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(stateSyncProofsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(stateSyncProofsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(stateSyncRelayerEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(stateSyncRelayerEventsBucket), err)
	}

	return nil
}

// insertStateSyncEvent inserts a new state sync event to state event bucket in db
func (s *StateSyncStore) insertStateSyncEvent(event *contractsapi.StateSyncedEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}

		bucket := tx.Bucket(stateSyncEventsBucket)

		return bucket.Put(common.EncodeUint64ToBytes(event.ID.Uint64()), raw)
	})
}

// removeStateSyncEventsAndProofs removes state sync events and their proofs from the buckets in db
func (s *StateSyncStore) removeStateSyncEventsAndProofs(stateSyncEventIDs []uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		eventsBucket := tx.Bucket(stateSyncEventsBucket)
		proofsBucket := tx.Bucket(stateSyncProofsBucket)

		for _, stateSyncEventID := range stateSyncEventIDs {
			stateSyncEventIDKey := common.EncodeUint64ToBytes(stateSyncEventID)

			if err := eventsBucket.Delete(stateSyncEventIDKey); err != nil {
				return fmt.Errorf("failed to remove state sync event (ID=%d): %w", stateSyncEventID, err)
			}

			if err := proofsBucket.Delete(stateSyncEventIDKey); err != nil {
				return fmt.Errorf("failed to remove state sync event proof (ID=%d): %w", stateSyncEventID, err)
			}
		}

		return nil
	})
}

// list iterates through all events in events bucket in db, un-marshals them, and returns as array
func (s *StateSyncStore) list() ([]*contractsapi.StateSyncedEvent, error) {
	events := []*contractsapi.StateSyncedEvent{}

	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(stateSyncEventsBucket).ForEach(func(k, v []byte) error {
			var event *contractsapi.StateSyncedEvent
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

// getStateSyncEventsForCommitment returns state sync events for commitment
func (s *StateSyncStore) getStateSyncEventsForCommitment(
	fromIndex, toIndex uint64, dbTx *bolt.Tx) ([]*contractsapi.StateSyncedEvent, error) {
	var (
		events []*contractsapi.StateSyncedEvent
		err    error
	)

	getFn := func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateSyncEventsBucket)
		for i := fromIndex; i <= toIndex; i++ {
			v := bucket.Get(common.EncodeUint64ToBytes(i))
			if v == nil {
				return errNotEnoughStateSyncs
			}

			var event *contractsapi.StateSyncedEvent
			if err := json.Unmarshal(v, &event); err != nil {
				return err
			}

			events = append(events, event)
		}

		return nil
	}

	if dbTx == nil {
		err = s.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

	return events, err
}

// getCommitmentForStateSync returns the commitment that contains given state sync event if it exists
func (s *StateSyncStore) getCommitmentForStateSync(stateSyncID uint64) (*CommitmentMessageSigned, error) {
	var commitment *CommitmentMessageSigned

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(commitmentsBucket).Cursor()

		k, v := c.Seek(common.EncodeUint64ToBytes(stateSyncID))
		if k == nil {
			return errNoCommitmentForStateSync
		}

		if err := json.Unmarshal(v, &commitment); err != nil {
			return err
		}

		if !commitment.ContainsStateSync(stateSyncID) {
			return errNoCommitmentForStateSync
		}

		return nil
	})

	return commitment, err
}

// insertCommitmentMessage inserts signed commitment to db
func (s *StateSyncStore) insertCommitmentMessage(commitment *CommitmentMessageSigned,
	dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := json.Marshal(commitment)
		if err != nil {
			return err
		}

		if err := tx.Bucket(commitmentsBucket).Put(
			common.EncodeUint64ToBytes(commitment.Message.EndID.Uint64()), raw); err != nil {
			return err
		}

		return nil
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getCommitmentMessage queries the signed commitment from the db
func (s *StateSyncStore) getCommitmentMessage(toIndex uint64) (*CommitmentMessageSigned, error) {
	var commitment *CommitmentMessageSigned

	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(commitmentsBucket).Get(common.EncodeUint64ToBytes(toIndex))
		if raw == nil {
			return nil
		}

		return json.Unmarshal(raw, &commitment)
	})

	return commitment, err
}

// insertMessageVote inserts given vote to signatures bucket of given epoch
func (s *StateSyncStore) insertMessageVote(epoch uint64, key []byte,
	vote *MessageSignature, dbTx *bolt.Tx) (int, error) {
	var (
		numOfSignatures int
		err             error
	)

	insertFn := func(tx *bolt.Tx) error {
		signatures, err := s.getMessageVotesLocked(tx, epoch, key)
		if err != nil {
			return err
		}

		// check if the signature has already being included
		for _, sigs := range signatures {
			if sigs.From == vote.From {
				return nil
			}
		}

		if signatures == nil {
			signatures = []*MessageSignature{vote}
		} else {
			signatures = append(signatures, vote)
		}

		raw, err := json.Marshal(signatures)
		if err != nil {
			return err
		}

		bucket, err := getNestedBucketInEpoch(tx, epoch, messageVotesBucket)
		if err != nil {
			return err
		}

		numOfSignatures = len(signatures)

		return bucket.Put(key, raw)
	}

	if dbTx == nil {
		err = s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	} else {
		err = insertFn(dbTx)
	}

	return numOfSignatures, err
}

// getMessageVotes gets all signatures from db associated with given epoch and hash
func (s *StateSyncStore) getMessageVotes(epoch uint64, hash []byte) ([]*MessageSignature, error) {
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
func (s *StateSyncStore) getMessageVotesLocked(tx *bolt.Tx, epoch uint64,
	hash []byte) ([]*MessageSignature, error) {
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

// insertStateSyncProofs inserts the provided state sync proofs to db
func (s *StateSyncStore) insertStateSyncProofs(stateSyncProof []*StateSyncProof, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateSyncProofsBucket)

		for _, ssp := range stateSyncProof {
			raw, err := json.Marshal(ssp)
			if err != nil {
				return err
			}

			if err := bucket.Put(common.EncodeUint64ToBytes(ssp.StateSync.ID.Uint64()), raw); err != nil {
				return err
			}
		}

		return nil
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getStateSyncProof gets state sync proof that are not executed
func (s *StateSyncStore) getStateSyncProof(stateSyncID uint64) (*StateSyncProof, error) {
	var ssp *StateSyncProof

	err := s.db.View(func(tx *bolt.Tx) error {
		if v := tx.Bucket(stateSyncProofsBucket).Get(common.EncodeUint64ToBytes(stateSyncID)); v != nil {
			if err := json.Unmarshal(v, &ssp); err != nil {
				return err
			}
		}

		return nil
	})

	return ssp, err
}

// updateStateSyncRelayerEvents updates/remove desired events
func (s *StateSyncStore) updateStateSyncRelayerEvents(
	events []*StateSyncRelayerEventData, removeIDs []uint64, dbTx *bolt.Tx) error {
	updateFn := func(tx *bolt.Tx) error {
		relayerEventsBucket := tx.Bucket(stateSyncRelayerEventsBucket)

		for _, evnt := range events {
			raw, err := json.Marshal(evnt)
			if err != nil {
				return err
			}

			key := common.EncodeUint64ToBytes(evnt.EventID)

			if err := relayerEventsBucket.Put(key, raw); err != nil {
				return err
			}
		}

		for _, stateSyncEventID := range removeIDs {
			stateSyncEventIDKey := common.EncodeUint64ToBytes(stateSyncEventID)

			if err := relayerEventsBucket.Delete(stateSyncEventIDKey); err != nil {
				return fmt.Errorf("failed to remove state sync relayer event (ID=%d): %w", stateSyncEventID, err)
			}
		}

		return nil
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return updateFn(tx)
		})
	}

	return updateFn(dbTx)
}

// getAllAvailableEvents retrieves all StateSyncRelayerEventData that should be sent as a transactions
func (s *StateSyncStore) getAllAvailableEvents(limit int) (result []*StateSyncRelayerEventData, err error) {
	if err = s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(stateSyncRelayerEventsBucket).Cursor()

		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			var event *StateSyncRelayerEventData

			if err := json.Unmarshal(v, &event); err != nil {
				return err
			}

			result = append(result, event)

			if limit > 0 && len(result) >= limit {
				break
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}
