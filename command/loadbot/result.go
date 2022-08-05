package loadbot

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/loadbot/generator"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

const (
	durationPrecision = 5
)

type TxnCountData struct {
	Total  uint64 `json:"total"`
	Failed uint64 `json:"failed"`
}

type TxnTurnAroundData struct {
	FastestTurnAround float64 `json:"fastest_turn_around"`
	SlowestTurnAround float64 `json:"slowest_turn_around"`
	AverageTurnAround float64 `json:"average_turn_around"`
	TotalExecTime     float64 `json:"total_exec_time"`
}

type TxnBlockData struct {
	// BlocksRequired is the required number of blocks to seal the data
	BlocksRequired uint64 `json:"blocks_required"`

	// BlockTransactionsMap maps the block number to the number of loadbot transactions in it
	BlockTransactionsMap map[uint64]uint64 `json:"block_transactions_map"`

	// Total amount of gas used in block
	GasData map[uint64]GasMetrics `json:"gas_used"`
}

type TxnDetailedErrorData struct {
	// DetailedErrorMap groups transaction errors by error type, with each transaction hash
	// mapping to its specific error
	DetailedErrorMap map[generator.TxnErrorType][]*generator.FailedTxnInfo `json:"detailed_error_map"`
}

type LoadbotResult struct {
	CountData              TxnCountData         `json:"count_data"`
	TurnAroundData         TxnTurnAroundData    `json:"turn_around_data"`
	ContractTurnAroundData TxnTurnAroundData    `json:"contract_turn_around_data"`
	BlockData              TxnBlockData         `json:"block_data"`
	DetailedErrorData      TxnDetailedErrorData `json:"detailed_error_data,omitempty"`
	ApproxTPS              uint64               `json:"approx_tps"`
	ContractAddress        ethgo.Address        `json:"contract_address,omitempty"`
	ContractBlockData      TxnBlockData         `json:"contract_block_data,omitempty"`
}

func (lr *LoadbotResult) initExecutionData(metrics *Metrics) {
	// calculate real transactions per second value
	// by dividing total transactions by total runtime in seconds
	lr.ApproxTPS = metrics.TotalTransactionsSentCount /
		uint64(math.Floor(metrics.TransactionDuration.TotalExecTime.Seconds()))

	lr.TurnAroundData.FastestTurnAround = common.ToFixedFloat(
		metrics.TransactionDuration.FastestTurnAround.Seconds(),
		durationPrecision,
	)

	lr.TurnAroundData.SlowestTurnAround = common.ToFixedFloat(
		metrics.TransactionDuration.SlowestTurnAround.Seconds(),
		durationPrecision,
	)

	lr.TurnAroundData.AverageTurnAround = common.ToFixedFloat(
		metrics.TransactionDuration.AverageTurnAround.Seconds(),
		durationPrecision,
	)

	lr.TurnAroundData.TotalExecTime = common.ToFixedFloat(
		metrics.TransactionDuration.TotalExecTime.Seconds(),
		durationPrecision,
	)

	lr.BlockData = TxnBlockData{
		BlocksRequired:       uint64(len(metrics.TransactionDuration.blockTransactions)),
		BlockTransactionsMap: metrics.TransactionDuration.blockTransactions,
		GasData:              metrics.GasMetrics.Blocks,
	}
}

func (lr *LoadbotResult) initContractDeploymentModesExecutionData(metrics *Metrics) {
	// set contract deployment metrics
	lr.ContractTurnAroundData.FastestTurnAround = common.ToFixedFloat(
		metrics.ContractMetrics.ContractDeploymentDuration.FastestTurnAround.Seconds(),
		durationPrecision,
	)
	lr.ContractTurnAroundData.SlowestTurnAround = common.ToFixedFloat(
		metrics.ContractMetrics.ContractDeploymentDuration.SlowestTurnAround.Seconds(),
		durationPrecision,
	)
	lr.ContractTurnAroundData.AverageTurnAround = common.ToFixedFloat(
		metrics.ContractMetrics.ContractDeploymentDuration.AverageTurnAround.Seconds(),
		durationPrecision,
	)
	lr.ContractTurnAroundData.TotalExecTime = common.ToFixedFloat(
		metrics.ContractMetrics.ContractDeploymentDuration.TotalExecTime.Seconds(),
		durationPrecision,
	)
	// set contract address
	lr.ContractAddress = metrics.ContractMetrics.ContractAddress
	lr.ContractBlockData = TxnBlockData{
		BlocksRequired:       uint64(len(metrics.ContractMetrics.ContractDeploymentDuration.blockTransactions)),
		BlockTransactionsMap: metrics.ContractMetrics.ContractDeploymentDuration.blockTransactions,
		GasData:              metrics.ContractMetrics.ContractGasMetrics.Blocks,
	}
}

