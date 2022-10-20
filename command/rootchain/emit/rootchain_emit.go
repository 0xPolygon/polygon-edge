package emit

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params emitParams

	contractsToParamTypes = map[string]string{
		contracts.NativeTokenContract.String(): "tuple(address,uint256)",
	}
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
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	paramsType, exists := contractsToParamTypes[params.address]
	if !exists {
		outputter.SetError(fmt.Errorf("no parameter types for given contract address: %v", params.address))

		return
	}

	pendingNonce, err := helper.GetPendingNonce(helper.GetDefAccount())
	if err != nil {
		outputter.SetError(fmt.Errorf("could not get pending nonce: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	for i := range params.wallets {
		wallet := params.wallets[i]
		amount := params.amounts[i]
		walletIndex := uint64(i)

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				txn, err := createTxInput(paramsType, wallet, amount)
				if err != nil {
					return fmt.Errorf("failed to create tx input: %w", err)
				}

				if _, err = helper.SendTxn(pendingNonce+walletIndex, txn); err != nil {
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

func createTxInput(paramsType string, parameters ...interface{}) (*ethgo.Transaction, error) {
	var prms []interface{}
	prms = append(prms, parameters...)

	wrapperInput, err := abi.MustNewType(paramsType).Encode(prms)
	if err != nil {
		return nil, fmt.Errorf("failed to encode parsed parameters: %w", err)
	}

	artifact, err := polybftcontracts.ReadArtifact(helper.ContractRootFolder, "root/StateSender.sol", "StateSender")
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact: %w", err)
	}

	method := artifact.Abi.Methods["syncState"]

	input, err := method.Encode([]interface{}{types.StringToAddress(params.address), wrapperInput})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	return &ethgo.Transaction{
		To:    (*ethgo.Address)(&helper.StateSenderAddress),
		Input: input,
	}, nil
}
