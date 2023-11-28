package withdraw

import (
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	"github.com/0xPolygon/polygon-edge/command/helper"
	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

var params withdrawParams

func GetCommand() *cobra.Command {
	withdrawCmd := &cobra.Command{
		Use:     "withdraw-root",
		Short:   "Withdraws sender's withdrawable amount to specified address on the root chain",
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

	cmd.Flags().StringVar(
		&params.stakeManagerAddr,
		bridgeHelper.StakeManagerFlag,
		"",
		bridgeHelper.StakeManagerFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.amount,
		validatorHelper.AmountFlag,
		"",
		"amount to withdraw",
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

	validatorAccount, err := validatorHelper.GetAccount(params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptTimeout(150*time.Millisecond))
	if err != nil {
		return err
	}

	encoded, err := contractsapi.StakeManager.Abi.Methods["withdraw"].Encode([]interface{}{})
	if err != nil {
		return err
	}

	receiver := (*ethgo.Address)(&contracts.StakeManagerContract)
	txn := bridgeHelper.CreateTransaction(validatorAccount.Ecdsa.Address(), receiver, encoded, nil, false)

	receipt, err := txRelayer.SendTransaction(txn, validatorAccount.Ecdsa)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("withdraw transaction failed on block %d", receipt.BlockNumber)
	}

	result := &withdrawResult{
		ValidatorAddress: validatorAccount.Ecdsa.Address().String(),
	}

	var (
		withdrawalEvent contractsapi.StakeWithdrawnEvent
		foundLog        bool
	)

	for _, log := range receipt.Logs {
		doesMatch, err := withdrawalEvent.ParseLog(log)
		if !doesMatch {
			continue
		}

		if err != nil {
			return err
		}

		result.Amount = withdrawalEvent.Amount.Uint64()
		foundLog = true

		break
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that withdrawal happened")
	}

	outputter.WriteCommandResult(result)

	return nil
}
