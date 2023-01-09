package validators

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
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
		sidechainHelper.AccountDirFlag,
		"",
		"the directory path where validator key is stored",
	)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	validatorAccount, err := sidechainHelper.GetAccountFromDir(params.accountDir)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return err
	}

	validatorAddr := validatorAccount.Ecdsa.Address()

	responseMap, err := sidechainHelper.GetValidatorInfo(validatorAddr, txRelayer)
	if err != nil {
		return fmt.Errorf("failed to get validator info for %s: %w", validatorAddr, err)
	}

	outputter.WriteCommandResult(&validatorsInfoResult{
		address:             validatorAccount.Ecdsa.Address().String(),
		stake:               responseMap["stake"].(*big.Int).Uint64(),               //nolint:forcetypeassert
		totalStake:          responseMap["totalStake"].(*big.Int).Uint64(),          //nolint:forcetypeassert
		commission:          responseMap["commission"].(*big.Int).Uint64(),          //nolint:forcetypeassert
		withdrawableRewards: responseMap["withdrawableRewards"].(*big.Int).Uint64(), //nolint:forcetypeassert
		active:              responseMap["active"].(bool),                           //nolint:forcetypeassert
	})

	return nil
}
