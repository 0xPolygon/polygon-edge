package polybft

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

// calculateFirstBlockOfPeriod calculates the first block of a period
func calculateFirstBlockOfPeriod(currentBlockNumber, periodSize uint64) uint64 {
	if currentBlockNumber <= periodSize {
		return 1 // it's the first epoch
	}

	switch currentBlockNumber % periodSize {
	case 1:
		return currentBlockNumber
	case 0:
		return currentBlockNumber - periodSize + 1
	default:
		return currentBlockNumber - (currentBlockNumber % periodSize) + 1
	}
}

// getEpochNumber returns epoch number for given blockNumber and epochSize.
// Epoch number is derived as a result of division of block number and epoch size.
// Since epoch number is 1-based (0 block represents special case zero epoch),
// we are incrementing result by one for non epoch-ending blocks.
func getEpochNumber(blockNumber, epochSize uint64) uint64 {
	if isEndOfPeriod(blockNumber, epochSize) {
		return blockNumber / epochSize
	}

	return blockNumber/epochSize + 1
}

// getEndEpochBlockNumber returns block number which corresponds
// to the one at the beginning of the given epoch with regards to epochSize
func getEndEpochBlockNumber(epoch, epochSize uint64) uint64 {
	return epoch * epochSize
}
