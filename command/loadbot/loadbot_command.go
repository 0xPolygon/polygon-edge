package loadbot

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/types"
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
		Description: "The JSON-RPC endpoint used to send transactions.",
		Arguments: []string{
			"JSONRPC_ADDRESS",
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
	var senderRaw string
	var receiverRaw string
	var valueRaw string
	var count uint64
	var jsonrpc string
	var maxConns int
	// Map flags to placeholders
	flags.Uint64Var(&tps, "tps", 100, "")
	flags.StringVar(&senderRaw, "sender", "", "")
	flags.StringVar(&receiverRaw, "receiver", "", "")
	flags.StringVar(&valueRaw, "value", "0x100", "")
	flags.Uint64Var(&count, "count", 1000, "")
	flags.StringVar(&jsonrpc, "jsonrpc", "", "")
	flags.IntVar(&maxConns, "max-conns", 0, "")

	var err error
	// Parse cli arguments
	if err = flags.Parse(args); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to parse args: %w", err))
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

	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to decode to value: %w", err))
		return 1
	}

	configuration := &Configuration{
		TPS:      tps,
		Sender:   sender,
		Receiver: receiver,
		Count:    count,
		Value:    value,
		JSONRPC:  jsonrpc,
		MaxConns: maxConns,
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

type LoadbotResult struct {
	CountData      TxnCountData      `json:"countData"`
	TurnAroundData TxnTurnAroundData `json:"turnAroundData"`
	BlockData      TxnBlockData      `json:"blockData"`
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

	buffer.WriteString("\n")

	return buffer.String()
}
