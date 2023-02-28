package validators

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/spf13/cobra"
)

var (
	params validatorInfoParams
)

func GetCommand() *cobra.Command {
	validatorInfoCmd := &cobra.Command{
		Use:     "validator-info",
		Short:   "Gets validator info",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	helper.RegisterJSONRPCFlag(validatorInfoCmd)
	setFlags(validatorInfoCmd)

	return validatorInfoCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.accountDir,
		polybftsecrets.DataPathFlag,
		"",
		polybftsecrets.DataPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.ConfigFlag,
		"",
		polybftsecrets.ConfigFlagDesc,
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.DataPathFlag, polybftsecrets.ConfigFlag)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	validatorAccount, err := sidechainHelper.GetAccount(params.accountDir, params.configPath)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return err
	}

	validatorAddr := validatorAccount.Ecdsa.Address()

	validatorInfo, err := sidechainHelper.GetValidatorInfo(validatorAddr, txRelayer)
	if err != nil {
		return fmt.Errorf("failed to get validator info for %s: %w", validatorAddr, err)
	}

	outputter.WriteCommandResult(&validatorsInfoResult{
		address:             validatorInfo.Address.String(),
		stake:               validatorInfo.Stake.Uint64(),
		totalStake:          validatorInfo.TotalStake.Uint64(),
		commission:          validatorInfo.Commission.Uint64(),
		withdrawableRewards: validatorInfo.WithdrawableRewards.Uint64(),
		active:              validatorInfo.Active,
	})

	return nil
}
