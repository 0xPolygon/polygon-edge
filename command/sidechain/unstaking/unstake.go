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

var (
	params             unstakeParams
	unstakeEventABI    = contractsapi.ChildValidatorSet.Abi.Events["Unstaked"]
	undelegateEventABI = contractsapi.ChildValidatorSet.Abi.Events["Undelegated"]
)

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

	cmd.Flags().BoolVar(
		&params.self,
		sidechainHelper.SelfFlag,
		false,
		"indicates if its a self unstake action",
	)

	cmd.Flags().Uint64Var(
		&params.amount,
		sidechainHelper.AmountFlag,
		0,
		"amount to unstake or undelegate amount from validator",
	)

	cmd.Flags().StringVar(
		&params.undelegateAddress,
		undelegateAddressFlag,
		"",
		"account address from which amount will be undelegated",
	)

	cmd.MarkFlagsMutuallyExclusive(sidechainHelper.SelfFlag, undelegateAddressFlag)
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

	var encoded []byte
	if params.self {
		encoded, err = contractsapi.ChildValidatorSet.Abi.Methods["unstake"].Encode([]interface{}{params.amount})
		if err != nil {
			return err
		}
	} else {
		encoded, err = contractsapi.ChildValidatorSet.Abi.Methods["undelegate"].Encode(
			[]interface{}{ethgo.HexToAddress(params.undelegateAddress), params.amount})
		if err != nil {
			return err
		}
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
		return fmt.Errorf("unstake transaction failed on block %d", receipt.BlockNumber)
	}

	result := &unstakeResult{
		validatorAddress: validatorAccount.Ecdsa.Address().String(),
	}

	foundLog := false

	// check the logs to check for the result
	for _, log := range receipt.Logs {
		if unstakeEventABI.Match(log) {
			event, err := unstakeEventABI.ParseLog(log)
			if err != nil {
				return err
			}

			result.isSelfUnstake = true
			result.amount = event["amount"].(*big.Int).Uint64() //nolint:forcetypeassert

			foundLog = true

			break
		} else if undelegateEventABI.Match(log) {
			event, err := undelegateEventABI.ParseLog(log)
			if err != nil {
				return err
			}

			result.amount = event["amount"].(*big.Int).Uint64()                  //nolint:forcetypeassert
			result.undelegatedFrom = event["validator"].(ethgo.Address).String() //nolint:forcetypeassert

			foundLog = true

			break
		}
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that unstake or undelegate happened")
	}

	outputter.WriteCommandResult(result)

	return nil
}
