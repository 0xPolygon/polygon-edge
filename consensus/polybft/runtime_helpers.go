package polybft

import (
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/types"
)

// isEndOfPeriod checks if an end of a period (either it be sprint or epoch)
// is reached with the current block (the parent block of the current fsm iteration)
func isEndOfPeriod(blockNumber, periodSize uint64) bool {
	return blockNumber%periodSize == 0
}

// getBlockData returns block header and extra
func getBlockData(blockNumber uint64, blockchainBackend blockchainBackend) (*types.Header, *Extra, error) {
	blockHeader, found := blockchainBackend.GetHeaderByNumber(blockNumber)
	if !found {
		return nil, nil, blockchain.ErrNoBlock
	}

	blockExtra, err := GetIbftExtra(blockHeader.ExtraData)
	if err != nil {
		return nil, nil, err
	}

	return blockHeader, blockExtra, nil
}

// isEpochEndingBlock checks if given block is an epoch ending block
func isEpochEndingBlock(blockNumber uint64, extra *Extra, blockchain blockchainBackend) (bool, error) {
	if extra.Validators == nil {
		// non epoch ending blocks have validator set delta as nil
		return false, nil
	}

	if !extra.Validators.IsEmpty() {
		// if validator set delta is not empty, the validator set was changed in this block
		// meaning the epoch changed as well
		return true, nil
	}

	_, nextBlockExtra, err := getBlockData(blockNumber+1, blockchain)
	if err != nil {
		return false, err
	}

	// validator set delta can be empty (no change in validator set happened)
	// so we need to check if their epoch numbers are different
	return extra.Checkpoint.EpochNumber != nextBlockExtra.Checkpoint.EpochNumber, nil
}
