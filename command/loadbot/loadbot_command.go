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
		Description: fmt.Sprintf("Number of transactions executed per second by the loadbot"),
		Arguments: []string{
			"TPS",
		},
		FlagOptional: true,
	}

	l.FlagMap["account"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the account the use while running stress tests"),
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

	l.FlagMap["chain-id"] = helper.FlagDescriptor{
		Description: "The Chain ID used to sign the transactions",
		Arguments: []string{
			"CHAIN_ID",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	l.FlagMap["max-pending"] = helper.FlagDescriptor{
		Description: "The maximum number of pending transaction the loadbot can process",
		Arguments: []string{
			"MAX_PENDING",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	l.FlagMap["value"] = helper.FlagDescriptor{
		Description: "The value sent during the transaction by the loadbot",
		Arguments: []string{
			"VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
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
	var accounts helperFlags.ArrayFlags
	var gasLimit uint64
	var gasPriceRaw string
	var urls helperFlags.ArrayFlags
	var chainID uint64
	var maxPending uint64
	var valueRaw string

	flags.Uint64Var(&tps, "tps", 100, "")
	flags.Var(&accounts, "account", "")
	flags.Uint64Var(&gasLimit, "gasLimit", 1000000, "")
	flags.StringVar(&gasPriceRaw, "gasPrice", "0x100000", "")
	flags.Var(&urls, "url", "")
	flags.Uint64Var(&chainID, "chain-id", helper.DefaultChainID, "")
	flags.Uint64Var(&maxPending, "max-pending", 1000, "")
	flags.StringVar(&valueRaw, "value", "", "")

	if err := flags.Parse(args); err != nil {
		l.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}

	// Parse accounts
	var addresses = []types.Address{}
	if accounts == nil {
		l.UI.Error(fmt.Sprintf("failed to parse accounts used by the loadbot"))
		return 1
	}
	for _, account := range accounts {
		placeholder := types.Address{}
		if err := placeholder.UnmarshalText([]byte(account)); err != nil {
			l.UI.Error(fmt.Sprintf("Failed to decode account address: %v", err))
			return 1
		}
		addresses = append(addresses, placeholder)
	}

	// Parse urls
	if len(urls) == 0 {
		l.UI.Error(fmt.Sprintf("please provide at least one node url to run the loabot"))
		return 1
	}

	return 0
}
