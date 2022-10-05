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
	epochSize  uint64
	blockchain blockchainBackend
	lock       sync.Mutex
	logger     hclog.Logger
}

// newValidatorsSnapshotCache initializes a new instance of validatorsSnapshotCache
func newValidatorsSnapshotCache(
	logger hclog.Logger, state *State, epochSize uint64, blockchain blockchainBackend,
) *validatorsSnapshotCache {
	return &validatorsSnapshotCache{
		snapshots:  map[uint64]AccountSet{},
		state:      state,
		epochSize:  epochSize,
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

	epochNumber := getEpochNumber(blockNumber, v.epochSize)
	if !isEndOfPeriod(blockNumber, v.epochSize) && epochNumber > 0 {
		// snapshot is retrieved for the previous epoch
		// since the last block of the previous epoch contains validator set for the current epoch
		epochNumber--
	}

	v.logger.Trace("Retrieving snapshot started...", "Block", blockNumber, "Epoch", epochNumber)

	var snapshot AccountSet
	currentEpochNumber := epochNumber

	for ; ; currentEpochNumber-- {
		snapshot = v.snapshots[currentEpochNumber]
		if snapshot != nil {
			v.logger.Trace("Found snapshot in memory cache", "Epoch", currentEpochNumber)

			break
		}

		dbSnapshot, err := v.state.getValidatorSnapshot(currentEpochNumber)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to retrieve validators snapshot from the database for epoch=%d: %w",
				currentEpochNumber,
				err,
			)
		}

		if dbSnapshot != nil {
			snapshot = dbSnapshot
			v.snapshots[currentEpochNumber] = dbSnapshot.Copy()
			v.logger.Trace("Found snapshot in database", "Epoch", currentEpochNumber)

			break
		}

		if currentEpochNumber == 0 {
			// prevent underflow of uint64
			break
		}
	}

	if snapshot != nil && currentEpochNumber == epochNumber {
		return snapshot, nil
	}

	if snapshot == nil {
		// Haven't managed to retrieve snapshot for any epoch from the cache.
		// Build snapshot from the scratch, by applying delta from the genesis block.
		// log.Trace("Building validators snapshot from scratch...")
		initialSnapshot, err := v.computeSnapshot(nil, 0, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch 0: %v", err)
		}

		snapshot = initialSnapshot
		// store same snapshot for epochs 0 and 1, since it is shared between those two epochs
		// (and there might be use case to require snapshot for the genesis block, i.e. epoch 0)
		err = v.storeSnapshot(0, initialSnapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch 0: %v", err)
		}

		v.logger.Trace("Built validators snapshot")
	}

	// Create most recent snapshot by incrementally applying validator set deltas from the resolved epoch ending block
	// which have snapshot associated to the last epoch ending block which is less than the one snapshot is requested for.
	endEpochBlockNumber := getEndEpochBlockNumber(currentEpochNumber, v.epochSize)
	deltasCount := 0

	v.logger.Trace("Applying deltas started...",
		"endEpochBlockNumber", endEpochBlockNumber,
		"blockNumber", blockNumber)

	for endEpochBlockNumber+v.epochSize <= blockNumber {
		endEpochBlockNumber += v.epochSize
		currentEpochNumber++

		v.logger.Trace("Applying delta", "epochEndBlock", endEpochBlockNumber)

		intermediateSnapshot, err := v.computeSnapshot(snapshot, endEpochBlockNumber, parents)
		if err != nil {
			return nil, fmt.Errorf("failed to compute snapshot for epoch %d: %v", currentEpochNumber, err)
		}

		snapshot = intermediateSnapshot
		deltasCount++
	}

	if deltasCount > 0 {
		err := v.storeSnapshot(currentEpochNumber, snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to store validators snapshot for epoch %d: %v", currentEpochNumber, err)
		}
	}

	v.logger.Trace(
		fmt.Sprintf("Applied %d delta(s) to the validators snapshot", deltasCount),
		"EndEpochBlock", endEpochBlockNumber,
		"Epoch", currentEpochNumber,
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
	epochEndBlockNumber uint64,
	parents []*types.Header,
) (AccountSet, error) {
	var header *types.Header

	v.logger.Trace("Compute snapshot started...", "BlockNumber", epochEndBlockNumber)

	if len(parents) > 0 {
		for i := len(parents) - 1; i >= 0; i-- {
			parentHeader := parents[i]
			if parentHeader.Number == epochEndBlockNumber {
				v.logger.Trace("Compute snapshot. Found header in parents", "Header", parentHeader.Number)
				header = parentHeader

				break
			}
		}
	}

	if header == nil {
		var ok bool

		header, ok = v.blockchain.GetHeaderByNumber(epochEndBlockNumber)
		if !ok {
			return nil, fmt.Errorf("unknown block. Block number=%v", epochEndBlockNumber)
		}
	}

	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode extra from the block#%d: %v", header.Number, err)
	}

	snapshot := existingSnapshot
	if existingSnapshot == nil {
		snapshot = AccountSet{}
	}

	snapshot, err = snapshot.ApplyDelta(extra.Validators)
	if err != nil {
		return nil, fmt.Errorf("failed to apply delta to the validators snapshot, block#%d: %v", header.Number, err)
	}

	v.logger.Trace("Computed snapshot",
		"blockNumber", epochEndBlockNumber,
		"snapshot", snapshot,
		"delta", extra.Validators)

	return snapshot, nil
}

// storeSnapshot stores given snapshot to the in-memory cache and database
func (v *validatorsSnapshotCache) storeSnapshot(epoch uint64, snapshot AccountSet) error {
	copySnap := snapshot.Copy()
	v.snapshots[epoch] = copySnap
	if err := v.state.insertValidatorSnapshot(epoch, copySnap); err != nil {
		return fmt.Errorf("failed to insert validator snapshot for epoch %d to the database: %v", epoch, err)
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

		epoch := latestEpoch
		cache := make(map[uint64]AccountSet, numberOfSnapshotsToLeaveInMemory)

		for i := 0; i < numberOfSnapshotsToLeaveInMemory; i++ {
			if snapshot, exists := v.snapshots[epoch]; exists {
				cache[epoch] = snapshot
			}

			epoch--
		}

		v.snapshots = cache

		return v.state.cleanValidatorSnapshotsFromDb(latestEpoch)
	}

	return nil
}
