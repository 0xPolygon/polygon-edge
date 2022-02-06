package root

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/monitor"
	"github.com/0xPolygon/polygon-edge/command/peers"
	"github.com/0xPolygon/polygon-edge/command/secrets"
	"github.com/0xPolygon/polygon-edge/command/status"
	"github.com/0xPolygon/polygon-edge/command/txpool"
	"github.com/0xPolygon/polygon-edge/command/version"
	"github.com/spf13/cobra"
	"os"
)

type RootCommand struct {
	baseCmd *cobra.Command
}

func NewRootCommand() *RootCommand {
	rootCommand := &RootCommand{
		baseCmd: &cobra.Command{
			Short: "Polygon Edge is a framework for building Ethereum-compatible Blockchain networks",
		},
	}

	helper.RegisterJSONOutputFlag(rootCommand.baseCmd)

	// Register all the commands
	rootCommand.registerSubCommands()

	return rootCommand
}

func (rc *RootCommand) registerSubCommands() {
	rc.baseCmd.AddCommand(version.GetCommand())
	rc.baseCmd.AddCommand(txpool.GetCommand())
	rc.baseCmd.AddCommand(status.GetCommand())
	rc.baseCmd.AddCommand(secrets.GetCommand())
	rc.baseCmd.AddCommand(peers.GetCommand())
	rc.baseCmd.AddCommand(monitor.GetCommand())
}

func (rc *RootCommand) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}
