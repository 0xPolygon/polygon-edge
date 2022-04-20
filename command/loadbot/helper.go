package loadbot

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dogechain-lab/jury/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
)

// getInitialSenderNonce queries the sender account nonce before starting the loadbot run.
// The nonce used for transactions is incremented by the loadbot during runtime
func getInitialSenderNonce(client *jsonrpc.Client, address types.Address) (uint64, error) {
	nonce, err := client.Eth().GetNonce(web3.Address(address), web3.Latest)
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
	gasEstimate, err := client.Eth().EstimateGas(&web3.CallMsg{
		From:     web3.Address(txn.From),
		To:       (*web3.Address)(txn.To),
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

// calculate block utilization in percents
func calculateBlockUtilization(blockInfo GasMetrics) float64 {
	return float64(blockInfo.GasUsed) / float64(blockInfo.GasLimit) * 100
}

// calculate average block utilization across all blocks
func calculateAvgBlockUtil(gasData map[uint64]GasMetrics) float64 {
	sum := float64(0)
	for _, i := range gasData {
		sum += i.Utilization
	}

	return sum / float64(len(gasData))
}

// fetch block gas usage and gas limit and calculate block utilization
func (l *Loadbot) calculateGasMetrics(jsonClient *jsonrpc.Client, gasMetrics *BlockGasMetrics) error {
	errGr, _ := errgroup.WithContext(context.Background())

	for num, data := range gasMetrics.Blocks {
		blockNum := num
		blockData := data

		errGr.Go(func() error {
			blockInfom, err := jsonClient.Eth().GetBlockByNumber(web3.BlockNumber(blockNum), false)
			if err != nil {
				return fmt.Errorf("could not fetch block %d by number, %w", blockNum, err)
			}

			blockData.GasLimit = blockInfom.GasLimit
			blockData.GasUsed = blockInfom.GasUsed
			blockData.Utilization = calculateBlockUtilization(blockData)
			gasMetrics.Blocks[blockNum] = blockData

			return nil
		})
	}

	if err := errGr.Wait(); err != nil {
		return err
	}

	return nil
}

func (l *Loadbot) updateGasEstimate(jsonClient *jsonrpc.Client) error {
	//nolint: ifshort
	gasLimit := l.cfg.GasLimit

	if gasLimit == nil {
		// Get the gas estimate
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
	}

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

// returns true if this is erc20 or erc721 mode
func (l *Loadbot) isTokenTransferMode() bool {
	switch l.cfg.GeneratorMode {
	case erc20, erc721:
		return true
	default:
		return false
	}
}

//initialze gas metrics blocks map with block number as key
func (l *Loadbot) initGasMetricsBlocksMap(blockNum uint64) {
	l.metrics.GasMetrics.BlockGasMutex.Lock()
	l.metrics.GasMetrics.Blocks[blockNum] = GasMetrics{}
	l.metrics.GasMetrics.BlockGasMutex.Unlock()
}
