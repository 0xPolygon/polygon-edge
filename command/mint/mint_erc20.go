package mint

import (
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	"github.com/0xPolygon/polygon-edge/command/helper"
	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	params mintParams
)

// GetCommand returns the rootchain fund command
func GetCommand() *cobra.Command {
	mintCmd := &cobra.Command{
		Use:     "mint-erc20",
		Short:   "Mints ERC20 tokens to specified addresses",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	helper.RegisterJSONRPCFlag(mintCmd)

	setFlags(mintCmd)

	return mintCmd
}

func preRunCommand(cmd *cobra.Command, _ []string) error {
	params.jsonRPCAddress = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.minterPrivateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		"the minter private key",
	)

	cmd.Flags().StringSliceVar(
		&params.addresses,
		bridgeHelper.AddressesFlag,
		nil,
		"receivers addresses",
	)

	cmd.Flags().StringSliceVar(
		&params.amounts,
		bridgeHelper.AmountsFlag,
		nil,
		"erc20 token amounts",
	)

	cmd.Flags().StringVar(
		&params.tokenAddr,
		bridgeHelper.Erc20TokenFlag,
		"",
		"erc20 token address",
	)

	cmd.Flags().Uint64Var(
		&params.txTimeout,
		bridgeHelper.TxTimeoutFlag,
		5000,
		"timeout for receipts in milliseconds",
	)

	cmd.Flags().Uint64Var(
		&params.txPollFreq,
		bridgeHelper.TxPollFreqFlag,
		50,
		"frequency in milliseconds for poll transactions",
	)

	_ = cmd.MarkFlagRequired(bridgeHelper.Erc20TokenFlag)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPCAddress),
		txrelayer.WithReceiptsTimeout(time.Duration(params.txTimeout*uint64(time.Millisecond))),
		txrelayer.WithReceiptsPollFreq(time.Duration(params.txPollFreq*uint64(time.Millisecond))))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize tx relayer: %w", err))

		return
	}

	deployerKey, err := bridgeHelper.DecodePrivateKey(params.minterPrivateKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize deployer private key: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	results := make([]command.CommandResult, len(params.addresses))
	tokenAddr := types.StringToAddress(params.tokenAddr)

	for i := 0; i < len(params.addresses); i++ {
		i := i

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()

			default:
				// mint tokens to address
				addr := types.StringToAddress(params.addresses[i])
				amount := params.amountValues[i]

				mintTxn, err := bridgeHelper.CreateMintTxn(addr, tokenAddr, amount, true)
				if err != nil {
					return fmt.Errorf("failed to create mint native tokens transaction for validator '%s'. err: %w",
						addr, err)
				}

				receipt, err := txRelayer.SendTransaction(mintTxn, deployerKey)
				if err != nil {
					return fmt.Errorf("failed to send mint native tokens transaction to validator '%s'. err: %w", addr, err)
				}

				if receipt.Status == uint64(types.ReceiptFailed) {
					return fmt.Errorf("failed to mint native tokens to validator '%s'", addr)
				}

				results[i] = &mintResult{
					Address: addr,
					Amount:  amount,
					TxHash:  types.Hash(receipt.TransactionHash),
				}

				return nil
			}
		})
	}

	if err := g.Wait(); err != nil {
		outputter.SetError(err)
		_, _ = outputter.Write([]byte("[MINT-ERC20] Successfully minted tokens to following accounts\n"))

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
