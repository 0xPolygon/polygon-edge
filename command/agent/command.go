package agent

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mitchellh/cli"
)

type configFlag []string

func (c *configFlag) String() string {
	return "configuration flags"
}

func (c *configFlag) Set(value string) error {
	*c = append(*c, value)
	return nil
}

type AgentCommand struct {
	Ui cli.Ui
}

func (a *AgentCommand) Help() string {
	return ""
}

func (a *AgentCommand) Synopsis() string {
	return ""
}

func readConfig(args []string) (*Config, error) {
	config := DefaultConfig()

	cliConfig := &Config{Telemetry: &Telemetry{}}

	flags := flag.NewFlagSet("agent", flag.ContinueOnError)
	flags.Usage = func() {}

	flags.IntVar(&cliConfig.BindPort, "port", 0, "")
	flags.StringVar(&cliConfig.BindAddr, "addr", "", "")
	flags.StringVar(&cliConfig.DataDir, "data-dir", "", "")
	flags.StringVar(&cliConfig.ServiceName, "service", "", "")
	flags.IntVar(&cliConfig.Telemetry.PrometheusPort, "prometheus", 0, "")
	flags.BoolVar(&cliConfig.Seal, "seal", false, "")

	var configFilePaths configFlag
	flags.Var(&configFilePaths, "config", "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	args = flags.Args()

	chain := "foundation"
	if len(args) == 1 {
		chain = args[0]
	} else if len(args) > 1 {
		return nil, fmt.Errorf("too many arguments, only expected one")
	}
	cliConfig.Chain = chain

	// config file
	if len(configFilePaths) != 0 {
		for _, path := range configFilePaths {
			configFile, err := readConfigFile(path)
			if err != nil {
				return nil, err
			}
			if err := config.Merge(configFile); err != nil {
				return nil, err
			}
		}
	}

	err := config.Merge(cliConfig)
	return config, err
}

func (a *AgentCommand) Run(args []string) int {
	config, err := readConfig(args)
	if err != nil {
		a.Ui.Error(fmt.Sprintf("Failed to read config: %v", err))
		return 1
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)

	agent := NewAgent(logger, config)
	if err := agent.Start(); err != nil {
		a.Ui.Error(fmt.Sprintf("Failed to start agent: %v", err))
		return 1
	}

	return handleSignals(agent)
}

func handleSignals(a *Agent) int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	fmt.Printf("Caught signal: %v\n", sig)
	fmt.Printf("Gracefully shutting down agent...\n")

	gracefulCh := make(chan struct{})
	go func() {
		a.Close()
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
