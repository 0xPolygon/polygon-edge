package server

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xPolygon/minimal/minimal"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
)

// ServerFlagDescriptor contains the Description elements for a command flag. Duplicate of FlagDescriptor because of cyclic imports. Implements types.FlagDescriptor
type ServerFlagDescriptor struct {
	description       string   // Flag description
	arguments         []string // Arguments list
	argumentsOptional bool     // Flag indicating if flag arguments are optional
	flagOptional      bool
}

func (s ServerFlagDescriptor) GetDescription() string {
	return s.description
}

func (s ServerFlagDescriptor) GetArgumentsList() []string {
	return s.arguments
}

func (s ServerFlagDescriptor) GetArgumentsOptional() bool {
	return s.argumentsOptional
}

func (s ServerFlagDescriptor) GetFlagOptional() bool {
	return s.flagOptional
}

// Command is the command to start the sever
type Command struct {
	UI cli.Ui

	flagMap map[string]types.FlagDescriptor
}

// DefineFlags defines the command flags
func (c *Command) DefineFlags() {
	if c.flagMap == nil {
		// Flag map not initialized
		c.flagMap = make(map[string]types.FlagDescriptor)
	}

	if len(c.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.flagMap["log-level"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Sets the log level for console output. Default: %s", defaultConfig().LogLevel),
		arguments: []string{
			"LOG_LEVEL",
		},
		flagOptional: true,
	}

	c.flagMap["seal"] = ServerFlagDescriptor{
		description: "Sets the flag indicating that the client should seal blocks. Default false",
		arguments: []string{
			"SHOULD_SEAL",
		},
		flagOptional: true,
	}

	c.flagMap["config"] = ServerFlagDescriptor{
		description: "Specifies the path to the CLI config. Supports .json and .hcl",
		arguments: []string{
			"CLI_CONFIG_PATH",
		},
		flagOptional: true,
	}

	c.flagMap["chain"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Specifies the genesis file used for starting the chain. Default %s", defaultConfig().Chain),
		arguments: []string{
			"GENESIS_FILE",
		},
		flagOptional: true,
	}

	c.flagMap["data-dir"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Specifies the data directory used for storing Polygon SDK client data. Default %s", defaultConfig().DataDir),
		arguments: []string{
			"DATA_DIRECTORY",
		},
		flagOptional: true,
	}

	c.flagMap["grpc"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Sets the address and port for the gRPC service (address:port). Default address: 127.0.0.1:%d", minimal.DefaultGRPCPort),
		arguments: []string{
			"GRPC_ADDRESS",
		},
		flagOptional: true,
	}

	c.flagMap["jsonrpc"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Sets the address and port for the JSON-RPC service (address:port). Default address: 127.0.0.1:%d", minimal.DefaultJSONRPCPort),
		arguments: []string{
			"JSONRPC_ADDRESS",
		},
		flagOptional: true,
	}

	c.flagMap["libp2p"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Sets the address and port for the libp2p service (address:port). Default address: 127.0.0.1:%d", network.DefaultLibp2pPort),
		arguments: []string{
			"LIBP2P_ADDRESS",
		},
		flagOptional: true,
	}

	c.flagMap["join"] = ServerFlagDescriptor{
		description: "Specifies the address of the peer that should be joined",
		arguments: []string{
			"JOIN_ADDRESS",
		},
		flagOptional: true,
	}

	c.flagMap["nat"] = ServerFlagDescriptor{
		description: "Sets the the external IP address without the port, as it can be seen by peers",
		arguments: []string{
			"NAT_ADDRESS",
		},
		flagOptional: true,
	}

	c.flagMap["no-discover"] = ServerFlagDescriptor{
		description: "Sets the client's discoverability for other peers. Default false",
		arguments: []string{
			"NO_DISCOVER",
		},
		flagOptional: true,
	}

	c.flagMap["max-peers"] = ServerFlagDescriptor{
		description: fmt.Sprintf("Sets the client's max peer count. Default %d", defaultConfig().Network.MaxPeers),
		arguments: []string{
			"PEER_COUNT",
		},
		flagOptional: true,
	}

	c.flagMap["dev"] = ServerFlagDescriptor{
		description: "Sets the client to dev mode. Default false",
		arguments: []string{
			"DEV_MODE",
		},
		flagOptional: true,
	}

	c.flagMap["dev-interval"] = ServerFlagDescriptor{
		description: "Sets the client's dev notification interval. Default 0",
		arguments: []string{
			"DEV_INTERVAL",
		},
		flagOptional: true,
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

	return types.GenerateHelp(c.Synopsis(), types.GenerateUsage(c.GetBaseCommand(), c.flagMap), c.flagMap)
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
