package erc20

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	helperCommon "github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	wp *common.ERC20BridgeParams = common.NewERC20BridgeParams()
)

// GetCommand returns the bridge withdraw command
func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-erc20",
		Short:   "Withdraws ERC 20 tokens from the destination to the origin chain",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	wp.RegisterCommonFlags(withdrawCmd)

	withdrawCmd.Flags().StringSliceVar(
		&wp.Amounts,
		common.AmountsFlag,
		nil,
		"amounts to send to receiving accounts",
	)

	withdrawCmd.Flags().StringVar(
		&wp.PredicateAddr,
		common.ChildPredicateFlag,
		contracts.ChildERC20PredicateContract.String(),
		"child ERC 20 predicate address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.TokenAddr,
		common.ChildTokenFlag,
		contracts.NativeERC20TokenContract.String(),
		"child ERC 20 token address",
	)

	_ = withdrawCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = withdrawCmd.MarkFlagRequired(common.AmountsFlag)

	return withdrawCmd
}

func preRunCommand(_ *cobra.Command, _ []string) error {
	return wp.Validate()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	senderKeyRaw, err := hex.DecodeString(wp.SenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to decode sender private key: %w", err))

		return
	}

	senderAccount, err := wallet.NewWalletFromPrivKey(senderKeyRaw)
	if err != nil {
		outputter.SetError(err)

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(wp.JSONRPCAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create destination chain tx relayer: %w", err))

		return
	}

	exitEventIDs := make([]*big.Int, 0, len(wp.Receivers))
	blockNumbers := make([]uint64, len(wp.Receivers))

	for i := range wp.Receivers {
		receiver := wp.Receivers[i]
		amount := wp.Amounts[i]

		amountBig, err := helperCommon.ParseUint256orHex(&amount)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided amount %s: %w", amount, err))

			return
		}

		// withdraw tokens transaction
		txn, err := createWithdrawTxn(types.StringToAddress(receiver), amountBig)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

			return
		}

		receipt, err := txRelayer.SendTransaction(txn, senderAccount)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to send withdraw transaction (receiver: %s, amount: %s). error: %w)",
				receiver, amount, err))

			return
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			outputter.SetError(fmt.Errorf("failed to execute withdrawal (receiver: %s, amount: %s)", receiver, amount))

			return
		}

		if !wp.ChildChainMintable {
			extractedExitEventIDs, err := common.ExtractExitEventIDs(receipt)
			if err != nil {
				outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

				return
			}

			exitEventIDs = append(exitEventIDs, extractedExitEventIDs...)
		}

		blockNumbers[i] = receipt.BlockNumber
	}

	outputter.SetCommandResult(
		&common.BridgeTxResult{
			Sender:       senderAccount.Address().String(),
			Receivers:    wp.Receivers,
			Amounts:      wp.Amounts,
			ExitEventIDs: exitEventIDs,
			BlockNumbers: blockNumbers,
			Title:        "WITHDRAW ERC 20",
		})
}

// createWithdrawTxn encodes parameters for withdraw function on destination predicate contract
func createWithdrawTxn(receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
	withdrawToFn := &contractsapi.WithdrawToChildERC20PredicateFn{
		ChildToken: types.StringToAddress(wp.TokenAddr),
		Receiver:   receiver,
		Amount:     amount,
	}

	input, err := withdrawToFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(wp.PredicateAddr))

	return helper.CreateTransaction(ethgo.ZeroAddress, &addr, input,
		nil, wp.ChildChainMintable), nil
}
