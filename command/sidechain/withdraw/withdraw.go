package withdraw

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

var (
	params           withdrawParams
	withdrawEventABI = contractsapi.ChildValidatorSet.Abi.Events["Withdrawal"]
)

func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw",
		Short:   "Withdraws sender's withdrawable amount to specified address",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(withdrawCmd)

	return withdrawCmd
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

	cmd.Flags().StringVar(
		&params.addressTo,
		addressToFlag,
		"",
		"address where to withdraw withdrawable amount",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	helper.RegisterJSONRPCFlag(cmd)
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

	encoded, err := contractsapi.ChildValidatorSet.Abi.Methods["withdraw"].Encode(
		[]interface{}{ethgo.HexToAddress(params.addressTo)})
	if err != nil {
		return err
	}

	txn := &ethgo.Transaction{
		From:     validatorAccount.Ecdsa.Address(),
		Input:    encoded,
		To:       (*ethgo.Address)(&contracts.ValidatorSetContract),
		GasPrice: sidechainHelper.DefaultGasPrice,
	}

	receipt, err := txRelayer.SendTransaction(txn, validatorAccount.Ecdsa)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("withdraw transaction failed on block %d", receipt.BlockNumber)
	}

	result := &withdrawResult{
		validatorAddress: validatorAccount.Ecdsa.Address().String(),
	}

	foundLog := false

	for _, log := range receipt.Logs {
		if withdrawEventABI.Match(log) {
			event, err := withdrawEventABI.ParseLog(log)
			if err != nil {
				return err
			}

			result.amount = event["amount"].(*big.Int).Uint64()       //nolint:forcetypeassert
			result.withdrawnTo = event["to"].(ethgo.Address).String() //nolint:forcetypeassert
			foundLog = true

			break
		}
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that withdrawal happened")
	}

	outputter.WriteCommandResult(result)

	return nil
}
