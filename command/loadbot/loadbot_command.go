package loadbot

import (
	"flag"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
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
		Description: "Number of transactions to send per second. Default: 1000",
		Arguments: []string{
			"TPS",
		},
		ArgumentsOptional: true,
	}

	l.FlagMap["accountsCount"] = helper.FlagDescriptor{
		Description: "How many accounts must be used by the loadbot to send transactions. Default: 1000",
		Arguments: []string{
			"ACCOUNTS_COUNT",
		},
		ArgumentsOptional: true,
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
	var accountsCount uint64

	// Map flags to placeholders
	flags.Uint64Var(&tps, "tps", 1000, "")
	flags.Uint64Var(&accountsCount, "accountsCount", 1000, "")

	// Parse cli arguments
	if err := flags.Parse(args); err != nil {
		l.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}

	fmt.Println("TPS:", tps)
	fmt.Println("Accounts count", accountsCount)

	return 0
}
