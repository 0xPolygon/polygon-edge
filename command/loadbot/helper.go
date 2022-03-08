package loadbot

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
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
	blockNumErr := make(chan error)
	defer close(blockNumErr)

	for blockNum, blockData := range gasMetrics.Blocks {
		go func(
			jsonClient *jsonrpc.Client,
			gasMetrics *BlockGasMetrics,
			blockNum uint64,
			blockData GasMetrics,
			blockNumErr chan error,
		) {
			blockInfom, err := jsonClient.Eth().GetBlockByNumber(web3.BlockNumber(blockNum), false)
			if err != nil {
				blockNumErr <- fmt.Errorf("could not fetch block %d by number, %w", blockNum, err)
			}

			blockData.GasLimit = blockInfom.GasLimit
			blockData.GasUsed = blockInfom.GasUsed
			blockData.Utilization = calculateBlockUtilization(blockData)
			gasMetrics.Blocks[blockNum] = blockData

			blockNumErr <- nil
		}(jsonClient, gasMetrics, blockNum, blockData, blockNumErr)

		if err := <-blockNumErr; err != nil {
			return err
		}
	}

	return nil
}
