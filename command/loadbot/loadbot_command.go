package loadbot

import (
	"bytes"
	"fmt"
	"net/url"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/types"
)

type LoadbotCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
}

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

	// Map flags to placeholders
	flags.Uint64Var(&tps, "tps", 100, "")
	flags.StringVar(&senderRaw, "sender", "", "")
	flags.StringVar(&receiverRaw, "receiver", "", "")
	flags.StringVar(&valueRaw, "value", "0x100", "")
	flags.Uint64Var(&count, "count", 1000, "")
	flags.StringVar(&jsonrpc, "jsonrpc", "", "")

	var err error
	// Parse cli arguments
	if err = flags.Parse(args); err != nil {
		l.Formatter.OutputError(fmt.Errorf("Failed to parse args: %w", err))
		return 1
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
	}

	// Create the metrics placeholder
	metrics := &Metrics{
		TotalTransactionsSentCount: 0,
		FailedTransactionsCount:    0,
	}

	// create a loadbot instance
	loadBot := NewLoadBot(configuration, metrics)

	// run the loadbot
	if err := loadBot.Run(); err != nil {
		l.Formatter.OutputError(fmt.Errorf("an error occured while running the loadbot: %w", err))
		return 1
	}

	res := &LoadbotResult{
		Total:  metrics.TotalTransactionsSentCount,
		Failed: metrics.FailedTransactionsCount,
	}
	l.Formatter.OutputResult(res)

	return 0
}

type LoadbotResult struct {
	Total  uint64 `json:"total"`
	Failed uint64 `json:"failed"`
}

func (r *LoadbotResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[LOADBOT RUN]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Transactions submitted|%d", r.Total),
		fmt.Sprintf("Transactions failed|%d", r.Failed),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
