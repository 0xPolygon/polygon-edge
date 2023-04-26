package unstaking

import (
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

var params unstakeParams

func GetCommand() *cobra.Command {
	unstakeCmd := &cobra.Command{
		Use:     "unstake",
		Short:   "Unstakes the amount sent for validator or undelegates amount from validator",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	helper.RegisterJSONRPCFlag(unstakeCmd)
	setFlags(unstakeCmd)

	return unstakeCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.accountDir,
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.accountConfig,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
	)

	cmd.Flags().Uint64Var(
		&params.amount,
		sidechainHelper.AmountFlag,
		0,
		"amount to unstake from validator",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	validatorAccount, err := sidechainHelper.GetAccount(params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptTimeout(150*time.Millisecond))
	if err != nil {
		return err
	}

	unstakeFn := &contractsapi.UnstakeValidatorSetFn{
		Amount: new(big.Int).SetUint64(params.amount),
	}

	encoded, err := unstakeFn.EncodeAbi()
	if err != nil {
		return err
	}

	receipt, err := sendTransactionToValidatorSetContract(encoded, validatorAccount.Ecdsa, txRelayer, "unstake")
	if err != nil {
		return err
	}

	var (
		withdrawalRegisteredEvent contractsapi.WithdrawalRegisteredEvent
		withdrawalEvent           contractsapi.WithdrawalEvent
		foundLog                  bool
	)

	// check the logs to check for the result
	for _, log := range receipt.Logs {
		doesMatch, err := withdrawalRegisteredEvent.ParseLog(log)
		if err != nil {
			return err
		}

		if doesMatch {
			foundLog = true

			break
		}
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that unstake happened (withdrawal registered)")
	}

	foundLog = false

	encoded, err = contractsapi.ValidatorSet.Abi.Methods["withdraw"].Encode([]interface{}{})
	if err != nil {
		return err
	}

	receipt, err = sendTransactionToValidatorSetContract(encoded, validatorAccount.Ecdsa, txRelayer, "withdraw")
	if err != nil {
		return err
	}

	result := &unstakeResult{
		validatorAddress: validatorAccount.Ecdsa.Address().String(),
	}

	// check the logs to check for the result
	for _, log := range receipt.Logs {
		doesMatch, err := withdrawalEvent.ParseLog(log)
		if err != nil {
			return err
		}

		if doesMatch {
			foundLog = true
			result.amount = withdrawalEvent.Amount.Uint64()

			break
		}
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that withdraw happened on ValidatorSet")
	}

	outputter.WriteCommandResult(result)

	return nil
}

func sendTransactionToValidatorSetContract(encoded []byte, senderKey ethgo.Key,
	txRelayer txrelayer.TxRelayer, txName string) (*ethgo.Receipt, error) {
	txn := &ethgo.Transaction{
		From:     senderKey.Address(),
		Input:    encoded,
		To:       (*ethgo.Address)(&contracts.ValidatorSetContract),
		GasPrice: sidechainHelper.DefaultGasPrice,
	}

	receipt, err := txRelayer.SendTransaction(txn, senderKey)
	if err != nil {
		return nil, err
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("%s transaction failed on block: %d", txName, receipt.BlockNumber)
	}

	return receipt, nil
}
