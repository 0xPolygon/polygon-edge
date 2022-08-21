package contractdeployment

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	contractDeploymentCmd := &cobra.Command{
		Use:     "contractDeployment",
		Short:   "Updates smart contract deployment whitelist",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(contractDeploymentCmd)

	return contractDeploymentCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		chainFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the genesis file to update",
	)
	cmd.Flags().StringArrayVar(
		&params.addAddressRaw,
		addAddressFlag,
		[]string{},
		"adds a new address in the contract deployment whitelist",
	)

	cmd.Flags().StringArrayVar(
		&params.removeAddressRaw,
		removeAddressFlag,
		[]string{},
		"removes a new address from the contract deployment whitelist",
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
