package loadbot

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/command/loadbot/generator"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
)

func (l *Loadbot) deployContract(
	grpcClient txpoolOp.TxnPoolOperatorClient,
	jsonClient *jsonrpc.Client,
	receiptTimeout time.Duration) {
	// if the loadbot mode is set to ERC20 or ERC721 we need to deploy smart contract
	if l.cfg.GeneratorMode == erc20 || l.cfg.GeneratorMode == erc721 {
		start := time.Now()

		// deploy smart contract
		txHash, err := l.executeTxn(grpcClient, "contract", &types.ZeroAddress)
		if err != nil {
			fmt.Println("Could not deploy SC. Fatal error:")
			log.Fatalln(err.Error())
			// l.generator.MarkFailedContractTxn(&generator.FailedContractTxnInfo{
			// 	TxHash: txHash.String(),
			// 	Error: &generator.TxnError{
			// 		Error:     err,
			// 		ErrorType: generator.AddErrorType,
			// 	},
			// })
			// atomic.AddUint64(&l.metrics.FailedContractTransactionsCount, 1)

			// return
		}

		ctx, cancel := context.WithTimeout(context.Background(), receiptTimeout)
		defer cancel()

		// and wait for receipt
		receipt, err := tests.WaitForReceipt(ctx, jsonClient.Eth(), txHash)
		// set block number
		l.metrics.ContractGasMetrics.Blocks[receipt.BlockNumber] = GasMetrics{}
		if err != nil {
			l.generator.MarkFailedContractTxn(&generator.FailedContractTxnInfo{
				TxHash: txHash.String(),
				Error: &generator.TxnError{
					Error:     err,
					ErrorType: generator.ReceiptErrorType,
				},
			})
			atomic.AddUint64(&l.metrics.FailedContractTransactionsCount, 1)

			return
		}

		end := time.Now()
		// fetch contract address
		l.metrics.ContractAddress = receipt.ContractAddress

		// record contract deployment metrics
		l.metrics.ContractDeploymentDuration.reportTurnAroundTime(
			txHash,
			&metadata{
				turnAroundTime: end.Sub(start),
				blockNumber:    receipt.BlockNumber,
			},
		)
		// calculate contract deployment metrics
		for k, v := range l.metrics.ContractGasMetrics.Blocks {
			blockInfom, err := jsonClient.Eth().GetBlockByNumber(web3.BlockNumber(k), false)
			if err != nil {
				log.Fatalln("Could not fetch block by number")
			}

			v.GasLimit = blockInfom.GasLimit
			v.GasUsed = blockInfom.GasUsed
			l.metrics.ContractGasMetrics.Blocks[k] = v
		}

		l.metrics.ContractDeploymentDuration.calcTurnAroundMetrics()
		l.metrics.ContractDeploymentDuration.TotalExecTime = end.Sub(start)
	}
}
