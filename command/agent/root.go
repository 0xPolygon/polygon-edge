package agent

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/umbracle/minimal/command"
)

type configFlag []string

func (c *configFlag) String() string {
	return "configuration flags"
}

func (c *configFlag) Set(value string) error {
	*c = append(*c, value)
	return nil
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent...",
	Run:   agentRun,
	RunE:  agentRunE,
}

func init() {
	agentCmd.Flags().Int("port", 0, "Port ...")
	agentCmd.Flags().String("addr", "", "Addr ...")
	agentCmd.Flags().String("data-dir", "", "Data-dir ...")
	agentCmd.Flags().String("service", "", "Service ...")
	agentCmd.Flags().Int("prometheus", 0, "Prometheus ...")
	agentCmd.Flags().Bool("seal", false, "Seal ...")
	agentCmd.Flags().String("log-level", "", "Log-level ...")
	agentCmd.Flags().String("state-storage", "", "State-storage ...")
	agentCmd.Flags().StringSlice("config", nil, "Config ...")

	command.RegisterCmd(agentCmd)
}

func readConfig(cmd *cobra.Command, args []string) (*Config, error) {
	config := DefaultConfig()

	cliConfig := &Config{
		Telemetry: &Telemetry{},
	}

	var configFilePaths configFlag
	configFilePaths, err := cmd.Flags().GetStringSlice("config")
	if err == nil {
		chain := "foundation"
		if len(args) > 0 {
			chain = args[0]
		}

		cliConfig.Chain = chain
		cliConfig.BindAddr, _ = cmd.Flags().GetString("addr")
		cliConfig.DataDir, _ = cmd.Flags().GetString("data-dir")
		cliConfig.ServiceName, _ = cmd.Flags().GetString("service")
		cliConfig.Telemetry.PrometheusPort, _ = cmd.Flags().GetInt("prometheus")
		cliConfig.Seal, _ = cmd.Flags().GetBool("seal")
		cliConfig.LogLevel, _ = cmd.Flags().GetString("log-level")
		cliConfig.StateStorage, _ = cmd.Flags().GetString("state-storage")

		// config file
		if len(configFilePaths) != 0 {
			var configFile *Config
			for _, path := range configFilePaths {
				configFile, err = readConfigFile(path)
				if err != nil {
					break
				}
				if err = config.Merge(configFile); err != nil {
					break
				}
			}
		}
		if err == nil {
			err = config.Merge(cliConfig)
		}
	}

	// if err != nil {
	//	config = nil
	//}
	return config, err
}

func agentRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, agentRunE)
}

func agentRunE(cmd *cobra.Command, args []string) error {
	config, err := readConfig(cmd, args)
	if err != nil {
		return fmt.Errorf("Failed to read config: %v", err)
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)

	agent := NewAgent(logger, config)
	if err := agent.Start(); err != nil {
		return fmt.Errorf("Failed to start agent: %v", err)
	}

	return handleClose(agent)
}

func handleClose(a *Agent) error {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	fmt.Printf("Caught signal: %v\n", sig)
	fmt.Printf("Gracefully shutting down agent...\n")

	gracefulCh := make(chan struct{}, 1)
	go func() {
		a.Close()
		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return errors.New("Forced exit")
	case <-time.After(5 * time.Second):
		return errors.New("Timed out when exiting the agent")
	case <-gracefulCh:
	}
	//signal.Stop(signalCh)
	//signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	return nil
}
