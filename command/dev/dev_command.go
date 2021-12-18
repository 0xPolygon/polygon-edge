package dev

import (
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/server"
	"github.com/hashicorp/go-hclog"
)

// DevCommand is the command to show the version of the agent
type DevCommand struct {
	helper.Base
}

// DefineFlags defines the command flags
func (d *DevCommand) DefineFlags() {
	d.Base.DefineFlags()

	d.FlagMap["log-level"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the log level for console output. Default: %s", helper.DefaultConfig().LogLevel),
		Arguments: []string{
			"LOG_LEVEL",
		},
		FlagOptional: true,
	}

	d.FlagMap["premine"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the premined accounts and balances. Default premined balance: %s", helper.DefaultPremineBalance),
		Arguments: []string{
			"ADDRESS:VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	d.FlagMap["dev-interval"] = helper.FlagDescriptor{
		Description: "Sets the client's dev notification interval. Default: 0",
		Arguments: []string{
			"DEV_INTERVAL",
		},
		FlagOptional: true,
	}

	d.FlagMap["locals"] = helper.FlagDescriptor{
		Description: "Sets comma separated accounts whose transactions are treated as locals",
		Arguments: []string{
			"LOCALS",
		},
		FlagOptional: true,
	}

	d.FlagMap["nolocals"] = helper.FlagDescriptor{
		Description: "Sets flag to disable price exemptions for locally submitted transactions",
		Arguments: []string{
			"NOLOCALS",
		},
		FlagOptional: true,
	}

	d.FlagMap["price-limit"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets minimum gas price limit to enforce for acceptance into the pool. Default: %d", helper.DefaultConfig().TxPool.PriceLimit),
		Arguments: []string{
			"PRICE_LIMIT",
		},
		FlagOptional: true,
	}

	d.FlagMap["block-gas-limit"] = helper.FlagDescriptor{
		Description: "Sets the gas limit of each block. Default: 5000",
		Arguments: []string{
			"BLOCK_GAS_LIMIT",
		},
		FlagOptional: true,
	}

	d.FlagMap["block-gas-target"] = helper.FlagDescriptor{
		Description: "Sets the target block gas limit for the chain. If omitted, the value of the parent block is used",
		Arguments: []string{
			"BLOCK_GAS_TARGET",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	d.FlagMap["chainid"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the ID of the chain. Default: %d", helper.DefaultChainID),
		Arguments: []string{
			"CHAIN_ID",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

func (d *DevCommand) GetHelperText() string {
	return "\"Bypasses\" consensus and networking and starts a blockchain locally. " +
		"It starts a local node and mines every transaction in a separate block"
}

// Help implements the cli.Command interface
func (d *DevCommand) Help() string {
	d.DefineFlags()

	return helper.GenerateHelp(d.Synopsis(), helper.GenerateUsage(d.GetBaseCommand(), d.FlagMap), d.FlagMap)
}

// Synopsis implements the cli.Command interface
func (d *DevCommand) Synopsis() string {
	return d.GetHelperText()
}

func (d *DevCommand) GetBaseCommand() string {
	return "dev"
}

// Run implements the cli.Command interface
func (d *DevCommand) Run(args []string) int {
	conf, err := helper.BootstrapDevCommand(d.GetBaseCommand(), args)
	if err != nil {
		d.UI.Error(err.Error())

		return 1
	}

	config, err := conf.BuildConfig()
	if err != nil {
		d.UI.Error(err.Error())

		return 1
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "polygon-dev",
		Level: hclog.LevelFromString(conf.LogLevel),
	})

	server, err := server.NewServer(logger, config)
	if err != nil {
		d.UI.Error(err.Error())

		return 1
	}

	return helper.HandleSignals(server.Close, d.UI)
}
