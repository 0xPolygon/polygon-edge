package txpool

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/txpool/add"
	"github.com/0xPolygon/polygon-edge/command/txpool/status"
	"github.com/0xPolygon/polygon-edge/command/txpool/subscribe"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	txPoolCmd := &cobra.Command{
		Use:   "txpool",
		Short: "Top level command for interacting with the transaction pool. Only accepts subcommands.",
	}

	// Register the base GRPC address flag
	txPoolCmd.PersistentFlags().String(
		helper.GRPCAddressFlag,
		fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort),
		helper.GRPCAddressFlag,
	)

	registerSubcommands(txPoolCmd)

	return txPoolCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	// txpool add
	baseCmd.AddCommand(add.GetCommand())

	// txpool status
	baseCmd.AddCommand(status.GetCommand())

	// txpool subscribe
	baseCmd.AddCommand(subscribe.GetCommand())
}
