package polybft

import (
	"github.com/0xPolygon/polygon-edge/command/sidechain/registration"
	"github.com/0xPolygon/polygon-edge/command/sidechain/staking"
	"github.com/0xPolygon/polygon-edge/command/sidechain/unstaking"
	"github.com/0xPolygon/polygon-edge/command/sidechain/validators"

	"github.com/0xPolygon/polygon-edge/command/sidechain/whitelist"
	"github.com/0xPolygon/polygon-edge/command/sidechain/withdraw"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	polybftCmd := &cobra.Command{
		Use:   "polybft",
		Short: "Polybft command",
	}

	polybftCmd.AddCommand(
		staking.GetCommand(),
		unstaking.GetCommand(),
		withdraw.GetCommand(),
		validators.GetCommand(),
		whitelist.GetCommand(),
		registration.GetCommand(),
	)

	return polybftCmd
}
