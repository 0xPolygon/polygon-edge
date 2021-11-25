package loadbot

import (
	"flag"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	helperFlags "github.com/0xPolygon/polygon-sdk/helper/flags"
	"github.com/0xPolygon/polygon-sdk/types"
)

type LoadbotCommand struct {
	helper.Meta
}

func (l *LoadbotCommand) DefineFlags() {
	if l.FlagMap == nil {
		l.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	l.FlagMap["tps"] = helper.FlagDescriptor{
		Description: "Number of transactions executed per second by the loadbot",
		Arguments: []string{
			"TPS",
		},
		FlagOptional: true,
	}

	l.FlagMap["account"] = helper.FlagDescriptor{
		Description: "Sets the account the use while running stress tests",
		Arguments: []string{
			"ACCOUNT",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["gasLimit"] = helper.FlagDescriptor{
		Description: "The specified gas limit",
		Arguments: []string{
			"LIMIT",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	l.FlagMap["gasPrice"] = helper.FlagDescriptor{
		Description: "The gas price",
		Arguments: []string{
			"GASPRICE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	l.FlagMap["url"] = helper.FlagDescriptor{
		Description: "The URL used to make the transaction submission",
		Arguments: []string{
			"URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["chainid"] = helper.FlagDescriptor{
		Description: "The Chain ID used to sign the transactions",
		Arguments: []string{
			"CHAIN_ID",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	l.FlagMap["count"] = helper.FlagDescriptor{
		Description: "The number of transactions to send",
		Arguments: []string{
			"COUNT",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["value"] = helper.FlagDescriptor{
		Description: "The value sent during the transaction by the loadbot",
		Arguments: []string{
			"VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	l.FlagMap["grpc"] = helper.FlagDescriptor{
		Description: "The gRPC url used by the loadbot to verify post load transactions status",
		Arguments: []string{
			"GRPC_URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}
}

func (l *LoadbotCommand) GetHelperText() string {
	return "Runs the loadbot with the given properties (TPS, accounts...)"
}

func (l *LoadbotCommand) GetBaseCommand() string {
	return "loadbot"
}

func (l *LoadbotCommand) Synopsis() string {
	return l.GetHelperText()
}

func (l *LoadbotCommand) Help() string {
	l.Meta.DefineFlags()
	l.DefineFlags()

	return helper.GenerateHelp(l.Synopsis(), helper.GenerateUsage(l.GetBaseCommand(), l.FlagMap), l.FlagMap)
}

func (l *LoadbotCommand) Run(args []string) int {
	flags := flag.NewFlagSet(l.GetBaseCommand(), flag.ContinueOnError)

	var tps uint64
	var accountsRaw helperFlags.ArrayFlags
	var gasLimit uint64
	var gasPriceRaw string
	var urls helperFlags.ArrayFlags
	var chainID uint64
	var count uint64
	var valueRaw string
	var gRPC string

	flags.Uint64Var(&tps, "tps", 100, "")
	flags.Var(&accountsRaw, "account", "")
	flags.Uint64Var(&gasLimit, "gasLimit", 1000000, "")
	flags.StringVar(&gasPriceRaw, "gasPrice", "0x100000", "")
	flags.Var(&urls, "url", "")
	flags.Uint64Var(&chainID, "chainid", helper.DefaultChainID, "")
	flags.Uint64Var(&count, "count", 1000, "")
	flags.StringVar(&valueRaw, "value", "", "")
	flags.StringVar(&gRPC, "grpc", "", "")

	if err := flags.Parse(args); err != nil {
		l.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}

	// Parse accountsRaw
	var addresses = []types.Address{}
	if accountsRaw == nil {
		l.UI.Error("failed to parse accounts used by the loadbot")
		return 1
	}
	for _, account := range accountsRaw {
		placeholder := types.Address{}
		if err := placeholder.UnmarshalText([]byte(account)); err != nil {
			l.UI.Error(fmt.Sprintf("Failed to decode account address: %v", err))
			return 1
		}
		addresses = append(addresses, placeholder)
	}

	// Parse urls
	if len(urls) == 0 {
		l.UI.Error("please provide at least one node url to run the loadbot")
		return 1
	}

	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		l.UI.Error(fmt.Sprintf("Failed to decode to value: %v", err))
		return 1
	}
	gasPrice, err := types.ParseUint256orHex(&gasPriceRaw)
	if err != nil {
		l.UI.Error(fmt.Sprintf("Failed to decode to gasPrice: %v", err))
		return 1
	}

	var accounts []types.Address
	for _, account := range accountsRaw {
		acc := types.Address{}
		if err := acc.UnmarshalText([]byte(account)); err != nil {
			l.UI.Error(fmt.Sprintf("Failed to decode to address: %v", err))
			return 1
		}

		accounts = append(accounts, acc)
	}

	metrics, err := Execute(&Configuration{
		TPS:       tps,
		Value:     value,
		Gas:       gasLimit,
		GasPrice:  gasPrice,
		Accounts:  accounts,
		RPCURLs:   urls,
		ChainID:   chainID,
		TxnToSend: count,
		GRPCUrl:   gRPC,
	})
	if err != nil {
		l.UI.Error(fmt.Sprintf("failed to execute loadbot: %v", err))
		return 1
	}

	output := "\n[LOADBOT RUN]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Transactions submitted|%d", metrics.Total),
		fmt.Sprintf("Transactions failed|%d", metrics.Failed),
		fmt.Sprintf("Duration|%v", metrics.Duration),
	})
	output += "\n"

	l.UI.Output(output)

	return 0
}
