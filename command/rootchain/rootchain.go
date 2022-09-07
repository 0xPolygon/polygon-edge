package rootchain

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	rootchainCmd := &cobra.Command{
		Use: "rootchain",
		Short: "Generates the rootchain configuration file with the passed in parameters. " +
			"Appends to the config if it is already present.",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(rootchainCmd)

	helper.SetRequiredFlags(rootchainCmd, params.getRequiredFlags())

	return rootchainCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.configPath,
		dirFlag,
		fmt.Sprintf("./%s", defaultRootchainName),
		"the path for the Polygon Edge rootchain configuration",
	)

	cmd.Flags().StringVar(
		&params.rootchainAddr,
		rootchainAddrFlag,
		"",
		"the JSON-RPC address of the rootchain network",
	)

	cmd.Flags().StringVar(
		&params.eventABI,
		eventABIFlag,
		"",
		"the ABI of the event to listen for",
	)

	cmd.Flags().StringVar(
		&params.methodABI,
		methodABIFlag,
		"",
		"the ABI of the local SC method to call",
	)

	cmd.Flags().StringVar(
		&params.localAddr,
		localAddrFlag,
		"",
		"the Ethereum address of the local Smart Contract to call",
	)

	cmd.Flags().StringVar(
		&params.methodName,
		methodNameFlag,
		"",
		"the name of the method being called on the local Smart Contract",
	)

	cmd.Flags().Uint64Var(
		&params.payloadType,
		payloadTypeFlag,
		0,
		"the payload type for the rootchain event. Possible types: [0 - ValidatorSetPayload]",
	)

	cmd.Flags().Uint64Var(
		&params.blockConfirmations,
		blockConfirmationsFlag,
		6,
		"the number of blocks required for making sure the event is sealed",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	if err := params.validateFlags(); err != nil {
		return err
	}

	return params.loadExistingConfig()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.generateConfig(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
