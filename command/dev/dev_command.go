package dev

import (
	"fmt"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/0xPolygon/minimal/network"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
)

// DevCommand is the command to show the version of the agent
type DevCommand struct {
	UI cli.Ui

	helper.Meta
}

// DefineFlags defines the command flags
func (d *DevCommand) DefineFlags() {
	if d.FlagMap == nil {
		// Flag map not initialized
		d.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	d.FlagMap["log-level"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the log level for console output. Default: %s", helper.DefaultConfig().LogLevel),
		Arguments: []string{
			"LOG_LEVEL",
		},
		FlagOptional: true,
	}

	d.FlagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the genesis file used for starting the chain. Default: %s", helper.DefaultConfig().Chain),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	d.FlagMap["data-dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the data directory used for storing Polygon SDK client data. Default: %s", helper.DefaultConfig().DataDir),
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		FlagOptional: true,
	}

	d.FlagMap["grpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the gRPC service (address:port). Default: address: 127.0.0.1:%d", minimal.DefaultGRPCPort),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	d.FlagMap["jsonrpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the JSON-RPC service (address:port). Default: address: 127.0.0.1:%d", minimal.DefaultJSONRPCPort),
		Arguments: []string{
			"JSONRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	d.FlagMap["libp2p"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the libp2p service (address:port). Default: address: 127.0.0.1:%d", network.DefaultLibp2pPort),
		Arguments: []string{
			"LIBP2P_ADDRESS",
		},
		FlagOptional: true,
	}

	d.FlagMap["dev-interval"] = helper.FlagDescriptor{
		Description: "Sets the client's dev notification interval. Default: 0",
		Arguments: []string{
			"DEV_INTERVAL",
		},
		FlagOptional: true,
	}
}

func (d *DevCommand) GetHelperText() string {
	return "dev \"bypasses\" consensus and networking and starts a blockchain locally. " +
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
	conf, err := helper.ReadDevConfig(d.GetBaseCommand(), args)
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
		Level: hclog.LevelFromString("debug"),
	})

	server, err := minimal.NewServer(logger, config)
	if err != nil {
		d.UI.Error(err.Error())

		return 1
	}

	return helper.HandleSignals(server.Close, d.UI)
}
