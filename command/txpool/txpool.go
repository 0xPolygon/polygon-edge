package txpool

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/txpool/add"
	"github.com/0xPolygon/polygon-edge/command/txpool/status"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/spf13/cobra"
)

type TxPoolCommand struct {
	baseCmd *cobra.Command
}

func NewTxPoolCommand() *cobra.Command {
	txPoolCmd := &TxPoolCommand{
		baseCmd: &cobra.Command{
			Use:   "txpool",
			Short: "Top level command for interacting with the transaction pool. Only accepts subcommands.",
		},
	}

	// Register the base GRPC address flag
	txPoolCmd.baseCmd.PersistentFlags().String(
		helper.GRPCAddressFlag,
		fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort),
		helper.GRPCAddressFlag,
	)

	txPoolCmd.registerSubcommands()

	return txPoolCmd.baseCmd
}

func (t *TxPoolCommand) registerSubcommands() {
	// txpool add
	t.baseCmd.AddCommand(add.GetCommand())

	// txpool status
	t.baseCmd.AddCommand(status.GetCommand())
}
