package loadbot

import (
	"flag"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/mitchellh/cli"
)

type LoadbotCommand struct {
	UI      cli.Ui
	FlagMap map[string]helper.FlagDescriptor
}

func (l *LoadbotCommand) DefineFlags() {
	if l.FlagMap == nil {
		l.FlagMap = make(map[string]helper.FlagDescriptor)
	}

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

	l.FlagMap["grpc"] = helper.FlagDescriptor{
		Description: "The gRPC endpoint used to verify TxPool.",
		Arguments: []string{
			"GRPC_ADDRESS",
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
	flags := flag.NewFlagSet(l.GetBaseCommand(), flag.ExitOnError)

	// Placeholders for flags
	var tps uint64
	var senderRaw string
	var receiverRaw string
	var valueRaw string
	var count uint64
	var jsonrpc string
	var grpc string

	// Map flags to placeholders
	flags.Uint64Var(&tps, "tps", 100, "")
	flags.StringVar(&senderRaw, "sender", "", "")
	flags.StringVar(&receiverRaw, "receiver", "", "")
	flags.StringVar(&valueRaw, "value", "0x100", "")
	flags.Uint64Var(&count, "count", 1000, "")
	flags.StringVar(&jsonrpc, "jsonrpc", "", "")
	flags.StringVar(&grpc, "grpc", "", "")

	var err error
	// Parse cli arguments
	if err = flags.Parse(args); err != nil {
		l.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}

	sender := types.Address{}
	if err = sender.UnmarshalText([]byte(senderRaw)); err != nil {
		l.UI.Error(fmt.Sprintf("Failed to decode sender address: %v", err))
		return 1
	}

	receiver := types.Address{}
	if err = receiver.UnmarshalText([]byte(receiverRaw)); err != nil {
		l.UI.Error(fmt.Sprintf("Failed to decode receiver address: %v", err))
		return 1
	}

	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		l.UI.Error(fmt.Sprintf("Failed to decode to value: %v", err))
		return 1
	}

	configuration := Configuration{
		TPS:      tps,
		Sender:   sender,
		Receiver: receiver,
		Count:    count,
		Value:    value,
		JSONRPC:  jsonrpc,
		GRPC:     grpc,
	}

	// Create the metrics placeholder
	metrics := Metrics{
		Duration:                   0,
		TotalTransactionsSentCount: 0,
		FailedTransactionsCount:    0,
	}

	err = Run(&configuration, &metrics)
	if err != nil {
		l.UI.Error(fmt.Sprintf("an error occured while running the loadbot: %v", err))
		return 1
	}

	output := "\n[LOADBOT RUN]\n"
	output += helper.FormatKV([]string{
		fmt.Sprintf("Transactions submitted|%d", metrics.TotalTransactionsSentCount),
		fmt.Sprintf("Transactions failed|%d", metrics.FailedTransactionsCount),
		fmt.Sprintf("Duration|%v", metrics.Duration),
	})
	output += "\n"

	l.UI.Output(output)

	return 0
}
