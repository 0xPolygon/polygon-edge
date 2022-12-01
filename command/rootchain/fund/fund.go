package fund

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	basicParams fundParams
	fundNumber  int
	jsonrpcAddr string
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
		&basicParams.dataDir,
		dataDirFlag,
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&basicParams.configPath,
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

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(dataDirFlag, configFlag)

	// num flag should be used with data-dir flag only so it should not be used with config flag.
	cmd.MarkFlagsMutuallyExclusive(numFlag, configFlag)

	cmd.Flags().StringVar(
		&jsonrpcAddr,
		"jsonrpc",
		"http://localhost:8545",
		"Jsonrpc address of the rootchain",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return basicParams.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	paramsList := getParamsList()
	resList := make(command.Results, len(paramsList))

	relayer, err := txrelayer.NewTxRelayer(jsonrpcAddr)
	if err != nil {
		outputter.SetError(err)

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

		addr := ethgo.Address(validatorAcc)
		txn := &ethgo.Transaction{
			To:    &(addr),
			Value: big.NewInt(1000000000000000000),
		}
		receipt, err := relayer.SendTxnLocal(txn)
		if err != nil {
			outputter.SetError(err)

			return
		}

		resList[i] = &result{
			ValidatorAddr: validatorAcc,
			TxHash:        types.Hash(receipt.TransactionHash),
		}
	}

	outputter.SetCommandResult(resList)
}

// getParamsList creates a list of initParams with num elements.
// This function basically copies the given initParams but updating dataDir by applying an index.
func getParamsList() []fundParams {
	if fundNumber == 1 {
		return []fundParams{basicParams}
	}

	paramsList := make([]fundParams, fundNumber)
	for i := 1; i <= fundNumber; i++ {
		paramsList[i-1] = fundParams{
			dataDir:    fmt.Sprintf("%s%d", basicParams.dataDir, i),
			configPath: basicParams.configPath,
		}
	}

	return paramsList
}
