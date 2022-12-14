package test

import (
	"github.com/0xPolygon/polygon-edge/command/e2e"
	"github.com/0xPolygon/polygon-edge/command/sidechain/staking"
	"github.com/0xPolygon/polygon-edge/command/sidechain/unstaking"
	"github.com/0xPolygon/polygon-edge/command/sidechain/validators"
	"github.com/0xPolygon/polygon-edge/command/sidechain/withdraw"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	testCmd.AddCommand(
		staking.GetCommand(),
		unstaking.GetCommand(),
		withdraw.GetCommand(),
		validators.GetCommand(),
		e2e.GetCommand(),
	)

	return testCmd
}
