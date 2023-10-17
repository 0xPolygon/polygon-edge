package ibftswitch

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/validators"
)

func GetCommand() *cobra.Command {
	ibftSwitchCmd := &cobra.Command{
		Use:     "switch",
		Short:   "Add settings in genesis.json to switch IBFT type",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(ibftSwitchCmd)
	helper.SetRequiredFlags(ibftSwitchCmd, params.getRequiredFlags())

	return ibftSwitchCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		chainFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the genesis file to update",
	)

	cmd.Flags().StringVar(
		&params.typeRaw,
		typeFlag,
		"",
		"the new IBFT type [PoA, PoS]",
	)

	{
		// switch block height
		cmd.Flags().StringVar(
			&params.deploymentRaw,
			deploymentFlag,
			"",
			"the height to deploy the contract in PoS",
		)

		cmd.Flags().StringVar(
			&params.fromRaw,
			fromFlag,
			"",
			"the height to switch the new type",
		)
	}

	// IBFT
	{
		cmd.Flags().StringVar(
			&params.rawIBFTValidatorType,
			command.IBFTValidatorTypeFlag,
			string(validators.BLSValidatorType),
			"the type of validators in IBFT",
		)
	}

	{
		// PoS Configuration
		cmd.Flags().StringVar(
			&params.minValidatorCountRaw,
			command.MinValidatorCountFlag,
			"",
			"the minimum number of validators in the validator set for PoS",
		)

		cmd.Flags().StringVar(
			&params.maxValidatorCountRaw,
			command.MaxValidatorCountFlag,
			"",
			"the maximum number of validators in the validator set for PoS",
		)

		cmd.Flags().StringVar(
			&params.validatorRootPath,
			command.ValidatorRootFlag,
			command.DefaultValidatorRoot,
			"root path for validator folder directory. "+
				"Needs to be present if validators is omitted",
		)

		cmd.Flags().StringVar(
			&params.validatorPrefixPath,
			command.ValidatorPrefixFlag,
			command.DefaultValidatorPrefix,
			"prefix path for validator folder directory. "+
				"Needs to be present if validators is omitted",
		)

		cmd.Flags().StringArrayVar(
			&params.validatorsRaw,
			command.ValidatorFlag,
			[]string{},
			"addresses to be used as IBFT validators, can be used multiple times. "+
				"Needs to be present if validators-prefix is omitted",
		)

		cmd.MarkFlagsMutuallyExclusive(command.ValidatorPrefixFlag, command.ValidatorFlag)
		cmd.MarkFlagsMutuallyExclusive(command.ValidatorRootFlag, command.ValidatorFlag)
	}
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.initRawParams()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.updateGenesisConfig(); err != nil {
		outputter.SetError(err)

		return
	}

	if err := params.overrideGenesisConfig(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
