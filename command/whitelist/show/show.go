package show

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	showCmd := &cobra.Command{
		Use:     "show",
		Short:   "Displays whitelist information",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(showCmd)

	return showCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		chainFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the genesis file to update",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.initRawParams()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	outputter.SetCommandResult(params.getResult())
}
