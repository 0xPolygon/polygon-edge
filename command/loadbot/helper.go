package loadbot

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/jsonrpc"
)

// getInitialSenderNonce queries the sender account nonce before starting the loadbot run.
// The nonce used for transactions is incremented by the loadbot during runtime
func getInitialSenderNonce(client *jsonrpc.Client, address types.Address) (uint64, error) {
	nonce, err := client.Eth().GetNonce(ethgo.Address(address), ethgo.Latest)
	if err != nil {
		return 0, fmt.Errorf("failed to query initial sender nonce: %w", err)
	}

	return nonce, nil
}

// getAverageGasPrice queries the network node for the average gas price
// before starting the loadbot run
func getAverageGasPrice(client *jsonrpc.Client) (uint64, error) {
	gasPrice, err := client.Eth().GasPrice()
	if err != nil {
		return 0, fmt.Errorf("failed to query initial gas price: %w", err)
	}

	return gasPrice, nil
}

// estimateGas queries the network node for a gas estimation before starting
// the loadbot run
func estimateGas(client *jsonrpc.Client, txn *types.Transaction) (uint64, error) {
	gasEstimate, err := client.Eth().EstimateGas(&ethgo.CallMsg{
		From:     ethgo.Address(txn.From),
		To:       (*ethgo.Address)(txn.To),
		Data:     txn.Input,
		GasPrice: txn.GasPrice.Uint64(),
		Value:    txn.Value,
	})

	if err != nil {
		return 0, fmt.Errorf("failed to query gas estimate: %w", err)
	}

	if gasEstimate == 0 {
		gasEstimate = defaultGasLimit
	}

	return gasEstimate, nil
}

// calculateBlockUtilization calculates block utilization in percents
func calculateBlockUtilization(gasUsed, gasLimit uint64) float64 {
	return float64(gasUsed) / float64(gasLimit) * 100
}

// calculateAvgBlockUtil calculates average block utilization across all blocks
func calculateAvgBlockUtil(gasData map[uint64]GasMetrics) float64 {
	sum := float64(0)
	for _, i := range gasData {
		sum += i.Utilization
	}

	return sum / float64(len(gasData))
}

// getBlockGasMetrics fetches block gas metrics from the JSON-RPC client
// for the specified blocks
func getBlockGasMetrics(
	jsonClient *jsonrpc.Client,
	blockNums map[uint64]struct{},
) (*BlockGasMetrics, error) {
	var (
		errors          = make([]error, 0)
		errorsLock      sync.Mutex
		blockGasMetrics = &BlockGasMetrics{
			Blocks: make(map[uint64]GasMetrics),
		}
		wg sync.WaitGroup
	)

	// addError is a helper for accumulating errors
	// in go routines
	addError := func(fetchErr error) {
		errorsLock.Lock()
		defer errorsLock.Unlock()

		errors = append(errors, fetchErr)
	}

	queryBlockInfo := func(
		blockNum uint64,
	) {
		// Query node for block
		blockInfo, err := jsonClient.Eth().GetBlockByNumber(
			ethgo.BlockNumber(blockNum),
			false,
		)
		if err != nil {
			addError(
				fmt.Errorf("could not fetch block %d by number, %w", blockNum, err),
			)

			return
		}

		// Update the block gas metrics
		blockGasMetrics.AddBlockMetric(
			blockNum,
			GasMetrics{
				GasUsed:     blockInfo.GasUsed,
				GasLimit:    blockInfo.GasLimit,
				Utilization: calculateBlockUtilization(blockInfo.GasUsed, blockInfo.GasLimit),
			},
		)
	}

	// For each block number, fetch the corresponding
	// block info data
	for blockNum := range blockNums {
		wg.Add(1)

		go func(blockNum uint64) {
			defer wg.Done()

			queryBlockInfo(blockNum)
		}(blockNum)
	}

	wg.Wait()

	if len(errors) > 1 {
		return nil, fmt.Errorf(
			"unable to successfully fetch gas metrics, %v",
			errors,
		)
	}

	return blockGasMetrics, nil
}

// updateGasEstimate updates the loadbot generator gas estimate
func (l *Loadbot) updateGasEstimate(jsonClient *jsonrpc.Client) error {
	gasLimit := l.cfg.GasLimit

	if gasLimit != nil {
		// User specified a gas limit to use,
		// no need to calculate one
		return nil
	}

	// User didn't specify a gas limit to use, calculate it
	exampleTxn, err := l.generator.GetExampleTransaction()
	if err != nil {
		return fmt.Errorf("unable to get example transaction, %w", err)
	}

	// No gas limit specified, query the network for an estimation
	gasEstimate, estimateErr := estimateGas(jsonClient, exampleTxn)
	if estimateErr != nil {
		return fmt.Errorf("unable to get gas estimate, %w", err)
	}

	gasLimit = new(big.Int).SetUint64(gasEstimate)

	l.generator.SetGasEstimate(gasLimit.Uint64())

	return nil
}

// calcMaxTimeout calculates the max timeout for transactions receipts
// based on the transaction count and tps params
func calcMaxTimeout(count, tps uint64) time.Duration {
	waitTime := minReceiptWait
	// The receipt timeout should be at max maxReceiptWait
	// or minReceiptWait + tps / count * 100
	// This way the wait time scales linearly for more stressful situations
	waitFactor := time.Duration(float64(tps)/float64(count)*100) * time.Second

	if waitTime+waitFactor > maxReceiptWait {
		return maxReceiptWait
	}

	return waitTime + waitFactor
}

// isTokenTransferMode checks if the mode is erc20 or erc721
func (l *Loadbot) isTokenTransferMode() bool {
	switch l.cfg.GeneratorMode {
	case erc20, erc721:
		return true
	default:
		return false
	}
}
