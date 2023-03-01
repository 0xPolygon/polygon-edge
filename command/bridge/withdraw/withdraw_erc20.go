package withdraw

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
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
	*common.BridgeParams
	childPredicateAddr string
	childTokenAddr     string
	jsonRPCAddress     string
}

var (
	wp *withdrawParams = &withdrawParams{}
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

	return withdrawCmd
}

func runPreRunWithdraw(cmd *cobra.Command, _ []string) error {
	sharedParams, err := common.GetBridgeParams(cmd)
	if err != nil {
		return err
	}

	wp.BridgeParams = sharedParams

	if err = wp.ValidateFlags(); err != nil {
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

	// TODO: Consider to paralellize. The reason why it isn't paralellized is because txn.Nonce isn't set correctly
	// (nonce is too low for all except the 1st transaction)
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
	}
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
