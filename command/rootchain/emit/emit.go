package emit

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params emitParams

	jsonRPCAddress string

	contractsToParamTypes = map[string]string{
		contracts.NativeTokenContract.String(): "tuple(address,uint256)",
	}

	syncStateAbiMethod, _ = abi.NewMethod("function syncState(address receiver, bytes data)")
)

// GetCommand returns the rootchain emit command
func GetCommand() *cobra.Command {
	rootchainEmitCmd := &cobra.Command{
		Use:     "emit",
		Short:   "Emit an event from the bridge",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(rootchainEmitCmd)

	return rootchainEmitCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.address,
		contractFlag,
		contracts.NativeTokenContract.String(),
		"ERC20 bridge contract address",
	)

	cmd.Flags().StringSliceVar(
		&params.wallets,
		walletsFlag,
		nil,
		"list of wallet addresses",
	)

	cmd.Flags().StringSliceVar(
		&params.amounts,
		amountsFlag,
		nil,
		"list of amounts to fund wallets",
	)

	cmd.Flags().StringVar(
		&jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)",
	)

	cmd.Flags().StringVar(
		&params.adminKey,
		adminKeyFlag,
		helper.DefaultPrivateKeyRaw,
		"Hex encoded private key of the account which sends rootchain transactions",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	err := helper.InitRootchainAdminKey(params.adminKey)
	if err != nil {
		outputter.SetError(err)

		return
	}

	paramsType, exists := contractsToParamTypes[params.address]
	if !exists {
		outputter.SetError(fmt.Errorf("no parameter types for given contract address: %v", params.address))

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create rootchain interactor: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	for i := range params.wallets {
		wallet := params.wallets[i]
		amount := params.amounts[i]

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				txn, err := createEmitTxn(paramsType, wallet, amount)
				if err != nil {
					return fmt.Errorf("failed to create tx input: %w", err)
				}

				if _, err = txRelayer.SendTransaction(
					txn,
					helper.GetRootchainAdminKey()); err != nil {
					return fmt.Errorf("sending transaction to wallet: %s with amount: %s, failed with error: %w", wallet, amount, err)
				}

				return nil
			}
		})
	}

	if err = g.Wait(); err != nil {
		outputter.SetError(fmt.Errorf("sending transactions to rootchain failed: %w", err))

		return
	}

	outputter.SetCommandResult(&result{
		Address: params.address,
		Wallets: params.wallets,
		Amounts: params.amounts,
	})
}

func createEmitTxn(paramsType string, parameters ...interface{}) (*ethgo.Transaction, error) {
	var prms []interface{}
	prms = append(prms, parameters...)

	wrapperInput, err := abi.MustNewType(paramsType).Encode(prms)
	if err != nil {
		return nil, fmt.Errorf("failed to encode parsed parameters: %w", err)
	}

	sender := types.StringToAddress(params.address)

	input, err := syncStateAbiMethod.Encode([]interface{}{sender, wrapperInput})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	return &ethgo.Transaction{
		To:    (*ethgo.Address)(&helper.StateSenderAddress),
		Input: input,
	}, nil
}
