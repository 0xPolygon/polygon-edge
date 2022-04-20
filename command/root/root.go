package root

import (
	"fmt"
	"os"

	"github.com/dogechain-lab/jury/command/backup"
	"github.com/dogechain-lab/jury/command/genesis"
	"github.com/dogechain-lab/jury/command/helper"
	"github.com/dogechain-lab/jury/command/ibft"
	"github.com/dogechain-lab/jury/command/license"
	"github.com/dogechain-lab/jury/command/loadbot"
	"github.com/dogechain-lab/jury/command/monitor"
	"github.com/dogechain-lab/jury/command/peers"
	"github.com/dogechain-lab/jury/command/secrets"
	"github.com/dogechain-lab/jury/command/server"
	"github.com/dogechain-lab/jury/command/status"
	"github.com/dogechain-lab/jury/command/txpool"
	"github.com/dogechain-lab/jury/command/version"
	"github.com/spf13/cobra"
)

type RootCommand struct {
	baseCmd *cobra.Command
}

func NewRootCommand() *RootCommand {
	rootCommand := &RootCommand{
		baseCmd: &cobra.Command{
			Short: "DogeChain-Lab Jury is a framework for building Ethereum-compatible Blockchain networks",
		},
	}

	helper.RegisterJSONOutputFlag(rootCommand.baseCmd)

	rootCommand.registerSubCommands()

	return rootCommand
}

func (rc *RootCommand) registerSubCommands() {
	rc.baseCmd.AddCommand(
		version.GetCommand(),
		txpool.GetCommand(),
		status.GetCommand(),
		secrets.GetCommand(),
		peers.GetCommand(),
		monitor.GetCommand(),
		loadbot.GetCommand(),
		ibft.GetCommand(),
		backup.GetCommand(),
		genesis.GetCommand(),
		server.GetCommand(),
		license.GetCommand(),
	)
}

func (rc *RootCommand) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}
