package polybft

import (
	"errors"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

type validatorSnapshot struct {
	Epoch            uint64     `json:"epoch"`
	EpochEndingBlock uint64     `json:"epochEndingBlock"`
	Snapshot         AccountSet `json:"snapshot"`
}

func (vs *validatorSnapshot) copy() *validatorSnapshot {
	copiedAccountSet := vs.Snapshot.Copy()

	return &validatorSnapshot{
		Epoch:            vs.Epoch,
		EpochEndingBlock: vs.EpochEndingBlock,
		Snapshot:         copiedAccountSet,
	}
}

// TODO: Should we switch validators snapshot to Edge implementation? (validators/store/snapshot/snapshot.go)
type validatorsSnapshotCache struct {
	snapshots  map[uint64]*validatorSnapshot
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
		snapshots:  map[uint64]*validatorSnapshot{},
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

	_, extra, err := getBlockData(blockNumber, v.blockchain)
	if err != nil {
		return nil, err
	}

	isEpochEndingBlock, err := v.isEpochEndingBlock(blockNumber, extra)
	if err != nil {
		return nil, err
	}

	epochToGetSnapshot := extra.Checkpoint.EpochNumber
	if !isEpochEndingBlock {
		epochToGetSnapshot--
	}

	v.logger.Trace("Retrieving snapshot started...", "Block", blockNumber, "Epoch", epochToGetSnapshot)

	latestValidatorSnapshot, err := v.getLastCachedSnapshot(epochToGetSnapshot)
	if err != nil {
		return nil, err
	}

	if latestValidatorSnapshot != nil && latestValidatorSnapshot.Epoch == epochToGetSnapshot {
		// we have snapshot for required block (epoch) in cache
		return latestValidatorSnapshot.Snapshot, nil
	}

	if latestValidatorSnapshot == nil {
		// Haven't managed to retrieve snapshot for any epoch from the cache.
		// Build snapshot from the scratch, by applying delta from the genesis block.
		// log.Trace("Building validators snapshot from scratch...")
		genesisBlockSnapshot, err := v.computeSnapshot(nil, 0, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch 0: %w", err)
		}

		err = v.storeSnapshot(genesisBlockSnapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch 0: %w", err)
		}

		latestValidatorSnapshot = genesisBlockSnapshot

		v.logger.Trace("Built validators snapshot for genesis block")
	}

	deltasCount := 0

	v.logger.Trace("Applying deltas started...", "LatestSnapshotEpoch", latestValidatorSnapshot.Epoch)

	// Create the snapshot for the desired block (epoch) by incrementally applying deltas to the latest stored snapshot
	for latestValidatorSnapshot.Epoch < epochToGetSnapshot {
		nextEpochEndBlockNumber, err := v.getNextEpochEndingBlock(latestValidatorSnapshot.EpochEndingBlock)
		if err != nil {
			return nil, fmt.Errorf("failed to get the epoch ending block for epoch: %d. Error: %w",
				latestValidatorSnapshot.Epoch+1, err)
		}

		v.logger.Trace("Applying delta", "epochEndBlock", nextEpochEndBlockNumber)

		intermediateSnapshot, err := v.computeSnapshot(latestValidatorSnapshot, nextEpochEndBlockNumber, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch %d: %w", latestValidatorSnapshot.Epoch+1, err)
		}

		latestValidatorSnapshot = intermediateSnapshot
		if err = v.storeSnapshot(latestValidatorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch %d: %w", latestValidatorSnapshot.Epoch, err)
		}

		deltasCount++
	}

	v.logger.Trace(
		fmt.Sprintf("Applied %d delta(s) to the validators snapshot", deltasCount),
		"Epoch", latestValidatorSnapshot.Epoch,
	)

	if err := v.cleanup(); err != nil {
		// error on cleanup should not block or fail any action
		v.logger.Error("could not clean validator snapshots from cache and db", "err", err)
	}

	return latestValidatorSnapshot.Snapshot, nil
}

// computeSnapshot gets desired block header by block number, extracts its extra and applies given delta to the snapshot
func (v *validatorsSnapshotCache) computeSnapshot(
	existingSnapshot *validatorSnapshot,
	nextEpochEndBlockNumber uint64,
	parents []*types.Header,
) (*validatorSnapshot, error) {
	var header *types.Header

	v.logger.Trace("Compute snapshot started...", "BlockNumber", nextEpochEndBlockNumber)

	if len(parents) > 0 {
		for i := len(parents) - 1; i >= 0; i-- {
			parentHeader := parents[i]
			if parentHeader.Number == nextEpochEndBlockNumber {
				v.logger.Trace("Compute snapshot. Found header in parents", "Header", parentHeader.Number)
				header = parentHeader

				break
			}
		}
	}

	if header == nil {
		var ok bool

		header, ok = v.blockchain.GetHeaderByNumber(nextEpochEndBlockNumber)
		if !ok {
			return nil, fmt.Errorf("unknown block. Block number=%v", nextEpochEndBlockNumber)
		}
	}

	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode extra from the block#%d: %w", header.Number, err)
	}

	var snapshot AccountSet

	var snapshotEpoch uint64

	if existingSnapshot == nil {
		snapshot = AccountSet{}
	} else {
		snapshot = existingSnapshot.Snapshot
		snapshotEpoch = existingSnapshot.Epoch + 1
	}

	snapshot, err = snapshot.ApplyDelta(extra.Validators)
	if err != nil {
		return nil, fmt.Errorf("failed to apply delta to the validators snapshot, block#%d: %w", header.Number, err)
	}

	v.logger.Trace("Computed snapshot",
		"blockNumber", nextEpochEndBlockNumber,
		"snapshot", snapshot,
		"delta", extra.Validators)

	return &validatorSnapshot{
		Epoch:            snapshotEpoch,
		EpochEndingBlock: nextEpochEndBlockNumber,
		Snapshot:         snapshot,
	}, nil
}

