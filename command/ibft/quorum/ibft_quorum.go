package quorum

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	ibftQuorumCmd := &cobra.Command{
		Use:     "quorum",
		Short:   "Specify the block number after which quorum optimal will be used for reaching consensus",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(ibftQuorumCmd)
	helper.SetRequiredFlags(ibftQuorumCmd, params.getRequiredFlags())

	return ibftQuorumCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		chainFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the genesis file to update",
	)

	cmd.Flags().Uint64Var(
		&params.from,
		fromFlag,
		0,
		"the height to switch the quorum calculation",
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
