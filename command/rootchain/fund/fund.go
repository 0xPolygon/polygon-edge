package fund

import (
	"fmt"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params         fundParams
	fundNumber     int
	jsonRPCAddress string
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
		&jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:8545",
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
	resList := make(command.Results, 0)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize tx relayer: %w", err))

		return
	}

	var (
		wg    sync.WaitGroup
		lock  sync.Mutex
		errCh = make(chan error)
	)

	sendFundTx := func(params fundParams) {
		defer wg.Done()

		if err := params.initSecretsManager(); err != nil {
			errCh <- err

			return
		}

		validatorAcc, err := params.getValidatorAccount()
		if err != nil {
			errCh <- err

			return
		}

		fundAddr := ethgo.Address(validatorAcc)
		txn := &ethgo.Transaction{
			To:    &fundAddr,
			Value: ethgo.Ether(100),
		}

		receipt, err := txRelayer.SendTransactionLocal(txn)
		if err != nil {
			errCh <- err

			return
		}

		lock.Lock()

		resList = append(resList, &result{
			ValidatorAddr: validatorAcc,
			TxHash:        types.Hash(receipt.TransactionHash),
		})

		lock.Unlock()
	}

	for _, params := range paramsList {
		wg.Add(1)

		go sendFundTx(params)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	var resultingError error
	for err := range errCh {
		resultingError = multierror.Append(resultingError, err)
	}

	if resultingError != nil {
		outputter.SetError(resultingError)
	} else {
		outputter.SetCommandResult(resList)
	}
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
