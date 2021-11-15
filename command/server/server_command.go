package server

import (
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/server"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
)

// ServerCommand is the command to start the sever
type ServerCommand struct {
	UI cli.Ui

	flagMap map[string]helper.FlagDescriptor
}

// DefineFlags defines the command flags
func (c *ServerCommand) DefineFlags() {
	if c.flagMap == nil {
		// Flag map not initialized
		c.flagMap = make(map[string]helper.FlagDescriptor)
	}

	if len(c.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.flagMap["log-level"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the log level for console output. Default: %s", helper.DefaultConfig().LogLevel),
		Arguments: []string{
			"LOG_LEVEL",
		},
		FlagOptional: true,
	}

	c.flagMap["seal"] = helper.FlagDescriptor{
		Description: "Sets the flag indicating that the client should seal blocks. Default: false",
		Arguments: []string{
			"SHOULD_SEAL",
		},
		FlagOptional: true,
	}

	c.flagMap["block-gas-target"] = helper.FlagDescriptor{
		Description: "Sets the target block gas limit for the chain. If omitted, the value of the parent block is used",
		Arguments: []string{
			"BLOCK_GAS_TARGET",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["config"] = helper.FlagDescriptor{
		Description: "Specifies the path to the CLI config. Supports .json and .hcl",
		Arguments: []string{
			"CLI_CONFIG_PATH",
		},
		FlagOptional: true,
	}

	c.flagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the genesis file used for starting the chain. Default: %s", helper.DefaultConfig().Chain),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	c.flagMap["data-dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the data directory used for storing Polygon SDK client data. Default: %s", helper.DefaultConfig().DataDir),
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		FlagOptional: true,
	}

	c.flagMap["grpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the gRPC service (address:port). Default: address: 127.0.0.1:%d", server.DefaultGRPCPort),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["jsonrpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the JSON-RPC service (address:port). Default: address: 127.0.0.1:%d", server.DefaultJSONRPCPort),
		Arguments: []string{
			"JSONRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["libp2p"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the libp2p service (address:port). Default: address: 127.0.0.1:%d", network.DefaultLibp2pPort),
		Arguments: []string{
			"LIBP2P_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["join"] = helper.FlagDescriptor{
		Description: "Specifies the address of the peer that should be joined",
		Arguments: []string{
			"JOIN_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["nat"] = helper.FlagDescriptor{
		Description: "Sets the external IP address without the port, as it can be seen by peers",
		Arguments: []string{
			"NAT_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["dns"] = helper.FlagDescriptor{
		Description: "Sets the host DNS address",
		Arguments: []string{
			"DNS_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["no-discover"] = helper.FlagDescriptor{
		Description: "Prevents the client from discovering other peers. Default: false",
		Arguments: []string{
			"NO_DISCOVER",
		},
		FlagOptional: true,
	}

	c.flagMap["max-peers"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the client's max peer count. Default: %d", helper.DefaultConfig().Network.MaxPeers),
		Arguments: []string{
			"PEER_COUNT",
		},
		FlagOptional: true,
	}

	c.flagMap["locals"] = helper.FlagDescriptor{
		Description: "Sets comma separated accounts whose transactions are treated as locals",
		Arguments: []string{
			"LOCALS",
		},
		FlagOptional: true,
	}

	c.flagMap["nolocals"] = helper.FlagDescriptor{
		Description: "Sets flag to disable price exemptions for locally submitted transactions",
		Arguments: []string{
			"NOLOCALS",
		},
		FlagOptional: true,
	}

	c.flagMap["price-limit"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets minimum gas price limit to enforce for acceptance into the pool. Default: %d", helper.DefaultConfig().TxPool.PriceLimit),
		Arguments: []string{
			"PRICE_LIMIT",
		},
		FlagOptional: true,
	}

	c.flagMap["max-slots"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets maximum slots in the pool. Default: %d", helper.DefaultConfig().TxPool.MaxSlots),
		Arguments: []string{
			"MAX_SLOTS",
		},
		FlagOptional: true,
	}

	c.flagMap["dev"] = helper.FlagDescriptor{
		Description: "Sets the client to dev mode. Default: false",
		Arguments: []string{
			"DEV_MODE",
		},
		FlagOptional: true,
	}

	c.flagMap["dev-interval"] = helper.FlagDescriptor{
		Description: "Sets the client's dev notification interval. Default: 0",
		Arguments: []string{
			"DEV_INTERVAL",
		},
		FlagOptional: true,
	}
}

// GetHelperText returns a simple description of the command
func (c *ServerCommand) GetHelperText() string {
	return "The default command that starts the Polygon-SDK client, by bootstrapping all modules together"
}

func (c *ServerCommand) GetBaseCommand() string {
	return "server"
}

// Help implements the cli.Command interface
func (c *ServerCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.flagMap), c.flagMap)
}

// Synopsis implements the cli.Command interface
func (c *ServerCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *ServerCommand) Run(args []string) int {
	conf, err := helper.ReadConfig(c.GetBaseCommand(), args)
	if err != nil {
		c.UI.Error(err.Error())

		return 1
	}

	config, err := conf.BuildConfig()
	if err != nil {
		c.UI.Error(err.Error())

		return 1
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "polygon",
		Level: hclog.LevelFromString(conf.LogLevel),
	})

	server, err := server.NewServer(logger, config)
	if err != nil {
		c.UI.Error(err.Error())

		return 1
	}

	if conf.Join != "" {
		// make a non-blocking join request
		if err = server.Join(conf.Join, 0); err != nil {
			c.UI.Error(fmt.Sprintf("Failed to join address %s: %v", conf.Join, err))
		}
	}

	return helper.HandleSignals(server.Close, c.UI)
}
