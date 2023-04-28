package erc20

import (
	"bytes"
	"encoding/hex"
	"errors"
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
		Short:   "Withdraws ERC 20 tokens from the child chain to the root chain",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	withdrawCmd.Flags().StringVar(
		&wp.SenderKey,
		common.SenderKeyFlag,
		"",
		"withdraw transaction sender hex-encoded private key",
	)

	withdrawCmd.Flags().StringSliceVar(
		&wp.Receivers,
		common.ReceiversFlag,
		nil,
		"receiving accounts addresses on the root chain",
	)

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
		"ERC20 child chain predicate address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.TokenAddr,
		common.ChildTokenFlag,
		contracts.NativeERC20TokenContract.String(),
		"ERC20 child chain token address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.JSONRPCAddr,
		common.JSONRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint",
	)

	_ = withdrawCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = withdrawCmd.MarkFlagRequired(common.AmountsFlag)

	return withdrawCmd
}

func preRunCommand(cmd *cobra.Command, _ []string) error {
	if err := wp.Validate(); err != nil {
		return err
	}

	return nil
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
		outputter.SetError(fmt.Errorf("could not create child chain tx relayer: %w", err))

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

		exitEventID, err := extractExitEventID(receipt)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

			return
		}

		exitEventIDs[i] = strconv.FormatUint(exitEventID.Uint64(), 10)
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

// createWithdrawTxn encodes parameters for withdraw function on child chain predicate contract
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

// extractExitEventID tries to extract exit event id from provided receipt
func extractExitEventID(receipt *ethgo.Receipt) (*big.Int, error) {
	var exitEvent contractsapi.L2StateSyncedEvent
	for _, log := range receipt.Logs {
		doesMatch, err := exitEvent.ParseLog(log)
		if err != nil {
			return nil, err
		}

		if !doesMatch {
			continue
		}

		return exitEvent.ID, nil
	}

	return nil, errors.New("failed to find exit event log")
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
	vals = append(vals, fmt.Sprintf("Exit Event IDs|%s", strings.Join(r.ExitEventIDs, ", ")))
	vals = append(vals, fmt.Sprintf("Inclusion Block Numbers|%s", strings.Join(r.BlockNumbers, ", ")))

	buffer.WriteString("\n[WITHDRAW ERC 20]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
