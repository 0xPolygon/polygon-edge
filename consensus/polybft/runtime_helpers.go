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

// getQuorumSize returns result of division of given number by two,
// but rounded to next integer value (similar to math.Ceil function).
func getQuorumSize(validatorsCount int) int {
	return (validatorsCount + 1) / 2
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
