package withdraw

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
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	jsonRPCFlag        = "json-rpc"
	childPredicateFlag = "child-predicate"
	childTokenFlag     = "child-token"
)

type withdrawParams struct {
	*common.ERC20BridgeParams
	childPredicateAddr string
	childTokenAddr     string
	jsonRPCAddress     string
}

var (
	wp *withdrawParams = &withdrawParams{ERC20BridgeParams: &common.ERC20BridgeParams{}}
)

// GetCommand returns the bridge withdraw command
func GetWithdrawCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-erc20",
		Short:   "Withdraws tokens from the child chain to the root chain",
		PreRunE: runPreRunWithdraw,
		Run:     runCommand,
	}

	withdrawCmd.Flags().StringVar(
		&wp.TxnSenderKey,
		common.SenderKeyFlag,
		"",
		"hex encoded private key of the account which sends withdraw transactions",
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
		&wp.childPredicateAddr,
		childPredicateFlag,
		contracts.ChildERC20PredicateContract.String(),
		"ERC20 child chain predicate address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.childTokenAddr,
		childTokenFlag,
		contracts.NativeERC20TokenContract.String(),
		"ERC20 child chain token address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint",
	)

	withdrawCmd.MarkFlagRequired(common.SenderKeyFlag)
	withdrawCmd.MarkFlagRequired(common.ReceiversFlag)
	withdrawCmd.MarkFlagRequired(common.AmountsFlag)

	return withdrawCmd
}

func runPreRunWithdraw(cmd *cobra.Command, _ []string) error {
	if err := wp.ValidateFlags(); err != nil {
		return err
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ecdsaRaw, err := hex.DecodeString(wp.TxnSenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to decode private key: %w", err))

		return
	}

	key, err := wallet.NewWalletFromPrivKey(ecdsaRaw)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create wallet from private key: %w", err))

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(wp.jsonRPCAddress))
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
		txn, err := createWithdrawTxn(ethgo.BytesToAddress([]byte(receiver)), amountBig)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

			return
		}

		receipt, err := txRelayer.SendTransaction(txn, key)
		if err != nil {
			outputter.SetError(fmt.Errorf("receiver: %s, amount: %s, error: %w", receiver, amount, err))

			return
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			outputter.SetError(fmt.Errorf("receiver: %s, amount: %s", receiver, amount))

			return
		}

		exitEvent, blockNumber, err := extractExitEvent(receipt)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

			return
		}

		exitEventIDs[i] = strconv.FormatUint(exitEvent.ID, 10)
		blockNumbers[i] = strconv.FormatUint(blockNumber, 10)
	}

	outputter.SetCommandResult(
		&withdrawERC20Result{
			Sender:       key.Address().String(),
			Receivers:    wp.Receivers,
			Amounts:      wp.Amounts,
			ExitEventIDs: exitEventIDs,
			BlockNumbers: blockNumbers,
		})
}

// createWithdrawTxn encodes parameters for withdraw function on child chain predicate contract
func createWithdrawTxn(receiver ethgo.Address, amount *big.Int) (*ethgo.Transaction, error) {
	input, err := contractsapi.ChildERC20Predicate.Abi.Methods["withdrawTo"].Encode([]interface{}{
		wp.childTokenAddr,
		receiver,
		amount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(wp.childPredicateAddr))

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

// extractExitEvent tries to extract ExitEvent from provided receipt
func extractExitEvent(receipt *ethgo.Receipt) (*polybft.ExitEvent, uint64, error) {
	exitEventABI := contractsapi.L2StateSender.Abi.Events["L2StateSynced"]

	for _, log := range receipt.Logs {
		if !exitEventABI.Match(log) {
			continue
		}

		exitEventMap, err := exitEventABI.Inputs.ParseLog(log)
		if err != nil {
			return nil, 0, err
		}

		exitEventGeneric, err := polybft.DecodeBridgeEventData(exitEventMap, log, createExitEvent)
		if err != nil {
			return nil, 0, err
		}

		exitEvent, ok := exitEventGeneric.(*polybft.ExitEvent)
		if !ok {
			return nil, 0, errors.New("failed to convert exit event")
		}

		return exitEvent, receipt.BlockNumber, err
	}

	return nil, 0, errors.New("failed to find exit event log")
}

func createExitEvent(id *big.Int, sender ethgo.Address, receiver ethgo.Address, data []byte) interface{} {
	return &polybft.ExitEvent{
		ID:       id.Uint64(),
		Sender:   sender,
		Receiver: receiver,
		Data:     data,
	}
}

type withdrawERC20Result struct {
	Sender       string   `json:"sender"`
	Receivers    []string `json:"receivers"`
	Amounts      []string `json:"amounts"`
	ExitEventIDs []string `json:"exitEventIDs"`
	BlockNumbers []string `json:"blockNumbers"`
}

func (r *withdrawERC20Result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 5)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))
	vals = append(vals, fmt.Sprintf("Exit Event IDs|%s", strings.Join(r.ExitEventIDs, ", ")))
	vals = append(vals, fmt.Sprintf("Inclusion Block Numbers|%s", strings.Join(r.BlockNumbers, ", ")))

	buffer.WriteString("\n[WITHDRAW ERC20]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
