package erc20

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
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
		Short:   "Withdraws ERC-20 tokens from the destination to the origin chain",
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
		"child ERC-20 predicate address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.TokenAddr,
		common.ChildTokenFlag,
		contracts.NativeERC20TokenContract.String(),
		"child ERC-20 token address",
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

	exitEventIDs := make([]string, len(wp.Receivers))
	blockNumbers := make([]string, len(wp.Receivers))

	for i := range wp.Receivers {
		receiver := wp.Receivers[i]
		amount := wp.Amounts[i]

		amountBig, err := types.ParseUint256orHex(&amount)
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
			exitEventID, err := common.ExtractExitEventID(receipt)
			if err != nil {
				outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

				return
			}

			exitEventIDs[i] = strconv.FormatUint(exitEventID.Uint64(), 10)
		}

		blockNumbers[i] = strconv.FormatUint(receipt.BlockNumber, 10)
	}

	outputter.SetCommandResult(
		&withdrawResult{
			Sender:       senderAccount.Address().String(),
			Receivers:    wp.Receivers,
			Amounts:      wp.Amounts,
			ExitEventIDs: exitEventIDs,
			BlockNumbers: blockNumbers,
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

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

type withdrawResult struct {
	Sender       string   `json:"sender"`
	Receivers    []string `json:"receivers"`
	Amounts      []string `json:"amounts"`
	ExitEventIDs []string `json:"exitEventIDs"`
	BlockNumbers []string `json:"blockNumbers"`
}

func (r *withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 5)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))

	if !wp.ChildChainMintable {
		vals = append(vals, fmt.Sprintf("Exit Event IDs|%s", strings.Join(r.ExitEventIDs, ", ")))
	}

	vals = append(vals, fmt.Sprintf("Inclusion Block Numbers|%s", strings.Join(r.BlockNumbers, ", ")))

	buffer.WriteString("\n[WITHDRAW ERC-20]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
