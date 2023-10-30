package mint

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
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
	cmd.Flags().StringSliceVar(
		&params.addresses,
		rootHelper.AddressesFlag,
		nil,
		"addresses to which tokens should be minted",
	)

	cmd.Flags().StringSliceVar(
		&params.amounts,
		rootHelper.AmountsFlag,
		nil,
		"token amounts which should be minted to given addresses",
	)

	cmd.Flags().StringVar(
		&params.tokenAddr,
		rootHelper.Erc20TokenFlag,
		"",
		"address of the erc20 token to be minted",
	)

	cmd.Flags().StringVar(
		&params.deployerPrivateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		"private key of the token deployer (minter)",
	)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize tx relayer: %w", err))

		return
	}

	deployerKey, err := rootHelper.DecodePrivateKey(params.deployerPrivateKey)
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

				mintTxn, err := rootHelper.CreateMintTxn(addr, tokenAddr, amount, true)
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
