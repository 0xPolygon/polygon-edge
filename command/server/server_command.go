package server

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/hashicorp/go-hclog"
)

// ServerCommand is the command to start the sever
type ServerCommand struct {
	helper.Base
}

// DefineFlags defines the command flags
func (c *ServerCommand) DefineFlags() {
	c.Base.DefineFlags()

	if len(c.FlagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.FlagMap["log-level"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the log level for console output. Default: %s", helper.DefaultConfig().LogLevel),
		Arguments: []string{
			"LOG_LEVEL",
		},
		FlagOptional: true,
	}

	c.FlagMap["seal"] = helper.FlagDescriptor{
		Description: "Sets the flag indicating that the client should seal blocks. Default: false",
		Arguments: []string{
			"SHOULD_SEAL",
		},
		FlagOptional: true,
	}

	c.FlagMap["block-gas-target"] = helper.FlagDescriptor{
		Description: "Sets the target block gas limit for the chain. If omitted, the value of the parent block is used",
		Arguments: []string{
			"BLOCK_GAS_TARGET",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["config"] = helper.FlagDescriptor{
		Description: "Specifies the path to the CLI config. Supports .json and .hcl",
		Arguments: []string{
			"CLI_CONFIG_PATH",
		},
		FlagOptional: true,
	}

	c.FlagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Specifies the genesis file used for starting the chain. Default: %s",
			helper.DefaultConfig().Chain,
		),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	c.FlagMap["data-dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Specifies the data directory used for storing Polygon Edge client data. Default: %s",
			helper.DefaultConfig().DataDir,
		),
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		FlagOptional: true,
	}

	c.FlagMap["grpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Sets the address and port for the gRPC service (address:port). Default: address: 127.0.0.1:%d",
			server.DefaultGRPCPort,
		),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["jsonrpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Sets the address and port for the JSON-RPC service (address:port). Default: address: 127.0.0.1:%d",
			server.DefaultJSONRPCPort,
		),
		Arguments: []string{
			"JSONRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["libp2p"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Sets the address and port for the libp2p service (address:port). Default: address: 127.0.0.1:%d",
			network.DefaultLibp2pPort,
		),
		Arguments: []string{
			"LIBP2P_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["join"] = helper.FlagDescriptor{
		Description: "Specifies the address of the peer that should be joined",
		Arguments: []string{
			"JOIN_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["nat"] = helper.FlagDescriptor{
		Description: "Sets the external IP address without the port, as it can be seen by peers",
		Arguments: []string{
			"NAT_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["dns"] = helper.FlagDescriptor{
		Description: "Sets the host DNS address",
		Arguments: []string{
			"DNS_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["no-discover"] = helper.FlagDescriptor{
		Description: "Prevents the client from discovering other peers. Default: false",
		Arguments: []string{
			"NO_DISCOVER",
		},
		FlagOptional: true,
	}

	c.FlagMap["max-peers"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the client's max no.of peers allowded. Default: %d",
			helper.DefaultConfig().Network.MaxPeers),
		Arguments: []string{
			"PEER_COUNT",
		},
		FlagOptional: true,
	}

	c.FlagMap["max-inbound-peers"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the client's max no.of inbound peers allowded. Default: %d",
			helper.DefaultConfig().Network.MaxInboundPeers),
		Arguments: []string{
			"PEER_COUNT",
		},
		FlagOptional: true,
	}

	c.FlagMap["max-outbound-peers"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the client's max no.of outbound peers allowded. Default: %d",
			helper.DefaultConfig().Network.MaxOutboundPeers),
		Arguments: []string{
			"PEER_COUNT",
		},
		FlagOptional: true,
	}

	c.FlagMap["price-limit"] = helper.FlagDescriptor{
		Description: fmt.Sprintf(
			"Sets minimum gas price limit to enforce for acceptance into the pool. Default: %d",
			helper.DefaultConfig().TxPool.PriceLimit,
		),
		Arguments: []string{
			"PRICE_LIMIT",
		},
		FlagOptional: true,
	}

	c.FlagMap["max-slots"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets maximum slots in the pool. Default: %d", helper.DefaultConfig().TxPool.MaxSlots),
		Arguments: []string{
			"MAX_SLOTS",
		},
		FlagOptional: true,
	}

	c.FlagMap["dev"] = helper.FlagDescriptor{
		Description: "Sets the client to dev mode. Default: false",
		Arguments: []string{
			"DEV_MODE",
		},
		FlagOptional: true,
	}

	c.FlagMap["dev-interval"] = helper.FlagDescriptor{
		Description: "Sets the client's dev notification interval. Default: 0",
		Arguments: []string{
			"DEV_INTERVAL",
		},
		FlagOptional: true,
	}

	c.FlagMap["prometheus"] = helper.FlagDescriptor{
		Description: "Sets the address and port for the prometheus instrumentation service (address:port)",
		Arguments: []string{
			"PROMETHEUS_ADDRESS",
		},
		FlagOptional: true,
	}

	c.FlagMap["secrets-config"] = helper.FlagDescriptor{
		Description: "Sets the path to the SecretsManager config file. Used for Hashicorp Vault. " +
			"If omitted, the local FS secrets manager is used",
		Arguments: []string{
			"SECRETS_CONFIG",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["access-control-allow-origins"] = helper.FlagDescriptor{
		Description: "Sets the header indicating whether any JSON-RPC response can be shared with requesting code" +
			" from the given origin",
		Arguments: []string{
			"ACCESS_CONTROL_ALLOW_ORIGINS",
		},
		FlagOptional: true,
	}

	c.FlagMap["restore"] = helper.FlagDescriptor{
		Description: "Sets the path to the archive blockchain data to restore on initialization",
		Arguments: []string{
			"RESTORE",
		},
		FlagOptional: true,
	}

	c.FlagMap["block-time"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets block time in seconds. Default: %ds", helper.DefaultConfig().BlockTime),
		Arguments: []string{
			"BLOCK_TIME",
		},
		FlagOptional: true,
	}
}

// GetHelperText returns a simple description of the command
func (c *ServerCommand) GetHelperText() string {
	return "The default command that starts the Polygon Edge client, by bootstrapping all modules together"
}

func (c *ServerCommand) GetBaseCommand() string {
	return "server"
}

// Help implements the cli.Command interface
func (c *ServerCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
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
