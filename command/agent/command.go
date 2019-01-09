package agent

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mitchellh/cli"
)

type AgentCommand struct {
	Ui cli.Ui
}

func (a *AgentCommand) Help() string {
	return ""
}

func (a *AgentCommand) Synopsis() string {
	return ""
}

func (a *AgentCommand) Run(args []string) int {

	// TODO, read config from flags and args
	config := DefaultConfig()

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
