package loadbot

import (
	"flag"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	helperFlags "github.com/0xPolygon/polygon-sdk/helper/flags"
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

	l.FlagMap["accountsCount"] = helper.FlagDescriptor{
		Description: "How many accounts must be used by the loadbot to send transactions. Default: 1000",
		Arguments: []string{
			"ACCOUNTS_COUNT",
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
		Description: "The JSON-RPC endpoint used to send transactions. You can provide multiple endpoints.",
		Arguments: []string{
			"JSONRPC_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	l.FlagMap["grpc"] = helper.FlagDescriptor{
		Description: "The gRPC endpoint used to verify TxPool. " +
			"You must provide a gRPC endpoint for each one of the JSON-RPC you provided.",
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
	var accountsCount uint64
	var valueRaw string
	var count uint64
	var jsonrpcs helperFlags.ArrayFlags
	var grpcs helperFlags.ArrayFlags

	// Map flags to placeholders
	flags.Uint64Var(&tps, "tps", 100, "")
	flags.Uint64Var(&accountsCount, "accountsCount", 1000, "")
	flags.StringVar(&valueRaw, "value", "-1", "")
	flags.Uint64Var(&count, "count", 1000, "")
	flags.Var(&jsonrpcs, "jsonrpc", "")
	flags.Var(&grpcs, "grpc", "")

	var err error
	// Parse cli arguments
	if err = flags.Parse(args); err != nil {
		l.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}

	// Trying to parse value is a custom one is provided
	var value int64 = -1
	if valueRaw != "-1" {
		value, err = types.ParseInt64orHex(&valueRaw)
		if err != nil {
			l.UI.Error(fmt.Sprintf("failed to parse value: %v", err))
			return 1
		}
	}

	// There must be at least one JSON-RPC endpoint
	if len(jsonrpcs) == 0 {
		l.UI.Error("No JSON-RPC endpoint provided")
		return 1
	}

	// There must be at least one gRPC endpoint
	if len(grpcs) == 0 {
		l.UI.Error("No gRPC endpoint provided")
		return 1
	}

	configuration := Configuration{
		TPS:           tps,
		AccountsCount: accountsCount,
		Value:         value,
		Count:         count,
		JSONRPCs:      jsonrpcs,
		GRPCs:         grpcs,
	}

	err, m := Run(&configuration)
	if err != nil {
		l.UI.Error(fmt.Sprintf("an error occured while running the loadbot: %v", err))
		return 1
	}

	fmt.Println(m)

	return 0
}
