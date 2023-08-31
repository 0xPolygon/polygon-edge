package withdraw

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	helperCommon "github.com/0xPolygon/polygon-edge/helper/common"
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
		Short:   "Withdraws ERC 721 tokens from the destination to the origin chain",
		PreRunE: preRun,
		Run:     run,
	}

	wp.RegisterCommonFlags(withdrawCmd)

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

	_ = withdrawCmd.MarkFlagRequired(common.SenderKeyFlag)
	_ = withdrawCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = withdrawCmd.MarkFlagRequired(common.TokenIDsFlag)

	return withdrawCmd
}

func preRun(_ *cobra.Command, _ []string) error {
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

		tokenID, err := helperCommon.ParseUint256orHex(&tokenIDRaw)
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

	res := &common.BridgeTxResult{
		Sender:       senderAccount.Address().String(),
		Receivers:    wp.Receivers,
		TokenIDs:     wp.TokenIDs,
		BlockNumbers: []uint64{receipt.BlockNumber},
		Title:        "WITHDRAW ERC 721",
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

	return helper.CreateTransaction(ethgo.ZeroAddress, &addr, input,
		nil, wp.ChildChainMintable), nil
}
