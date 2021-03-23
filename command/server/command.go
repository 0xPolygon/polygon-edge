package server

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xPolygon/minimal/minimal"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
)

// Command is the command to start the sever
type Command struct {
	UI cli.Ui
}

// Help implements the cli.Command interface
func (c *Command) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *Command) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (c *Command) Run(args []string) int {
	conf, err := readConfig(args)
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
		server.Join(conf.Join)
	}
	return c.handleSignals(server.Close)
}

func (c *Command) handleSignals(closeFn func()) int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	c.UI.Output(fmt.Sprintf("Caught signal: %v", sig))
	c.UI.Output("Gracefully shutting down agent...")

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

func readConfig(args []string) (*Config, error) {
	config := defaultConfig()

	cliConfig := &Config{
		Telemetry: &Telemetry{},
	}

	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.Usage = func() {}

	var configFile string
	flags.StringVar(&cliConfig.LogLevel, "log-level", "", "")
	flags.BoolVar(&cliConfig.Seal, "seal", false, "")
	flags.StringVar(&configFile, "config", "", "")
	flags.StringVar(&cliConfig.DataDir, "data-dir", "", "")
	flags.StringVar(&cliConfig.GRPCAddr, "grpc", "", "")
	flags.StringVar(&cliConfig.LibP2PAddr, "libp2p", "", "")
	flags.StringVar(&cliConfig.JSONRPCAddr, "jsonrpc", "", "")
	flags.StringVar(&cliConfig.Join, "join", "", "")
	flags.StringVar(&cliConfig.Chain, "chain", "", "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if configFile != "" {
		conf2, err := readConfigFile(configFile)
		if err != nil {
			return nil, err
		}
		if err := config.merge(conf2); err != nil {
			return nil, err
		}
	}

	if err := config.merge(cliConfig); err != nil {
		return nil, err
	}
	return config, nil
}
