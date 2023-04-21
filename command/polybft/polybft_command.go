package polybft

import (
	"github.com/0xPolygon/polygon-edge/command/rootchain/registration"
	"github.com/0xPolygon/polygon-edge/command/rootchain/staking"
	"github.com/0xPolygon/polygon-edge/command/rootchain/whitelist"
	"github.com/0xPolygon/polygon-edge/command/sidechain/unstaking"
	"github.com/0xPolygon/polygon-edge/command/sidechain/validators"

	"github.com/0xPolygon/polygon-edge/command/sidechain/withdraw"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	polybftCmd := &cobra.Command{
		Use:   "polybft",
		Short: "Polybft command",
	}

	polybftCmd.AddCommand(
		unstaking.GetCommand(),
		withdraw.GetCommand(),
		validators.GetCommand(),
		// rootchain (supernet manager) whitelist validator
		whitelist.GetCommand(),
		// rootchain (supernet manager) register validator
		registration.GetCommand(),
		// rootchain (stake manager) stake command
		staking.GetCommand(),
	)

	return polybftCmd
}
