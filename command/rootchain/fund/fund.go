package fund

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
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
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
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

	cmd.Flags().StringVar(
		&params.nativeRootTokenAddr,
		helper.NativeRootTokenFlag,
		"",
		helper.NativeRootTokenFlagDesc,
	)

	cmd.Flags().BoolVar(
		&params.mintRootToken,
		mintRootTokenFlag,
		false,
		"indicates if root token deployer should mint root tokens to given validators",
	)

	cmd.Flags().StringVar(
		&params.deployerPrivateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		polybftsecrets.PrivateKeyFlagDesc,
	)

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)

	// num flag should be used with data-dir flag only so it should not be used with config flag.
	cmd.MarkFlagsMutuallyExclusive(numFlag, polybftsecrets.AccountConfigFlag)
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
		depositorKey  ethgo.Key
		rootTokenAddr types.Address
	)

	if params.mintRootToken {
		depositorKey, err = helper.GetRootchainPrivateKey(params.deployerPrivateKey)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to initialize depositor private key: %w", err))
		}

		rootTokenAddr = types.StringToAddress(params.nativeRootTokenAddr)
	}

	var (
		lock   sync.Mutex
		g, ctx = errgroup.WithContext(cmd.Context())
		amount = ethgo.Ether(100)
	)

	for _, params := range paramsList {
		params := params

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if err := params.initSecretsManager(); err != nil {
					return err
				}

				validatorAcc, err := params.getValidatorAccount()
				if err != nil {
					return err
				}

				fundAddr := ethgo.Address(validatorAcc)
				txn := &ethgo.Transaction{
					To:    &fundAddr,
					Value: amount,
				}

				receipt, err := txRelayer.SendTransactionLocal(txn)
				if err != nil {
					return err
				}

				result := &result{
					ValidatorAddr: validatorAcc,
					TxHash:        types.Hash(receipt.TransactionHash),
				}

				if params.mintRootToken {
					// mint tokens to validator, so he is able to send them
					mintTxn, err := helper.CreateMintTxn(validatorAcc, validatorAcc, rootTokenAddr, amount)
					if err != nil {
						return fmt.Errorf("mint transaction creation failed for validator: %s. err: %w", validatorAcc, err)
					}

					receipt, err := txRelayer.SendTransaction(mintTxn, depositorKey)
					if err != nil {
						return fmt.Errorf("failed to send mint transaction to depositor %s. err: %w", validatorAcc, err)
					}

					if receipt.Status == uint64(types.ReceiptFailed) {
						return fmt.Errorf("failed to mint tokens to depositor %s", validatorAcc)
					}

					result.IsMinted = true
				}

				lock.Lock()

				resList = append(resList, result)

				lock.Unlock()

				return nil
			}
		})
	}

	if err = g.Wait(); err != nil {
		outputter.SetError(fmt.Errorf("fund command failed. error: %w", err))

		return
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
			dataDir:       fmt.Sprintf("%s%d", params.dataDir, i),
			configPath:    params.configPath,
			mintRootToken: params.mintRootToken,
		}
	}

	return paramsList
}
