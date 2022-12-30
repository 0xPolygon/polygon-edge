package polybft

import (
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/types"
)

type blockGetter func(uint64) (*types.Header, bool)

// isEndOfPeriod checks if an end of a period (either it be sprint or epoch)
// is reached with the current block (the parent block of the current fsm iteration)
func isEndOfPeriod(blockNumber, periodSize uint64) bool {
	return blockNumber%periodSize == 0
}

// getQuorumSize returns result of division of given number by two,
// but rounded to next integer value (similar to math.Ceil function).
func getQuorumSize(validatorsCount int) int {
	return (validatorsCount + 1) / 2
}

// getBlockData returns block header and extra
func getBlockData(blockNumber uint64,
	getBlock blockGetter) (*types.Header, *Extra, error) {
	blockHeader, found := getBlock(blockNumber)
	if !found {
		return nil, nil, blockchain.ErrNoBlock
	}

	blockExtra, err := GetIbftExtra(blockHeader.ExtraData)
	if err != nil {
		return nil, nil, err
	}

	return blockHeader, blockExtra, nil
}

// getFirstBlockOfEpoch returns the first block of epoch in which provided header resides
func getFirstBlockOfEpoch(epochID uint64, header *types.Header, getBlock blockGetter) (uint64, error) {
	if header.Number == 0 {
		// if we are starting the chain, we know that the first block is block 1
		return 1, nil
	}

	blockHeader := header
	blockExtra, err := GetIbftExtra(header.ExtraData)

	if err != nil {
		return 0, err
	}

	if epochID != blockExtra.Checkpoint.EpochNumber {
		// its a regular epoch ending. No out of sync happened
		return header.Number + 1, nil
	}

	// node was out of sync, so we need to figure out what was the first block of the given epoch
	epoch := blockExtra.Checkpoint.EpochNumber

	var firstBlockInEpoch uint64

	for blockExtra.Checkpoint.EpochNumber == epoch {
		firstBlockInEpoch = blockHeader.Number
		blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, getBlock)

		if err != nil {
			return 0, err
		}
	}

	return firstBlockInEpoch, nil
}
