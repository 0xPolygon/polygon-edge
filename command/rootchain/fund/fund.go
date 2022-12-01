package fund

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

var (
	params     fundParams
	fundNumber int
)

// GetCommand returns the rootchain fund command
func GetCommand() *cobra.Command {
	rootchainFundCmd := &cobra.Command{
		Use:     "fund",
		Short:   "Fund funds all the genesis addresses",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(rootchainFundCmd)

	return rootchainFundCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dataDir,
		dataDirFlag,
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&params.configPath,
		configFlag,
		"",
		"the path to the SecretsManager config file, "+
			"if omitted, the local FS secrets manager is used",
	)

	cmd.Flags().IntVar(
		&fundNumber,
		numFlag,
		1,
		"the flag indicating the number of accounts to be funded",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		"",
		"the JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)",
	)

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(dataDirFlag, configFlag)

	// num flag should be used with data-dir flag only so it should not be used with config flag.
	cmd.MarkFlagsMutuallyExclusive(numFlag, configFlag)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	paramsList := getParamsList()
	resList := make(command.Results, len(paramsList))

	ipAddress, err := command.ResolveRootchainIP(params.jsonRPCAddress)
	if err != nil {
		outputter.SetError(err)

		return
	}

	rootchainInteractor, err := helper.NewDefaultRootchainInteractor(ipAddress)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize rootchain interactor: %w", err))

		return
	}

	for i, params := range paramsList {
		if err := params.initSecretsManager(); err != nil {
			outputter.SetError(err)

			return
		}

		validatorAcc, err := params.getValidatorAccount()
		if err != nil {
			outputter.SetError(err)

			return
		}

		txHash, err := rootchainInteractor.FundAccount(validatorAcc)
		if err != nil {
			outputter.SetError(err)

			return
		}

		resList[i] = &result{
			ValidatorAddr: validatorAcc,
			TxHash:        txHash,
		}
	}

	outputter.SetCommandResult(resList)
}

// getParamsList creates a list of initParams with num elements.
// This function basically copies the given initParams but updating dataDir by applying an index.
func getParamsList() []fundParams {
	if fundNumber == 1 {
		return []fundParams{params}
	}

	paramsList := make([]fundParams, fundNumber)
	for i := 1; i <= fundNumber; i++ {
		paramsList[i-1] = fundParams{
			dataDir:    fmt.Sprintf("%s%d", params.dataDir, i),
			configPath: params.configPath,
		}
	}

	return paramsList
}
