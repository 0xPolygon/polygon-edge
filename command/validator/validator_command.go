package validator

import (
	"github.com/0xPolygon/polygon-edge/command/validator/registration"
	staking "github.com/0xPolygon/polygon-edge/command/validator/stake"
	unstaking "github.com/0xPolygon/polygon-edge/command/validator/unstake"
	"github.com/0xPolygon/polygon-edge/command/validator/validators"
	"github.com/0xPolygon/polygon-edge/command/validator/whitelist"
	"github.com/0xPolygon/polygon-edge/command/validator/withdraw"
	withdrawRewards "github.com/0xPolygon/polygon-edge/command/validator/withdraw-rewards"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	polybftCmd := &cobra.Command{
		Use:   "validator",
		Short: "Validator command",
	}

	polybftCmd.AddCommand(
		// sidechain (validator set) command to unstake on child chain
		unstaking.GetCommand(),
		// sidechain (validator set) command to withdraw stake on child chain
		withdraw.GetCommand(),
		// sidechain (reward pool) command to withdraw pending rewards
		withdrawRewards.GetCommand(),
		// rootchain (supernet manager) command that queries validator info
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
