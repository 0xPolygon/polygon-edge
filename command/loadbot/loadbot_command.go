package loadbot

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/mitchellh/cli"
)

type LoadbotCommand struct {
	UI cli.Ui

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
		FlagOptional: false,
	}

	l.FlagMap["premine"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the premined accounts and balances. Default premined balance: %s", helper.DefaultPremineBalance),
		Arguments: []string{
			"ADDRESS:VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
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

	l.FlagMap["max-pending"] = helper.FlagDescriptor{
		Description: "The maximum number of pending transaction the loadbot can process",
		Arguments: []string{
			"MAX_PENDING",
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
	return 0
}
