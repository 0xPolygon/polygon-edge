package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	polyCommon "github.com/0xPolygon/polygon-edge/consensus/polybft/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	bolt "go.etcd.io/bbolt"
)

var (
	networkParamsEventsBucket          = []byte("networkParamsEvents")
	forkParamsEventsBucket             = []byte("forkParamsEvents")
	clientConfigBucket                 = []byte("clientConfig")
	clientConfigKey                    = []byte("clientConfigKey")
	lastProcessedGovernanceBlockBucket = []byte("lastProcessedGovernanceBlock")
	lastProcessedGovernanceBlockKey    = []byte("lastProcessedGovernanceBlockKey")

	errNoLastProcessedGovernanceBlock = errors.New("no last processed block for governance in db")
	errClientConfigNotFound           = errors.New("client (polybft) config not found in db")
)

type eventsRaw [][]byte

// Bolt db schema:
//
// governance events/
// |--> epoch -> slice of contractsapi.EventAbi
// |--> fork name hash -> block from which is active
// |--> clientConfigKey -> *PolyBFTConfig
// |--> lastProcessedGovernanceBlockKey -> blockNumber
type GovernanceStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (g *GovernanceStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(networkParamsEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w",
			string(networkParamsEventsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(forkParamsEventsBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w",
			string(forkParamsEventsBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(clientConfigBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w",
			string(clientConfigBucket), err)
	}

	if _, err := tx.CreateBucketIfNotExists(lastProcessedGovernanceBlockBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w",
			string(lastProcessedGovernanceBlockBucket), err)
	}

	if val := tx.Bucket(lastProcessedGovernanceBlockBucket).Get(
		lastProcessedGovernanceBlockKey); val == nil {
		return tx.Bucket(lastProcessedGovernanceBlockBucket).Put(
			lastProcessedGovernanceBlockKey, common.EncodeUint64ToBytes(0))
	}

	return nil
}

// insertGovernanceEvents inserts governance events to bolt db
// each epoch has a list of events that happened in it
func (g *GovernanceStore) insertGovernanceEvents(epoch, block uint64, events []contractsapi.EventAbi) error {
	return g.db.Update(func(tx *bolt.Tx) error {
		networkParamsBucket := tx.Bucket(networkParamsEventsBucket)
		forkParamsBucket := tx.Bucket(forkParamsEventsBucket)
		epochKey := common.EncodeUint64ToBytes(epoch)

		var rawEvents eventsRaw

		val := networkParamsBucket.Get(epochKey)
		if val != nil {
			if err := json.Unmarshal(val, &rawEvents); err != nil {
				return err
			}
		}

		for _, event := range events {
			if forkHash, block, isForkEvent := isForkParamsEvent(event); isForkEvent {
				// we save fork events to different bucket
				if err := forkParamsBucket.Put(forkHash.Bytes(), block.Bytes()); err != nil {
					return err
				}

				continue
			}

			rawEvent, err := networkParamsEventToByteArray(event)
			if err != nil {
				return err
			}

			rawEvents = append(rawEvents, rawEvent)
		}

		val, err := json.Marshal(rawEvents)
		if err != nil {
			return err
		}

		if err = networkParamsBucket.Put(epochKey, val); err != nil {
			return err
		}

		// update the last processed block
		return tx.Bucket(lastProcessedGovernanceBlockBucket).Put(
			lastProcessedGovernanceBlockKey, common.EncodeUint64ToBytes(block))
	})
}

// getNetworkParamsEvents returns a list of NetworkParams contract events that happened in given epoch
// it is valid that epoch has no events if there was no governance proposal executed in it
func (g *GovernanceStore) getNetworkParamsEvents(epoch uint64) (eventsRaw, error) {
	var rawEvents eventsRaw

	err := g.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(networkParamsEventsBucket).Get(common.EncodeUint64ToBytes(epoch))
		if val != nil {
			return json.Unmarshal(val, &rawEvents)
		}

		return nil // this is valid, since we might not have any proposals executed in given epoch
	})

	return rawEvents, err
}

// getAllForkEvents returns a list of all forks and their activation block
func (g *GovernanceStore) getAllForkEvents() (map[types.Hash]*big.Int, error) {
	allForks := map[types.Hash]*big.Int{}

	err := g.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(forkParamsEventsBucket).ForEach(func(k, v []byte) error {
			allForks[types.BytesToHash(k)] = new(big.Int).SetBytes(v)

			return nil
		})
	})

	return allForks, err
}

// insertLastProcessed inserts last processed block for governance events
func (g *GovernanceStore) insertLastProcessed(blockNumber uint64) error {
	return g.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(lastProcessedGovernanceBlockBucket).Put(
			lastProcessedGovernanceBlockKey, common.EncodeUint64ToBytes(blockNumber))
	})
}

// getLastProcessed returns the last processed block for governance events
func (g *GovernanceStore) getLastProcessed() (uint64, error) {
	var lastProcessedBlock uint64

	err := g.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(lastProcessedGovernanceBlockBucket).Get(lastProcessedGovernanceBlockKey)
		if val == nil {
			return errNoLastProcessedGovernanceBlock
		}

		lastProcessedBlock = common.EncodeBytesToUint64(val)

		return nil
	})

	return lastProcessedBlock, err
}

// insertClientConfig inserts client (polybft) config to bolt db
func (g *GovernanceStore) insertClientConfig(config *polyCommon.PolyBFTConfig) error {
	return g.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(config)
		if err != nil {
			return err
		}

		return tx.Bucket(clientConfigBucket).Put(clientConfigKey, raw)
	})
}

// getClientConfig returns client (polybft) config from bolt db
func (g *GovernanceStore) getClientConfig() (*polyCommon.PolyBFTConfig, error) {
	var config *polyCommon.PolyBFTConfig

	err := g.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(clientConfigBucket).Get(clientConfigKey)
		if val == nil {
			return errClientConfigNotFound
		}

		return json.Unmarshal(val, &config)
	})

	return config, err
}

// networkParamsEventToByteArray marshals event but adds it's signature
// to the beginning of marshaled array so that later we can know which type of event it is
func networkParamsEventToByteArray(event contractsapi.EventAbi) ([]byte, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	return bytes.Join([][]byte{event.Sig().Bytes(), raw}, nil), nil
}
