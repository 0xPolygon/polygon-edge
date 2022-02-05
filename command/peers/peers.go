package peers

import (
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/peers/add"
	"github.com/0xPolygon/polygon-edge/command/peers/list"
	"github.com/0xPolygon/polygon-edge/command/peers/status"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	peersCmd := &cobra.Command{
		Use:   "peers",
		Short: "Top level command for interacting with the network peers. Only accepts subcommands.",
	}

	helper.RegisterGRPCAddressFlag(peersCmd)

	registerSubcommands(peersCmd)

	return peersCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	// peers status
	baseCmd.AddCommand(status.GetCommand())

	// peers list
	baseCmd.AddCommand(list.GetCommand())

	// peers add
	baseCmd.AddCommand(add.GetCommand())
}
