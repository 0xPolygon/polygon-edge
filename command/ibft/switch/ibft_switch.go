package ibftswitch

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
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

	cmd.Flags().StringVar(
		&params.minValidatorCountRaw,
		minValidatorCount,
		"",
		"the minimum number of validators in the validator set for PoS",
	)

	cmd.Flags().StringVar(
		&params.maxValidatorCountRaw,
		maxValidatorCount,
		"",
		"the maximum number of validators in the validator set for PoS",
	)
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
