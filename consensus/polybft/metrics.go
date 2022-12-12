package polybft

import (
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/armon/go-metrics"
)

// updateBlockMetrics updates various metrics based on the given block
// (such as block interval, number of transactions and block rounds metrics)
func updateBlockMetrics(block *types.Block, blockchain blockchainBackend) error {
	if block.Number() > 1 {
		// retrieve parent header
		parentHeader, parentExists := blockchain.GetHeaderByNumber(block.Number() - 1)
		if !parentExists {
			return fmt.Errorf("failed to retrieve block %d", block.Number()-1)
		}

		parentTime := time.Unix(int64(parentHeader.Timestamp), 0)
		headerTime := time.Unix(int64(block.Header.Timestamp), 0)
		// update the block interval metric
		metrics.SetGauge([]string{"block_interval"}, float32(headerTime.Sub(parentTime).Seconds()))
	}

	// update the Number of transactions in the block metric
	metrics.SetGauge([]string{"num_txs"}, float32(len(block.Body().Transactions)))

	extra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		return err
	}

	// number of rounds needed to seal a block
	metrics.SetGauge([]string{"rounds"}, float32(extra.Checkpoint.BlockRound))

	return nil
}

// updateEpochMetrics updates epoch-related metrics
// (e.g. epoch number, validator set length)
func updateEpochMetrics(epoch epochMetadata) {
	// update epoch number metrics
	metrics.SetGauge([]string{"epoch_number"}, float32(epoch.Number))
	// update number of validators metrics
	metrics.SetGauge([]string{"validators"}, float32(epoch.Validators.Len()))
}
