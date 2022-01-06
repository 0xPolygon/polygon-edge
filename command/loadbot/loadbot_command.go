package loadbot

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/command/loadbot/generator"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"net"
	"net/url"
	"sort"
)

type LoadbotCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
}

const (
	durationPrecision = 5
)

func (l *LoadbotCommand) DefineFlags() {
	l.Base.DefineFlags(l.Formatter)

	l.FlagMap["tps"] = helper.FlagDescriptor{
		Description: "Number of transactions to send per second. Default: 100",
		Arguments: []string{
			"TPS",
		},
		ArgumentsOptional: true,
	}

	l.FlagMap["sender"] = helper.FlagDescriptor{
		Description: "The account used to send the transactions.",
		Arguments: []string{
			"SENDER",
		},
		ArgumentsOptional: true,
	}

	l.FlagMap["receiver"] = helper.FlagDescriptor{
		Description: "The account used to receive the transactions.",
		Arguments: []string{
			"RECEIVER",
		},
		ArgumentsOptional: true,
	}

	l.FlagMap["value"] = helper.FlagDescriptor{
		Description: "The value sent in each transaction in wei. If negative, " +
			"a random value will be generated. Default: 100",
		Arguments: []string{
			"VALUE",
		},
		ArgumentsOptional: true,
	}

	l.FlagMap["count"] = helper.FlagDescriptor{
		Description: "The number of transactions to sent in total. Default: 1000",
		Arguments: []string{
			"COUNT",
		},
		ArgumentsOptional: true,
	}

	l.FlagMap["jsonrpc"] = helper.FlagDescriptor{
		Description: "The JSON-RPC endpoint used to query transactions.",
		Arguments: []string{
			"JSONRPC_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["grpc"] = helper.FlagDescriptor{
		Description: "The GRPC endpoint used to send transactions.",
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["mode"] = helper.FlagDescriptor{
		Description: "The mode of operation [0, 1]. Default: 0",
		Arguments: []string{
			"MODE",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["chain-id"] = helper.FlagDescriptor{
		Description: "The network chain ID. Default: 100",
		Arguments: []string{
			"CHAIN_ID",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["detailed"] = helper.FlagDescriptor{
		Description: "Flag indicating if the error logs should be shown. Default: false",
		Arguments: []string{
			"DETAILED",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}
}

func (l *LoadbotCommand) GetHelperText() string {
	return "Runs the loadbot to stress test the network"
}

func (l *LoadbotCommand) GetBaseCommand() string {
	return "loadbot"
}

func (l *LoadbotCommand) Synopsis() string {
	return l.GetHelperText()
}

func (l *LoadbotCommand) Help() string {
	l.DefineFlags()

	return helper.GenerateHelp(l.Synopsis(), helper.GenerateUsage(l.GetBaseCommand(), l.FlagMap), l.FlagMap)
}

func (l *LoadbotCommand) Run(args []string) int {
	flags := l.NewFlagSet(l.GetBaseCommand(), l.Formatter)

	// Placeholders for flags
	var tps uint64
	var mode uint64
	var chainID uint64
	var senderRaw string
	var receiverRaw string
	var valueRaw string
	var count uint64
	var jsonrpc string
	var grpc string
	var maxConns int
	var detailed bool
	// Map flags to placeholders
	flags.Uint64Var(&tps, "tps", 100, "")
	flags.Uint64Var(&mode, "mode", 0, "")
	flags.BoolVar(&detailed, "detailed", false, "")
	flags.Uint64Var(&chainID, "chain-id", 100, "")
	flags.StringVar(&senderRaw, "sender", "", "")
	flags.StringVar(&receiverRaw, "receiver", "", "")
	flags.StringVar(&valueRaw, "value", "0x100", "")
	flags.Uint64Var(&count, "count", 1000, "")
	flags.StringVar(&jsonrpc, "jsonrpc", "", "")
	flags.StringVar(&grpc, "grpc", "", "")
	flags.IntVar(&maxConns, "max-conns", 0, "")

	var err error
	// Parse cli arguments
	if err = flags.Parse(args); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to parse args: %w", err))
		return 1
	}

	if mode > 1 {
		l.Formatter.OutputError(errors.New("invalid loadbot mode"))
		return 1
	}

	// maxConns is set to 2*tps if not specified by the user.
	if maxConns == 0 {
		maxConns = int(2 * tps)
	}
	var sender types.Address
	if err = sender.UnmarshalText([]byte(senderRaw)); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to decode sender address: %w", err))
		return 1
	}

	var receiver types.Address
	if err = receiver.UnmarshalText([]byte(receiverRaw)); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to decode receiver address: %w", err))
		return 1
	}

	if _, err := url.ParseRequestURI(jsonrpc); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Invalid JSON-RPC url : %w", err))
		return 1
	}

	if _, err := net.ResolveTCPAddr("tcp", grpc); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Invalid GRPC url : %w", err))
		return 1
	}

	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to decode to value: %w", err))
		return 1
	}

	configuration := &Configuration{
		TPS:           tps,
		Sender:        sender,
		Receiver:      receiver,
		Count:         count,
		Value:         value,
		JSONRPC:       jsonrpc,
		GRPC:          grpc,
		MaxConns:      maxConns,
		GeneratorMode: mode,
		ChainID:       chainID,
	}

	// Create the metrics placeholder
	metrics := &Metrics{
		TotalTransactionsSentCount: 0,
		FailedTransactionsCount:    0,
		TransactionDuration: ExecDuration{
			blockTransactions: make(map[uint64]uint64),
		},
	}

	// create a loadbot instance
	loadBot := NewLoadBot(configuration, metrics)

	// run the loadbot
	if err := loadBot.Run(); err != nil {
		l.Formatter.OutputError(fmt.Errorf("an error occured while running the loadbot: %w", err))
		return 1
	}

	res := &LoadbotResult{
		CountData: TxnCountData{
			Total:  metrics.TotalTransactionsSentCount,
			Failed: metrics.FailedTransactionsCount,
		},
	}
	res.extractExecutionData(metrics)

	if detailed {
		res.extractDetailedErrors(loadBot.generator)
	}

	l.Formatter.OutputResult(res)

	return 0
}

type TxnCountData struct {
	Total  uint64 `json:"total"`
	Failed uint64 `json:"failed"`
}

type TxnTurnAroundData struct {
	FastestTurnAround float64 `json:"fastestTurnAround"`
	SlowestTurnAround float64 `json:"slowestTurnAround"`
	AverageTurnAround float64 `json:"averageTurnAround"`
	TotalExecTime     float64 `json:"totalExecTime"`
}

type TxnBlockData struct {
	// BlocksRequired is the required number of blocks to seal the data
	BlocksRequired uint64 `json:"blocksRequired"`

	// BlockTransactionsMap maps the block number to the number of loadbot transactions in it
	BlockTransactionsMap map[uint64]uint64 `json:"blockTransactionsMap"`
}

type TxnDetailedErrorData struct {
	// DetailedErrorMap groups transaction errors by error type, with each transaction hash
	// mapping to its specific error
	DetailedErrorMap map[generator.TxnErrorType][]*generator.FailedTxnInfo `json:"detailedErrorMap"`
}

type LoadbotResult struct {
	CountData         TxnCountData         `json:"countData"`
	TurnAroundData    TxnTurnAroundData    `json:"turnAroundData"`
	BlockData         TxnBlockData         `json:"blockData"`
	DetailedErrorData TxnDetailedErrorData `json:"detailedErrorData"`
}

func (lr *LoadbotResult) extractExecutionData(metrics *Metrics) {
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
	}
}

