package withdraw

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

var (
	wp *common.ERC721BridgeParams = common.NewERC721BridgeParams()
)

func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-erc721",
		Short:   "Withdraws ERC 721 tokens from the child chain to the root chain",
		PreRunE: preRun,
		Run:     run,
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
		&wp.TokenIDs,
		common.TokenIDsFlag,
		nil,
		"tokens ids to send to receiving accounts",
	)

	withdrawCmd.Flags().StringVar(
		&wp.PredicateAddr,
		common.ChildPredicateFlag,
		contracts.ChildERC721PredicateContract.String(),
		"ERC 721 child chain predicate address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.TokenAddr,
		common.ChildTokenFlag,
		contracts.ChildERC721Contract.String(),
		"ERC 721 child chain token address",
	)

	withdrawCmd.Flags().StringVar(
		&wp.JSONRPCAddr,
		common.JSONRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint",
	)

	_ = withdrawCmd.MarkFlagRequired(common.SenderKeyFlag)
	_ = withdrawCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = withdrawCmd.MarkFlagRequired(common.TokenIDsFlag)

	return withdrawCmd
}

func preRun(cmd *cobra.Command, _ []string) error {
	return wp.Validate()
}

func run(cmd *cobra.Command, _ []string) {
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
	tokenIDs := make([]*big.Int, len(wp.Receivers))

	for i, tokenIDRaw := range wp.TokenIDs {
		tokenIDRaw := tokenIDRaw

		tokenID, err := types.ParseUint256orHex(&tokenIDRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided token id %s: %w", tokenIDRaw, err))

			return
		}

		receivers[i] = ethgo.Address(types.StringToAddress(wp.Receivers[i]))
		tokenIDs[i] = tokenID
	}

	txn, err := createWithdrawTxn(receivers, tokenIDs)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	receipt, err := txRelayer.SendTransaction(txn, senderAccount)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send withdrawal transaction (receivers: %s, token ids: %s): %w",
			strings.Join(wp.Receivers, ", "), strings.Join(wp.TokenIDs, ", "), err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to execute withdrawal transaction (receivers: %s, token ids: %s)",
			strings.Join(wp.Receivers, ", "), strings.Join(wp.TokenIDs, ", ")))

		return
	}

	exitEventIDRaw, err := extractExitEventID(receipt)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

		return
	}

	exitEventID := strconv.FormatUint(exitEventIDRaw.Uint64(), 10)
	blockNumber := strconv.FormatUint(receipt.BlockNumber, 10)

	outputter.SetCommandResult(
		&withdrawERC721Result{
			Sender:      senderAccount.Address().String(),
			Receivers:   wp.Receivers,
			TokenIDs:    wp.TokenIDs,
			ExitEventID: exitEventID,
			BlockNumber: blockNumber,
		})
}

// createWithdrawTxn encodes parameters for withdraw function on child chain predicate contract
func createWithdrawTxn(receivers []ethgo.Address, tokenIDs []*big.Int) (*ethgo.Transaction, error) {
	withdrawToFn := &contractsapi.WithdrawBatchChildERC721PredicateFn{
		ChildToken: types.StringToAddress(wp.TokenAddr),
		Receivers:  receivers,
		TokenIDs:   tokenIDs,
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

type withdrawERC721Result struct {
	Sender      string   `json:"sender"`
	Receivers   []string `json:"receivers"`
	TokenIDs    []string `json:"tokenIDs"`
	ExitEventID string   `json:"exitEventIDs"`
	BlockNumber string   `json:"blockNumbers"`
}

func (r *withdrawERC721Result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 5)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("TokenIDs|%s", strings.Join(r.TokenIDs, ", ")))
	vals = append(vals, fmt.Sprintf("Exit Event IDs|%s", r.ExitEventID))
	vals = append(vals, fmt.Sprintf("Inclusion Block Numbers|%s", r.BlockNumber))

	buffer.WriteString("\n[WITHDRAW ERC 721]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
