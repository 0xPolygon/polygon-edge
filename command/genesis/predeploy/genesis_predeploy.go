package predeploy

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

func GetCommand() *cobra.Command {
	genesisPredeployCmd := &cobra.Command{
		Use:     "predeploy",
		Short:   "Specifies the contract to be predeployed on chain start",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(genesisPredeployCmd)
	helper.SetRequiredFlags(genesisPredeployCmd, params.getRequiredFlags())

	return genesisPredeployCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		chainFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the genesis file to update",
	)

	cmd.Flags().StringVar(
		&params.addressRaw,
		predeployAddressFlag,
		predeployAddressMin.String(),
		fmt.Sprintf("the address to predeploy to. Must be >= %s", predeployAddressMin.String()),
	)

	cmd.Flags().StringVar(
		&params.artifactsName,
		artifactsNameFlag,
		"",
		"the built-in contract artifact name",
	)

	cmd.Flags().StringVar(
		&params.artifactsPath,
		artifactsPathFlag,
		"",
		"the path to the contract artifacts JSON",
	)

	cmd.Flags().StringArrayVar(
		&params.constructorArgs,
		constructorArgsFlag,
		[]string{},
		"the constructor arguments, if any",
	)

	cmd.Flags().StringVar(
		&params.deployerAddrRaw,
		deployerAddrFlag,
		"0x0",
		"address of contract deployer",
	)

	cmd.MarkFlagsMutuallyExclusive(artifactsNameFlag, artifactsPathFlag)
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