func (lr *LoadbotResult) extractDetailedErrors(gen generator.TransactionGenerator) {
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

func (lr *LoadbotResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n=====[LOADBOT RUN]=====\n")
	buffer.WriteString("\n[COUNT DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Transactions submitted|%d", lr.CountData.Total),
		fmt.Sprintf("Transactions failed|%d", lr.CountData.Failed),
	}))

	buffer.WriteString("\n\n[TURN AROUND DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Average transaction turn around|%fs", lr.TurnAroundData.AverageTurnAround),
		fmt.Sprintf("Fastest transaction turn around|%fs", lr.TurnAroundData.FastestTurnAround),
		fmt.Sprintf("Slowest transaction turn around|%fs", lr.TurnAroundData.SlowestTurnAround),
		fmt.Sprintf("Total loadbot execution time|%fs", lr.TurnAroundData.TotalExecTime),
	}))

	buffer.WriteString("\n\n[BLOCK DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Blocks required|%d", lr.BlockData.BlocksRequired),
	}))

	if lr.BlockData.BlocksRequired != 0 {
		buffer.WriteString("\n\n")

		keys := make([]uint64, 0, lr.BlockData.BlocksRequired)
		for k := range lr.BlockData.BlockTransactionsMap {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return keys[i] < keys[j]
		})

		formattedStrings := make([]string, 0)
		for _, blockNumber := range keys {
			formattedStrings = append(formattedStrings,
				fmt.Sprintf("Block #%d|%d txns", blockNumber, lr.BlockData.BlockTransactionsMap[blockNumber]),
			)
		}

		buffer.WriteString(helper.FormatKV(formattedStrings))
	}

	// Write out the error logs if detailed view
	// is requested
	if len(lr.DetailedErrorData.DetailedErrorMap) != 0 {
		buffer.WriteString("\n\n[DETAILED ERRORS]\n")

		addToBuffer := func(detailedError *generator.FailedTxnInfo) {
			if detailedError.TxHash != web3.ZeroHash.String() {
				buffer.WriteString(fmt.Sprintf("\n\n[%s]\n", detailedError.TxHash))
			} else {
				buffer.WriteString("\n\n[Tx Hash Unavailable]\n")
			}

			formattedStrings := make([]string, 0)
			formattedStrings = append(formattedStrings,
				fmt.Sprintf("Index|%d", detailedError.Index),
				fmt.Sprintf("Error|%s", detailedError.Error.Error.Error()),
			)

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

	buffer.WriteString("\n")

	return buffer.String()
}
