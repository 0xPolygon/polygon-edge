package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	bolt "go.etcd.io/bbolt"
)

var (
	governanceEventsBucket              = []byte("governanceEvents")
	clientConfigBucket                  = []byte("clientConfig")
	clientConfigKey                     = []byte("clientConfigKey")
	lastProcessedGovernanceEventsBucket = []byte("lastProcessedGovernanceEvents")
	lastProcessedGovernanceBlockKey     = []byte("lastProcessedGovernanceBlockKey")

	errNoLastProcessedGovernanceBlock = errors.New("no last processed block for governance in db")
	errClientConfigNotFound           = errors.New("client (polybft) config not found in db")
)

type eventsRaw [][]byte

// Bolt db schema:
//
// governance events/
// |--> epoch -> slice of contractsapi.EventAbi
type GovernanceStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (g *GovernanceStore) initialize(tx *bolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(governanceEventsBucket)
	if err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(governanceEventsBucket), err)
	}

	_, err = tx.CreateBucketIfNotExists(clientConfigBucket)
	if err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w",
			string(clientConfigBucket), err)
	}

	_, err = tx.CreateBucketIfNotExists(lastProcessedGovernanceEventsBucket)
	if err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w",
			string(lastProcessedGovernanceEventsBucket), err)
	}

	if val := tx.Bucket(lastProcessedGovernanceEventsBucket).Get(
		lastProcessedGovernanceBlockKey); val == nil {
		return tx.Bucket(lastProcessedGovernanceEventsBucket).Put(
			lastProcessedGovernanceBlockKey, common.EncodeUint64ToBytes(0))
	}

	return nil
}

// insertGovernanceEvents inserts governance events to bolt db
// each epoch has a list of events that happened in it
func (g *GovernanceStore) insertGovernanceEvents(epoch, block uint64, events []contractsapi.EventAbi) error {
	return g.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(governanceEventsBucket)
		epochKey := common.EncodeUint64ToBytes(epoch)

		var rawEvents eventsRaw

		val := bucket.Get(epochKey)
		if val != nil {
			if err := json.Unmarshal(val, &rawEvents); err != nil {
				return err
			}
		}

		for _, event := range events {
			rawEvent, err := governanceEventToByteArray(event)
			if err != nil {
				return err
			}

			rawEvents = append(rawEvents, rawEvent)
		}

		val, err := json.Marshal(rawEvents)
		if err != nil {
			return err
		}

		if err = bucket.Put(epochKey, val); err != nil {
			return err
		}

		// update the last processed block
		return tx.Bucket(lastProcessedGovernanceEventsBucket).Put(
			lastProcessedGovernanceBlockKey, common.EncodeUint64ToBytes(block))
	})
}

// getGovernanceEvents returns a list of governance events that happened in given epoch
// it is valid that epoch has no events if there was no governance proposal executed in it
func (g *GovernanceStore) getGovernanceEvents(epoch uint64) (eventsRaw, error) {
	var rawEvents eventsRaw

	err := g.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(governanceEventsBucket).Get(common.EncodeUint64ToBytes(epoch))
		if val != nil {
			return json.Unmarshal(val, &rawEvents)
		}

		return nil // this is valid, since we might not have any proposals executed in given epoch
	})

	return rawEvents, err
}

// getLastSaved returns the last processed block for governance events
func (g *GovernanceStore) getLastSaved() (uint64, error) {
	var lastProcessedBlock uint64

	err := g.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(lastProcessedGovernanceEventsBucket).Get(lastProcessedGovernanceBlockKey)
		if val == nil {
			return errNoLastProcessedGovernanceBlock
		}

		lastProcessedBlock = common.EncodeBytesToUint64(val)

		return nil
	})

	return lastProcessedBlock, err
}

// insertClientConfig inserts client (polybft) config to bolt db
func (g *GovernanceStore) insertClientConfig(config *PolyBFTConfig) error {
	return g.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(config)
		if err != nil {
			return err
		}

		return tx.Bucket(clientConfigBucket).Put(clientConfigKey, raw)
	})
}

// getClientConfig returns client (polybft) config from bolt db
func (g *GovernanceStore) getClientConfig() (*PolyBFTConfig, error) {
	var config *PolyBFTConfig

	err := g.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(clientConfigBucket).Get(clientConfigKey)
		if val == nil {
			return errClientConfigNotFound
		}

		return json.Unmarshal(val, &config)
	})

	return config, err
}

// governanceEventToByteArray marshals event but adds it's signature
// to the beginning of marshaled array so that later we can know which type of event it is
func governanceEventToByteArray(event contractsapi.EventAbi) ([]byte, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	return bytes.Join([][]byte{event.Sig().Bytes(), raw}, nil), nil
}
