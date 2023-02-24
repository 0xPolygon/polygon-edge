package rootchain

import (
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/deposit"
	"github.com/0xPolygon/polygon-edge/command/rootchain/fund"
	"github.com/0xPolygon/polygon-edge/command/rootchain/initcontracts"
	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
)

// GetCommand creates "rootchain" helper command
func GetCommand() *cobra.Command {
	rootchainCmd := &cobra.Command{
		Use:   "rootchain",
		Short: "Top level RootChain helper command.",
	}

	helper.RegisterGRPCAddressFlag(rootchainCmd)

	registerSubcommands(rootchainCmd)

	return rootchainCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	baseCmd.AddCommand(
		// rootchain deposit
		deposit.GetCommand(),
		// rootchain fund
		fund.GetCommand(),
		// rootchain server
		server.GetCommand(),
		// init-contracts
		initcontracts.GetCommand(),
	)
}
