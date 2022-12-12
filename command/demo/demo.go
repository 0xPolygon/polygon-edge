package demo

import (
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

// GetCommand creates "rootchain" helper command
func GetCommand() *cobra.Command {
	rootchainCmd := &cobra.Command{
		Use:   "demo",
		Short: "Top level RootChain helper command.",
	}

	helper.RegisterGRPCAddressFlag(rootchainCmd)

	registerSubcommands(rootchainCmd)

	return rootchainCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	baseCmd.AddCommand(
		// rootchain emit
		StakeCommand(),
		UnstakeCommand(),
	)
}
