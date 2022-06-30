package backup

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

func GetCommand() *cobra.Command {
	backupCmd := &cobra.Command{
		Use:     "backup",
		Short:   "Create blockchain backup file by fetching blockchain data from the running node",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(backupCmd)

	setFlags(backupCmd)
	helper.SetRequiredFlags(backupCmd, params.getRequiredFlags())

	return backupCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.out,
		outFlag,
		"",
		"the export path for the backup",
	)

	cmd.Flags().StringVar(
		&params.fromRaw,
		fromFlag,
		"0",
		"the beginning height of the chain in backup",
	)

	cmd.Flags().StringVar(
		&params.toRaw,
		toFlag,
		"",
		"the end height of the chain in backup",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.createBackup(helper.GetGRPCAddress(cmd)); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
