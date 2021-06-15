package server

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/0xPolygon/minimal/network"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
)

// Command is the command to start the sever
type Command struct {
	UI cli.Ui

	flagMap map[string]helper.FlagDescriptor
}

// DefineFlags defines the command flags
func (c *Command) DefineFlags() {
	if c.flagMap == nil {
		// Flag map not initialized
		c.flagMap = make(map[string]helper.FlagDescriptor)
	}

	if len(c.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.flagMap["log-level"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the log level for console output. Default: %s", defaultConfig().LogLevel),
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

	c.flagMap["config"] = helper.FlagDescriptor{
		Description: "Specifies the path to the CLI config. Supports .json and .hcl",
		Arguments: []string{
			"CLI_CONFIG_PATH",
		},
		FlagOptional: true,
	}

	c.flagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the genesis file used for starting the chain. Default: %s", defaultConfig().Chain),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	c.flagMap["data-dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the data directory used for storing Polygon SDK client data. Default: %s", defaultConfig().DataDir),
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		FlagOptional: true,
	}

	c.flagMap["grpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the gRPC service (address:port). Default: address: 127.0.0.1:%d", minimal.DefaultGRPCPort),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		FlagOptional: true,
	}

	c.flagMap["jsonrpc"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the address and port for the JSON-RPC service (address:port). Default: address: 127.0.0.1:%d", minimal.DefaultJSONRPCPort),
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
		Description: "Sets the the external IP address without the port, as it can be seen by peers",
		Arguments: []string{
			"NAT_ADDRESS",
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
		Description: fmt.Sprintf("Sets the client's max peer count. Default: %d", defaultConfig().Network.MaxPeers),
		Arguments: []string{
			"PEER_COUNT",
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
func (c *Command) GetHelperText() string {
	return "The default command that starts the Polygon-SDK client, by bootstrapping all modules together"
}

func (c *Command) GetBaseCommand() string {
	return "server"
}

// Help implements the cli.Command interface
func (c *Command) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.flagMap), c.flagMap)
}

// Synopsis implements the cli.Command interface
func (c *Command) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *Command) Run(args []string) int {
	conf, err := readConfig(c.GetBaseCommand(), args)
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
		Level: hclog.LevelFromString("debug"),
	})

	server, err := minimal.NewServer(logger, config)
	if err != nil {
		c.UI.Error(err.Error())

		return 1
	}

	if conf.Join != "" {
		// make a non-blocking join request
		server.Join(conf.Join, 0)
	}

	return c.handleSignals(server.Close)
}

// handleSignals listens for any client related signals, and closes the client accordingly
func (c *Command) handleSignals(closeFn func()) int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	output := fmt.Sprintf("\n[SIGNAL] Caught signal: %v\n", sig)
	output += "Gracefully shutting down client...\n"

	c.UI.Output(output)

	// Call the Minimal server close callback
	gracefulCh := make(chan struct{})
	go func() {
		if closeFn != nil {
			closeFn()
		}
		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return 1
	case <-time.After(5 * time.Second):
		return 1
	case <-gracefulCh:
		return 0
	}
}

func readConfig(baseCommand string, args []string) (*Config, error) {
	config := defaultConfig()

	cliConfig := &Config{
		Network: &Network{},
	}

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var configFile string
	flags.StringVar(&cliConfig.LogLevel, "log-level", "", "")
	flags.BoolVar(&cliConfig.Seal, "seal", false, "")
	flags.StringVar(&configFile, "config", "", "")
	flags.StringVar(&cliConfig.Chain, "chain", "", "")
	flags.StringVar(&cliConfig.DataDir, "data-dir", "", "")
	flags.StringVar(&cliConfig.GRPCAddr, "grpc", "", "")
	flags.StringVar(&cliConfig.JSONRPCAddr, "jsonrpc", "", "")
	flags.StringVar(&cliConfig.Join, "join", "", "")
	flags.StringVar(&cliConfig.Network.Addr, "libp2p", "", "")
	flags.StringVar(&cliConfig.Network.NatAddr, "nat", "", "the external IP address without port, as can be seen by peers")
	flags.BoolVar(&cliConfig.Network.NoDiscover, "no-discover", false, "")
	flags.Uint64Var(&cliConfig.Network.MaxPeers, "max-peers", 0, "")
	flags.BoolVar(&cliConfig.Dev, "dev", false, "")
	flags.Uint64Var(&cliConfig.DevInterval, "dev-interval", 0, "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if configFile != "" {
		// A config file has been passed in, parse it
		diskConfigFile, err := readConfigFile(configFile)
		if err != nil {
			return nil, err
		}

		if err := config.mergeConfigWith(diskConfigFile); err != nil {
			return nil, err
		}
	}

	if err := config.mergeConfigWith(cliConfig); err != nil {
		return nil, err
	}

	return config, nil
}