// storeSnapshot stores given snapshot to the in-memory cache and database
func (v *validatorsSnapshotCache) storeSnapshot(snapshot *validatorSnapshot) error {
	copySnap := snapshot.copy()
	v.snapshots[copySnap.Epoch] = copySnap

	if err := v.state.insertValidatorSnapshot(copySnap); err != nil {
		return fmt.Errorf("failed to insert validator snapshot for epoch %d to the database: %w", copySnap.Epoch, err)
	}

	v.logger.Trace("Store snapshot", "Snapshots", v.snapshots)

	return nil
}

// Cleanup cleans the validators cache in memory and db
func (v *validatorsSnapshotCache) cleanup() error {
	if len(v.snapshots) >= validatorSnapshotLimit {
		latestEpoch := uint64(0)

		for e := range v.snapshots {
			if e > latestEpoch {
				latestEpoch = e
			}
		}

		startEpoch := latestEpoch
		cache := make(map[uint64]*validatorSnapshot, numberOfSnapshotsToLeaveInMemory)

		for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
			if snapshot, exists := v.snapshots[startEpoch]; exists {
				cache[startEpoch] = snapshot
			}

			startEpoch--
		}

		v.snapshots = cache

		return v.state.cleanValidatorSnapshotsFromDB(latestEpoch)
	}

	return nil
}

// getLastCachedSnapshot gets the latest snapshot cached
// If it doesn't have snapshot cached for desired epoch, it will return the latest one it has
func (v *validatorsSnapshotCache) getLastCachedSnapshot(currentEpoch uint64) (*validatorSnapshot, error) {
	snapshot := v.snapshots[currentEpoch]
	if snapshot != nil {
		v.logger.Trace("Found snapshot in memory cache", "Epoch", currentEpoch)

		return snapshot, nil
	}

	snapshot, err := v.state.getLastSnapshot()
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// getNextEpochEndingBlock gets the epoch ending block of a newer epoch
// It start checking the blocks from the provided epoch ending block of the previous epoch
func (v *validatorsSnapshotCache) getNextEpochEndingBlock(latestEpochEndingBlock uint64) (uint64, error) {
	blockNumber := latestEpochEndingBlock + 1 // get next block

	_, extra, err := getBlockData(blockNumber, v.blockchain)
	if err != nil {
		return 0, err
	}

	startEpoch := extra.Checkpoint.EpochNumber
	epoch := startEpoch

	for startEpoch == epoch {
		blockNumber++

		_, extra, err = getBlockData(blockNumber, v.blockchain)
		if err != nil {
			if errors.Is(err, blockchain.ErrNoBlock) {
				return blockNumber - 1, nil
			}

			return 0, err
		}

		epoch = extra.Checkpoint.EpochNumber
	}

	return blockNumber - 1, nil
}

// isEpochEndingBlock checks if given block is an epoch ending block
func (v *validatorsSnapshotCache) isEpochEndingBlock(blockNumber uint64, extra *Extra) (bool, error) {
	if !extra.Validators.IsEmpty() {
		return true, nil
	}

	_, nextBlockExtra, err := getBlockData(blockNumber+1, v.blockchain)
	if err != nil {
		if errors.Is(err, blockchain.ErrNoBlock) {
			return true, nil
		}

		return false, nil
	}

	return extra.Checkpoint.EpochNumber != nextBlockExtra.Checkpoint.EpochNumber, nil
}
