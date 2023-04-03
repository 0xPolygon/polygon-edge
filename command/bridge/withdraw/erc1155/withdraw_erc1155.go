package erc1155

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
	wp *common.ERC1155BridgeParams = common.NewERC1155BridgeParams()
)

// GetCommand returns the bridge withdraw command
func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-erc1155",
		Short:   "Withdraws ERC 1155 tokens from the child chain to the root chain",
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
		"amounts that are sent to the receivers accounts",
	)

	withdrawCmd.Flags().StringSliceVar(
		&wp.TokenIDs,
		common.TokenIDsFlag,
		nil,
		"token ids that are sent to the receivers accounts",
	)

	withdrawCmd.Flags().StringVar(
		&wp.PredicateAddr,
		common.ChildPredicateFlag,
		contracts.ChildERC1155PredicateContract.String(),
		"ERC 1155 child chain predicate address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.TokenAddr,
		common.ChildTokenFlag,
		contracts.ChildERC1155Contract.String(),
		"ERC 1155 child chain token address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.JSONRPCAddr,
		common.JSONRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint",
	)

	_ = withdrawCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = withdrawCmd.MarkFlagRequired(common.AmountsFlag)
	_ = withdrawCmd.MarkFlagRequired(common.TokenIDsFlag)

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
		outputter.SetError(fmt.Errorf("could not create child chain tx relayer: %w", err))

		return
	}

	receivers := make([]ethgo.Address, len(wp.Receivers))
	amounts := make([]*big.Int, len(wp.Receivers))
	TokenIDs := make([]*big.Int, len(wp.Receivers))

	for i, receiverRaw := range wp.Receivers {
		receivers[i] = ethgo.Address(types.StringToAddress(receiverRaw))
		amountRaw := wp.Amounts[i]
		tokenIDRaw := wp.TokenIDs[i]

		amount, err := types.ParseUint256orHex(&amountRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided amount %s: %w", amountRaw, err))

			return
		}

		tokenID, err := types.ParseUint256orHex(&tokenIDRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided token id %s: %w", amountRaw, err))

			return
		}

		amounts[i] = amount
		TokenIDs[i] = tokenID
	}

	// withdraw tokens transaction
	txn, err := createWithdrawTxn(receivers, amounts, TokenIDs)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	receipt, err := txRelayer.SendTransaction(txn, senderAccount)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send withdrawal transaction (receivers: %s, amounts: %s, token ids: %s): %w",
			strings.Join(wp.Receivers, ", "), strings.Join(wp.Amounts, ", "), strings.Join(wp.TokenIDs, ", "), err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to execute withdrawal transaction (receivers: %s, amounts: %s, token ids: %s)",
			strings.Join(wp.Receivers, ", "), strings.Join(wp.Amounts, ", "), strings.Join(wp.TokenIDs, ", ")))

		return
	}

	exitEventID, err := extractExitEventID(receipt)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

		return
	}

	outputter.SetCommandResult(
		&withdrawResult{
			Sender:      senderAccount.Address().String(),
			Receivers:   wp.Receivers,
			Amounts:     wp.Amounts,
			TokenIDs:    wp.TokenIDs,
			ExitEventID: strconv.FormatUint(exitEventID.Uint64(), 10),
			BlockNumber: strconv.FormatUint(receipt.BlockNumber, 10),
		})
}

// createWithdrawTxn encodes parameters for withdraw function on child chain predicate contract
func createWithdrawTxn(receivers []ethgo.Address, amounts, TokenIDs []*big.Int) (*ethgo.Transaction, error) {
	withdrawFn := &contractsapi.WithdrawBatchChildERC1155PredicateFn{
		ChildToken: types.StringToAddress(wp.TokenAddr),
		Receivers:  receivers,
		Amounts:    amounts,
		TokenIDs:   TokenIDs,
	}

	input, err := withdrawFn.EncodeAbi()
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
	Sender      string   `json:"sender"`
	Receivers   []string `json:"receivers"`
	Amounts     []string `json:"amounts"`
	TokenIDs    []string `json:"TokenIDs"`
	ExitEventID string   `json:"exitEventID"`
	BlockNumber string   `json:"blockNumber"`
}

func (r *withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 6)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))
	vals = append(vals, fmt.Sprintf("Token IDs|%s", strings.Join(r.TokenIDs, ", ")))
	vals = append(vals, fmt.Sprintf("Exit Event ID|%s", r.ExitEventID))
	vals = append(vals, fmt.Sprintf("Inclusion Block Number|%s", r.BlockNumber))

	buffer.WriteString("\n[WITHDRAW ERC1155]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
