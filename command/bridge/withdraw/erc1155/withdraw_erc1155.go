package erc1155

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

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
	wp *common.ERC1155BridgeParams = common.NewERC1155BridgeParams()
)

// GetCommand returns the bridge withdraw command
func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-erc1155",
		Short:   "Withdraws ERC 1155 tokens from the destination to the origin chain",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	wp.RegisterCommonFlags(withdrawCmd)

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

		amount, err := helperCommon.ParseUint256orHex(&amountRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided amount %s: %w", amountRaw, err))

			return
		}

		tokenID, err := helperCommon.ParseUint256orHex(&tokenIDRaw)
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

	res := &common.BridgeTxResult{
		Sender:       senderAccount.Address().String(),
		Receivers:    wp.Receivers,
		Amounts:      wp.Amounts,
		TokenIDs:     wp.TokenIDs,
		BlockNumbers: []uint64{receipt.BlockNumber},
		Title:        "WITHDRAW ERC 1155",
	}

	if !wp.ChildChainMintable {
		exitEventIDs, err := common.ExtractExitEventIDs(receipt)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to extract exit event: %w", err))

			return
		}

		res.ExitEventIDs = exitEventIDs
	}

	outputter.SetCommandResult(res)
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

	return helper.CreateTransaction(ethgo.ZeroAddress, &addr, input,
		nil, wp.ChildChainMintable), nil
}
