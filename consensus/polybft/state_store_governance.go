package polybft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	bolt "go.etcd.io/bbolt"
)

var (
	networkParamsEventsBucket = []byte("networkParamsEvents")
	forkParamsEventsBucket    = []byte("forkParamsEvents")
	clientConfigBucket        = []byte("clientConfig")
	clientConfigKey           = []byte("clientConfigKey")

	errClientConfigNotFound = errors.New("client (polybft) config not found in db")
)

type eventsRaw [][]byte

// Bolt db schema:
//
// governance events/
// |--> epoch -> slice of contractsapi.EventAbi
// |--> fork name hash -> block from which is active
// |--> clientConfigKey -> *PolyBFTConfig
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

	return nil
}

// insertGovernanceEvent inserts governance event to bolt db
// each epoch has a list of events that happened in it
func (g *GovernanceStore) insertGovernanceEvent(epoch, block uint64,
	event contractsapi.EventAbi, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		if forkHash, forkBlock, isForkEvent := isForkParamsEvent(event); isForkEvent {
			forkParamsBucket := tx.Bucket(forkParamsEventsBucket)

			return forkParamsBucket.Put(forkHash.Bytes(), forkBlock.Bytes())
		}

		var rawEvents eventsRaw

		networkParamsBucket := tx.Bucket(networkParamsEventsBucket)
		epochKey := common.EncodeUint64ToBytes(epoch)

		val := networkParamsBucket.Get(epochKey)
		if val != nil {
			if err := json.Unmarshal(val, &rawEvents); err != nil {
				return err
			}
		}

		rawEvent, err := networkParamsEventToByteArray(event)
		if err != nil {
			return err
		}

		rawEvents = append(rawEvents, rawEvent)

		val, err = json.Marshal(rawEvents)
		if err != nil {
			return err
		}

		return networkParamsBucket.Put(epochKey, val)
	}

	if dbTx == nil {
		return g.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getNetworkParamsEvents returns a list of NetworkParams contract events that happened in given epoch
// it is valid that epoch has no events if there was no governance proposal executed in it
func (g *GovernanceStore) getNetworkParamsEvents(epoch uint64, dbTx *bolt.Tx) (eventsRaw, error) {
	var (
		rawEvents eventsRaw
		err       error
	)

	getFn := func(tx *bolt.Tx) error {
		val := tx.Bucket(networkParamsEventsBucket).Get(common.EncodeUint64ToBytes(epoch))
		if val != nil {
			return json.Unmarshal(val, &rawEvents)
		}

		return nil // this is valid, since we might not have any proposals executed in given epoch
	}

	if dbTx == nil {
		err = g.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

	return rawEvents, err
}

// getAllForkEvents returns a list of all forks and their activation block
func (g *GovernanceStore) getAllForkEvents(dbTx *bolt.Tx) (map[types.Hash]*big.Int, error) {
	var (
		allForks = map[types.Hash]*big.Int{}
		err      error
	)

	getFn := func(tx *bolt.Tx) error {
		return tx.Bucket(forkParamsEventsBucket).ForEach(func(k, v []byte) error {
			allForks[types.BytesToHash(k)] = new(big.Int).SetBytes(v)

			return nil
		})
	}

	if dbTx == nil {
		err = g.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

	return allForks, err
}

// insertClientConfig inserts client (polybft) config to bolt db
func (g *GovernanceStore) insertClientConfig(config *chain.Params, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := json.Marshal(config)
		if err != nil {
			return err
		}

		return tx.Bucket(clientConfigBucket).Put(clientConfigKey, raw)
	}

	if dbTx == nil {
		return g.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getClientConfig returns client (polybft) config from bolt db
func (g *GovernanceStore) getClientConfig(dbTx *bolt.Tx) (*chain.Params, error) {
	var (
		config *chain.Params
		err    error
	)

	getFn := func(tx *bolt.Tx) error {
		val := tx.Bucket(clientConfigBucket).Get(clientConfigKey)
		if val == nil {
			return errClientConfigNotFound
		}

		return json.Unmarshal(val, &config)
	}

	if dbTx == nil {
		err = g.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

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
