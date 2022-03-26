package loadbot

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/command/loadbot/generator"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/umbracle/go-web3/jsonrpc"
)

func (l *Loadbot) deployContract(
	grpcClient txpoolOp.TxnPoolOperatorClient,
	jsonClient *jsonrpc.Client,
	receiptTimeout time.Duration) error {
	// if this is a regular transfer skip token deployment
	if l.cfg.GeneratorMode == transfer {
		return nil
	}

	start := time.Now()

	// deploy SC
	txHash, err := l.executeTxn(grpcClient)
	if err != nil {
		l.generator.MarkFailedContractTxn(&generator.FailedContractTxnInfo{
			TxHash: txHash.String(),
			Error: &generator.TxnError{
				Error:     err,
				ErrorType: generator.AddErrorType,
			},
		})
		atomic.AddUint64(&l.metrics.FailedContractTransactionsCount, 1)

		return fmt.Errorf("could not execute transaction, %w", err)
	}

	// set timeout
	ctx, cancel := context.WithTimeout(context.Background(), receiptTimeout)
	defer cancel()

	// and wait for receipt
	receipt, err := tests.WaitForReceipt(ctx, jsonClient.Eth(), txHash)

	if err != nil {
		l.generator.MarkFailedContractTxn(&generator.FailedContractTxnInfo{
			TxHash: txHash.String(),
			Error: &generator.TxnError{
				Error:     err,
				ErrorType: generator.ReceiptErrorType,
			},
		})
		atomic.AddUint64(&l.metrics.FailedContractTransactionsCount, 1)

		return fmt.Errorf("could not get the receipt, %w", err)
	}

	end := time.Now()
	// initialize gas metrics map with block nuber as index
	l.metrics.ContractGasMetrics.Blocks[receipt.BlockNumber] = GasMetrics{}
	// fetch contract address
	l.metrics.ContractAddress = receipt.ContractAddress
	// set contract address in order to get new example txn and gas estimate
	l.generator.SetContractAddress(types.StringToAddress(
		receipt.ContractAddress.String(),
	))

	// we're done with SC deployment
	// we defined SC address and
	// now get new gas estimates for CS token transfers
	l.updateGasEstimate(jsonClient)

	// record contract deployment metrics
	l.metrics.ContractDeploymentDuration.reportTurnAroundTime(
		txHash,
		&metadata{
			turnAroundTime: end.Sub(start),
			blockNumber:    receipt.BlockNumber,
		},
	)
	// calculate contract deployment metrics
	if err := l.calculateGasMetrics(jsonClient, l.metrics.ContractGasMetrics); err != nil {
		return fmt.Errorf("unable to calculate contract block gas metrics: %w", err)
	}

	l.metrics.ContractDeploymentDuration.calcTurnAroundMetrics()
	l.metrics.ContractDeploymentDuration.TotalExecTime = end.Sub(start)

	return nil
}
