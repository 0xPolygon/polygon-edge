package withdraw

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
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
func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-erc20",
		Short:   "Withdraws tokens from the child chain to the root chain",
		PreRunE: preRun,
		Run:     run,
	}

	withdrawCmd.Flags().StringVar(
		&wp.AccountDir,
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	withdrawCmd.Flags().StringVar(
		&wp.AccountConfig,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
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

	withdrawCmd.MarkFlagRequired(common.ReceiversFlag) //nolint:errcheck
	withdrawCmd.MarkFlagRequired(common.AmountsFlag)   //nolint:errcheck

	withdrawCmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)

	return withdrawCmd
}

func preRun(cmd *cobra.Command, _ []string) error {
	if err := wp.ValidateFlags(false); err != nil {
		return err
	}

	return nil
}

func run(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	senderAccount, err := sidechain.GetAccount(wp.AccountDir, wp.AccountConfig)
	if err != nil {
		outputter.SetError(err)

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
		txn, err := createWithdrawTxn(types.StringToAddress(receiver), amountBig)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

			return
		}

		receipt, err := txRelayer.SendTransaction(txn, senderAccount.Ecdsa)
		if err != nil {
			outputter.SetError(fmt.Errorf("receiver: %s, amount: %s, error: %w", receiver, amount, err))

			return
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			outputter.SetError(fmt.Errorf("receiver: %s, amount: %s", receiver, amount))

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
		&withdrawERC20Result{
			Sender:       senderAccount.Ecdsa.Address().String(),
			Receivers:    wp.Receivers,
			Amounts:      wp.Amounts,
			ExitEventIDs: exitEventIDs,
			BlockNumbers: blockNumbers,
		})
}

// createWithdrawTxn encodes parameters for withdraw function on child chain predicate contract
func createWithdrawTxn(receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
	withdrawToFn := &contractsapi.WithdrawToFunction{
		ChildToken: types.StringToAddress(wp.childTokenAddr),
		Receiver:   receiver,
		Amount:     amount,
	}

	input, err := withdrawToFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(wp.childPredicateAddr))

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

// extractExitEventID tries to extract exit event id from provided receipt
func extractExitEventID(receipt *ethgo.Receipt) (*big.Int, error) {
	for _, log := range receipt.Logs {
		if !polybft.ExitEventABI.Match(log) {
			continue
		}

		exitEvent := &contractsapi.L2StateSyncedEvent{}
		if err := exitEvent.ParseLog(log); err != nil {
			return nil, err
		}

		return exitEvent.ID, nil
	}

	return nil, errors.New("failed to find exit event log")
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
