package fund

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/helper"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params fundParams
)

// GetCommand returns the bridge fund command
func GetCommand() *cobra.Command {
	rootchainFundCmd := &cobra.Command{
		Use:     "fund",
		Short:   "Fund validator account with given tokens amount",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	setFlags(rootchainFundCmd)

	return rootchainFundCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(
		&params.rawAddresses,
		helper.AddressesFlag,
		nil,
		"validator addresses",
	)

	cmd.Flags().StringSliceVar(
		&params.amounts,
		helper.AmountsFlag,
		nil,
		"token amounts which is funded to validator on a root chain",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the rootchain JSON RPC endpoint",
	)

	cmd.Flags().StringVar(
		&params.deployerPrivateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		polybftsecrets.PrivateKeyFlagDesc,
	)

	cmd.Flags().DurationVar(
		&params.txTimeout,
		cmdHelper.TxTimeoutFlag,
		txrelayer.DefaultTimeoutTransactions,
		cmdHelper.TxTimeoutDesc,
	)
}

func preRunCommand(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPCAddress),
		txrelayer.WithReceiptsTimeout(params.txTimeout))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize tx relayer: %w", err))

		return
	}

	deployerKey, err := helper.DecodePrivateKey(params.deployerPrivateKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize deployer private key: %w", err))

		return
	}

	results := make([]command.CommandResult, len(params.addresses))
	g, ctx := errgroup.WithContext(cmd.Context())

	for i := 0; i < len(params.addresses); i++ {
		i := i

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()

			default:
				fundAddr := params.addresses[i]
				txn := helper.CreateTransaction(types.ZeroAddress, &fundAddr, nil, params.amountValues[i], true)

				var (
					receipt *ethgo.Receipt
					err     error
				)

				if params.deployerPrivateKey != "" {
					receipt, err = txRelayer.SendTransaction(txn, deployerKey)
				} else {
					receipt, err = txRelayer.SendTransactionLocal(txn)
				}

				if err != nil {
					return fmt.Errorf("failed to send fund validator '%s' transaction: %w", fundAddr, err)
				}

				if receipt.Status == uint64(types.ReceiptFailed) {
					return fmt.Errorf("failed to fund validator '%s'", fundAddr)
				}

				results[i] = &result{
					ValidatorAddr: types.Address(fundAddr),
					TxHash:        types.Hash(receipt.TransactionHash),
				}
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		outputter.SetError(err)
		_, _ = outputter.Write([]byte("[BRIDGE FUND] Successfully funded following accounts\n"))

		for _, result := range results {
			if result != nil {
				// In case an error happened, some of the indices may not be populated.
				// Filter those out.
				outputter.SetCommandResult(result)
			}
		}

		return
	}

	outputter.SetCommandResult(command.Results(results))
}
