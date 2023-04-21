package rootchain

import (
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command/rootchain/deploy"
	"github.com/0xPolygon/polygon-edge/command/rootchain/fund"
	"github.com/0xPolygon/polygon-edge/command/rootchain/registration"
	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
	"github.com/0xPolygon/polygon-edge/command/rootchain/whitelist"
)

// GetCommand creates "rootchain" helper command
func GetCommand() *cobra.Command {
	rootchainCmd := &cobra.Command{
		Use:   "rootchain",
		Short: "Top level rootchain helper command.",
	}

	rootchainCmd.AddCommand(
		// rootchain server
		server.GetCommand(),
		// rootchain deploy
		deploy.GetCommand(),
		// rootchain fund
		fund.GetCommand(),
		// rootchain (supernet manager) whitelist validator
		whitelist.GetCommand(),
		// rootchain (supernet manager) register validator
		registration.GetCommand(),
	)

	return rootchainCmd
}
