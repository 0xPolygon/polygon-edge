package polybft

import (
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/armon/go-metrics"
)

const (
	// consensusMetricsPrefix is a consensus-related metrics prefix
	consensusMetricsPrefix = "consensus"
)

// updateBlockMetrics updates various metrics based on the given block
// (such as block interval, number of transactions and block rounds metrics)
func updateBlockMetrics(currentBlock *types.Block, parentHeader *types.Header) error {
	if currentBlock.Number() > 1 {
		parentTime := time.Unix(int64(parentHeader.Timestamp), 0)
		headerTime := time.Unix(int64(currentBlock.Header.Timestamp), 0)
		// update the block interval metric
		metrics.SetGauge([]string{consensusMetricsPrefix, "block_interval"}, float32(headerTime.Sub(parentTime).Seconds()))
	}

	// update the number of transactions in the block metric
	metrics.SetGauge([]string{consensusMetricsPrefix, "num_txs"}, float32(len(currentBlock.Transactions)))

	extra, err := GetIbftExtra(currentBlock.Header.ExtraData)
	if err != nil {
		return err
	}

	// number of rounds needed to seal a block
	metrics.SetGauge([]string{consensusMetricsPrefix, "rounds"}, float32(extra.Checkpoint.BlockRound))
	metrics.SetGauge([]string{consensusMetricsPrefix, "chain_head"}, float32(currentBlock.Number()))
	metrics.IncrCounter([]string{consensusMetricsPrefix, "block_counter"}, float32(1))
	metrics.SetGauge([]string{consensusMetricsPrefix, "block_space_used"}, float32(currentBlock.Header.GasUsed))

	// Update the base fee metric
	metrics.SetGauge([]string{consensusMetricsPrefix, "base_fee"}, float32(currentBlock.Header.BaseFee))

	return nil
}

// updateEpochMetrics updates epoch-related metrics
// (e.g. epoch number, validator set length)
func updateEpochMetrics(epoch epochMetadata) {
	// update epoch number metrics
	metrics.SetGauge([]string{consensusMetricsPrefix, "epoch_number"}, float32(epoch.Number))
	// update number of validators metrics
	metrics.SetGauge([]string{consensusMetricsPrefix, "validators"}, float32(epoch.Validators.Len()))
}

// updateBlockExecutionMetric updates the block execution metric
func updateBlockExecutionMetric(start time.Time) {
	metrics.SetGauge([]string{consensusMetricsPrefix, "block_execution_time"},
		float32(time.Now().UTC().Sub(start).Seconds()))
}
