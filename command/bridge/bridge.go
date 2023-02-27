package bridge

import (
	"github.com/0xPolygon/polygon-edge/command/bridge/deposit"
	"github.com/spf13/cobra"
)

// GetCommand creates "bridge" helper command
func GetCommand() *cobra.Command {
	rootchainCmd := &cobra.Command{
		Use:   "bridge",
		Short: "Top level bridge command.",
	}

	registerSubcommands(rootchainCmd)

	return rootchainCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	baseCmd.AddCommand(
		// bridge deposit
		deposit.GetCommand(),
	)
}
