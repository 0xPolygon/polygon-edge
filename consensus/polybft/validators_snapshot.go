package polybft

import (
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

// TODO: Should we switch validators snapshot to Edge implementation? (validators/store/snapshot/snapshot.go)
type validatorsSnapshotCache struct {
	snapshots  map[uint64]AccountSet
	state      *State
	blockchain blockchainBackend
	lock       sync.Mutex
	logger     hclog.Logger
}

// newValidatorsSnapshotCache initializes a new instance of validatorsSnapshotCache
func newValidatorsSnapshotCache(
	logger hclog.Logger, state *State, blockchain blockchainBackend,
) *validatorsSnapshotCache {
	return &validatorsSnapshotCache{
		snapshots:  map[uint64]AccountSet{},
		state:      state,
		blockchain: blockchain,
		logger:     logger.Named("validators_snapshot"),
	}
}

// GetSnapshot tries to retrieve the most recent cached snapshot (if any) and
// applies pending validator set deltas to it.
// Otherwise, it builds a snapshot from scratch and applies pending validator set deltas.
func (v *validatorsSnapshotCache) GetSnapshot(blockNumber uint64, parents []*types.Header) (AccountSet, error) {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.logger.Trace("Retrieving snapshot started...", "Block", blockNumber)

	var snapshot AccountSet

	lastBlockWithSnapshot := blockNumber

	for ; ; lastBlockWithSnapshot-- {
		snapshot = v.snapshots[lastBlockWithSnapshot]
		if snapshot != nil {
			v.logger.Trace("Found snapshot in memory cache", "Block", lastBlockWithSnapshot)

			break
		}

		dbSnapshot, err := v.state.getValidatorSnapshot(lastBlockWithSnapshot)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve validators snapshot from the database for block=%d: %w",
				lastBlockWithSnapshot,
				err,
			)
		}

		if dbSnapshot != nil {
			snapshot = dbSnapshot
			v.snapshots[lastBlockWithSnapshot] = dbSnapshot.Copy()
			v.logger.Trace("Found snapshot in database", "Block", lastBlockWithSnapshot)

			break
		}

		if lastBlockWithSnapshot == 0 {
			// prevent underflow of uint64
			break
		}
	}

	if snapshot != nil && lastBlockWithSnapshot == blockNumber {
		if err := v.cleanup(); err != nil {
			// error on cleanup should not block or fail any action
			v.logger.Error("could not clean validator snapshots from cache and db", "err", err)
		}

		return snapshot, nil
	}

	if snapshot == nil {
		// Haven't managed to retrieve snapshot for any block from the cache.
		// Build snapshot from the scratch, by applying delta from the genesis block.
		initialSnapshot, err := v.computeSnapshot(nil, 0, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch 0: %w", err)
		}

		snapshot = initialSnapshot
		// store same snapshot for epochs 0 and 1, since it is shared between those two epochs
		// (and there might be use case to require snapshot for the genesis block, i.e. epoch 0)
		err = v.storeSnapshot(0, initialSnapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch 0: %w", err)
		}

		v.logger.Trace("Built validators snapshot")
	}

	// Create most recent snapshot by incrementally applying validator set deltas from the resolved block
	// which have snapshot associated to the last block which is less than the one snapshot is requested for.
	deltasCount := 0

	v.logger.Trace("Applying deltas started...",
		"lastBlockWithSnapshot", lastBlockWithSnapshot,
		"blockNumber", blockNumber)

	for lastBlockWithSnapshot < blockNumber {
		lastBlockWithSnapshot++

		v.logger.Trace("Applying delta", "blockWithSnapshot", lastBlockWithSnapshot)

		intermediateSnapshot, err := v.computeSnapshot(snapshot, lastBlockWithSnapshot, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for block %d: %w", lastBlockWithSnapshot, err)
		}

		snapshot = intermediateSnapshot
		deltasCount++
	}

	if deltasCount > 0 {
		err := v.storeSnapshot(lastBlockWithSnapshot, snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for block %d: %w", lastBlockWithSnapshot, err)
		}
	}

	v.logger.Trace(
		fmt.Sprintf("Applied %d delta(s) to the validators snapshot", deltasCount),
		"BlockNumber", lastBlockWithSnapshot,
	)

	if err := v.cleanup(); err != nil {
		// error on cleanup should not block or fail any action
		v.logger.Error("could not clean validator snapshots from cache and db", "err", err)
	}

	return snapshot, nil
}

// computeSnapshot gets desired block header by block number, extracts its extra and applies given delta to the snapshot
func (v *validatorsSnapshotCache) computeSnapshot(
	existingSnapshot AccountSet,
	blockNumber uint64,
	parents []*types.Header,
) (AccountSet, error) {
	var header *types.Header

	v.logger.Trace("Compute snapshot started...", "BlockNumber", blockNumber)

	if len(parents) > 0 {
		for i := len(parents) - 1; i >= 0; i-- {
			parentHeader := parents[i]
			if parentHeader.Number == blockNumber {
				v.logger.Trace("Compute snapshot. Found header in parents", "Header", parentHeader.Number)
				header = parentHeader

				break
			}
		}
	}

	if header == nil {
		var ok bool

		header, ok = v.blockchain.GetHeaderByNumber(blockNumber)
		if !ok {
			return nil, fmt.Errorf("unknown block. Block number=%v", blockNumber)
		}
	}

	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode extra from the block#%d: %w", header.Number, err)
	}

	snapshot := existingSnapshot
	if existingSnapshot == nil {
		snapshot = AccountSet{}
	}

	snapshot, err = snapshot.ApplyDelta(extra.Validators)
	if err != nil {
		return nil, fmt.Errorf("failed to apply delta to the validators snapshot, block#%d: %w", header.Number, err)
	}

	v.logger.Trace("Computed snapshot",
		"blockNumber", blockNumber,
		"snapshot", snapshot,
		"delta", extra.Validators)

	return snapshot, nil
}

// storeSnapshot stores given snapshot to the in-memory cache and database
func (v *validatorsSnapshotCache) storeSnapshot(block uint64, snapshot AccountSet) error {
	copySnap := snapshot.Copy()
	v.snapshots[block] = copySnap

	if err := v.state.insertValidatorSnapshot(block, copySnap); err != nil {
		return fmt.Errorf("failed to insert validator snapshot for block %d to the database: %w", block, err)
	}

	v.logger.Trace("Store snapshot", "Snapshots", v.snapshots)

	return nil
}

// Cleanup cleans the validators cache in memory and db
func (v *validatorsSnapshotCache) cleanup() error {
	if len(v.snapshots) >= validatorSnapshotLimit {
		latestBlock := uint64(0)

		for e := range v.snapshots {
			if e > latestBlock {
				latestBlock = e
			}
		}

		block := latestBlock
		cache := make(map[uint64]AccountSet, numberOfSnapshotsToLeaveInMemory)

		for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
			if snapshot, exists := v.snapshots[block]; exists {
				cache[block] = snapshot
			}

			block--
		}

		v.snapshots = cache

		return v.state.cleanValidatorSnapshotsFromDB(latestBlock)
	}

	return nil
}