func (lr *LoadbotResult) initDetailedErrors(gen generator.TransactionGenerator) {
	transactionErrors := gen.GetTransactionErrors()
	if len(transactionErrors) == 0 {
		return
	}

	errMap := make(map[generator.TxnErrorType][]*generator.FailedTxnInfo)

	for _, txnError := range transactionErrors {
		errArray, ok := errMap[txnError.Error.ErrorType]
		if !ok {
			errArray = make([]*generator.FailedTxnInfo, 0)
		}

		errArray = append(errArray, txnError)

		errMap[txnError.Error.ErrorType] = errArray
	}

	lr.DetailedErrorData.DetailedErrorMap = errMap
}

func (lr *LoadbotResult) writeBlockData(buffer *bytes.Buffer) {
	blockData := &lr.BlockData

	buffer.WriteString("\n\n[BLOCK DATA]\n")

	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Blocks required|%d", blockData.BlocksRequired),
	}))

	if blockData.BlocksRequired != 0 {
		buffer.WriteString("\n\n")

		keys := make([]uint64, 0, blockData.BlocksRequired)

		for k := range blockData.BlockTransactionsMap {
			keys = append(keys, k)
		}

		sort.Slice(keys, func(i, j int) bool {
			return keys[i] < keys[j]
		})

		formattedStrings := make([]string, 0)

		for _, blockNumber := range keys {
			formattedStrings = append(formattedStrings,
				fmt.Sprintf("Block #%d|%d txns (%d gasUsed / %d gasLimit) utilization | %.2f%%",
					blockNumber,
					blockData.BlockTransactionsMap[blockNumber],
					blockData.GasData[blockNumber].GasUsed,
					blockData.GasData[blockNumber].GasLimit,
					blockData.GasData[blockNumber].Utilization,
				))
		}

		buffer.WriteString(helper.FormatKV(formattedStrings))
	}
}

func (lr *LoadbotResult) writeErrorData(buffer *bytes.Buffer) {
	// Write out the error logs if detailed view
	// is requested
	if len(lr.DetailedErrorData.DetailedErrorMap) != 0 {
		buffer.WriteString("\n\n[DETAILED ERRORS]\n")

		addToBuffer := func(detailedError *generator.FailedTxnInfo) {
			if detailedError.TxHash != ethgo.ZeroHash.String() {
				buffer.WriteString(fmt.Sprintf("\n\n[%s]\n", detailedError.TxHash))
			} else {
				buffer.WriteString("\n\n[Tx Hash Unavailable]\n")
			}

			formattedStrings := []string{
				fmt.Sprintf("Index|%d", detailedError.Index),
				fmt.Sprintf("Error|%s", detailedError.Error.Error.Error()),
			}

			buffer.WriteString(helper.FormatKV(formattedStrings))
		}

		receiptErrors, ok := lr.DetailedErrorData.DetailedErrorMap[generator.ReceiptErrorType]
		if ok {
			buffer.WriteString("[RECEIPT ERRORS]\n")

			for _, receiptError := range receiptErrors {
				addToBuffer(receiptError)
			}
		}

		addErrors, ok := lr.DetailedErrorData.DetailedErrorMap[generator.AddErrorType]
		if ok {
			buffer.WriteString("[ADD ERRORS]\n")

			for _, addError := range addErrors {
				addToBuffer(addError)
			}
		}
	}
}

