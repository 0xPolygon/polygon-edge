package ibft

import (
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/ibft/candidates"
	"github.com/0xPolygon/polygon-edge/command/ibft/propose"
	"github.com/0xPolygon/polygon-edge/command/ibft/snapshot"
	"github.com/0xPolygon/polygon-edge/command/ibft/status"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	ibftCmd := &cobra.Command{
		Use:   "ibft",
		Short: "Top level IBFT command for interacting with the IBFT consensus. Only accepts subcommands.",
	}

	helper.RegisterGRPCAddressFlag(ibftCmd)

	registerSubcommands(ibftCmd)

	return ibftCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	// ibft status
	baseCmd.AddCommand(status.GetCommand())

	// ibft snapshot
	baseCmd.AddCommand(snapshot.GetCommand())

	// ibft propose
	baseCmd.AddCommand(propose.GetCommand())

	// ibft candidates
	baseCmd.AddCommand(candidates.GetCommand())
}
