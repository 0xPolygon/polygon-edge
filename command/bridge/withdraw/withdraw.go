package withdraw

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params *common.BridgeParams

	manifest *polybft.Manifest
)

// GetCommand returns the bridge deposit command
func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw",
		Short:   "Withdraws tokens from the child chain to the root chain",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	return withdrawCmd
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	var err error

	params, err = common.GetBridgeParams(cmd)
	if err != nil {
		return err
	}

	if err = params.ValidateFlags(); err != nil {
		return err
	}

	manifest, err = polybft.LoadManifest(params.ManifestPath)
	if err != nil {
		return err
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ecdsaRaw, err := hex.DecodeString(params.TxnSenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to decode private key: %w", err))

		return
	}

	key, err := wallet.NewWalletFromPrivKey(ecdsaRaw)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create wallet from private key: %w", err))

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.JSONRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create child chain tx relayer: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	for i := range params.Receivers {
		receiver := params.Receivers[i]
		amount := params.Amounts[i]

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// withdraw tokens
				amountBig, err := types.ParseUint256orHex(&amount)
				if err != nil {
					return fmt.Errorf("failed to decode provided amount %s: %w", amount, err)
				}
				txn, err := createWithdrawTxn(ethgo.BytesToAddress([]byte(receiver)), amountBig)
				if err != nil {
					return fmt.Errorf("failed to create tx input: %w", err)
				}

				receipt, err := txRelayer.SendTransaction(txn, key)
				if err != nil {
					return fmt.Errorf("receiver: %s, amount: %s, error: %w",
						receiver, amount, err)
				}

				if receipt.Status == uint64(types.ReceiptFailed) {
					return fmt.Errorf("receiver: %s, amount: %s",
						receiver, amount)
				}

				return nil
			}
		})
	}

	if err = g.Wait(); err != nil {
		outputter.SetError(fmt.Errorf("sending withdrawal transaction to child chain failed: %w", err))

		return
	}
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createWithdrawTxn(receiver ethgo.Address, amount *big.Int) (*ethgo.Transaction, error) {
	input, err := contractsapi.ChildERC20Predicate.Abi.Methods["withdrawTo"].Encode([]interface{}{
		contracts.NativeERC20TokenContract,
		receiver,
		amount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(contracts.ChildERC20PredicateContract)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}