func (lr *LoadbotResult) GetOutput() string {
	buffer := new(bytes.Buffer)

	lr.writeLoadbotResults(buffer)

	return buffer.String()
}

func (lr *LoadbotResult) writeLoadbotResults(buffer *bytes.Buffer) {
	buffer.WriteString("\n=====[LOADBOT RUN]=====\n")

	lr.writeCountData(buffer)
	lr.writeApproximateTPSData(buffer)
	lr.writeContractDeploymentData(buffer)
	lr.writeTurnAroundData(buffer)
	lr.writeBlockData(buffer)
	lr.writeAverageBlockUtilization(buffer)
	lr.writeErrorData(buffer)

	buffer.WriteString("\n")
}

func (lr *LoadbotResult) writeAverageBlockUtilization(buffer *bytes.Buffer) {
	buffer.WriteString("\n\n[AVERAGE BLOCK UTILIZATION]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Average utilization across all blocks|%.2f%%", calculateAvgBlockUtil(lr.BlockData.GasData)),
	}))
}

func (lr *LoadbotResult) writeCountData(buffer *bytes.Buffer) {
	buffer.WriteString("\n[COUNT DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Transactions submitted|%d", lr.CountData.Total),
		fmt.Sprintf("Transactions failed|%d", lr.CountData.Failed),
	}))
}

func (lr *LoadbotResult) writeApproximateTPSData(buffer *bytes.Buffer) {
	buffer.WriteString("\n\n[APPROXIMATE TPS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Approximate number of transactions per second|%d", lr.ApproxTPS),
	}))
}

func (lr *LoadbotResult) writeTurnAroundData(buffer *bytes.Buffer) {
	buffer.WriteString("\n\n[TURN AROUND DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Average transaction turn around|%fs", lr.TurnAroundData.AverageTurnAround),
		fmt.Sprintf("Fastest transaction turn around|%fs", lr.TurnAroundData.FastestTurnAround),
		fmt.Sprintf("Slowest transaction turn around|%fs", lr.TurnAroundData.SlowestTurnAround),
		fmt.Sprintf("Total loadbot execution time|%fs", lr.TurnAroundData.TotalExecTime),
	}))
}

func (lr *LoadbotResult) writeContractDeploymentData(buffer *bytes.Buffer) {
	// skip if contract was not deployed
	if lr.ContractAddress == ethgo.ZeroAddress {
		return
	}

	buffer.WriteString("\n\n[CONTRACT DEPLOYMENT INFO]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Contract address|%s", lr.ContractAddress),
		fmt.Sprintf("Total execution time|%fs", lr.ContractTurnAroundData.TotalExecTime),
	}))

	buffer.WriteString("\n\n[CONTRACT DEPLOYMENT BLOCK DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Blocks required|%d", lr.ContractBlockData.BlocksRequired),
	}))
	buffer.WriteString("\n")

	formattedStrings := make([]string, 0)

	for blockNumber := range lr.ContractBlockData.BlockTransactionsMap {
		formattedStrings = append(formattedStrings,
			fmt.Sprintf("Block #%d|%d txns (%d gasUsed / %d gasLimit) utilization | %.2f%%",
				blockNumber,
				lr.ContractBlockData.BlockTransactionsMap[blockNumber],
				lr.ContractBlockData.GasData[blockNumber].GasUsed,
				lr.ContractBlockData.GasData[blockNumber].GasLimit,
				lr.ContractBlockData.GasData[blockNumber].Utilization,
			))
	}

	buffer.WriteString(helper.FormatKV(formattedStrings))
}

func newLoadbotResult(metrics *Metrics, mode Mode) *LoadbotResult {
	res := &LoadbotResult{
		CountData: TxnCountData{
			Total:  metrics.TotalTransactionsSentCount,
			Failed: metrics.FailedTransactionsCount,
		},
	}

	if mode == erc20 || mode == erc721 {
		res.initContractDeploymentModesExecutionData(metrics)
	}

	res.initExecutionData(metrics)

	return res
}
