package loadbot

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
	"time"
)

func getInitialSenderNonce(client *jsonrpc.Client, address types.Address) (uint64, error) {
	nonce, err := client.Eth().GetNonce(web3.Address(address), web3.Latest)
	if err != nil {
		return 0, fmt.Errorf("failed to query initial sender nonce: %w", err)
	}

	return nonce, nil
}

func getAverageGasPrice(client *jsonrpc.Client) (uint64, error) {
	gasPrice, err := client.Eth().GasPrice()
	if err != nil {
		return 0, fmt.Errorf("failed to query initial gas price: %w", err)
	}

	return gasPrice, nil
}

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
