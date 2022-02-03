package root

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
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

	// Register the --json output setting for all child commands
	rootCommand.baseCmd.PersistentFlags().Bool(
		helper.JSONOutputFlag,
		false,
		"Specifies if the output should be in JSON",
	)

	// Register all the commands
	rootCommand.registerCommands()

	return rootCommand
}

func (rc *RootCommand) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}
