package staking

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

var (
	params stakeParams
)

func GetCommand() *cobra.Command {
	stakeCmd := &cobra.Command{
		Use:     "stake",
		Short:   "Stakes the amount sent for validator",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	helper.RegisterJSONRPCFlag(stakeCmd)
	setFlags(stakeCmd)

	return stakeCmd
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
		&params.amount,
		validatorHelper.AmountFlag,
		"",
		"amount to stake",
	)

	cmd.Flags().StringVar(
		&params.stakeToken,
		polybftsecrets.StakeTokenFlag,
		contracts.NativeERC20TokenContract.String(),
		polybftsecrets.StakeTokenFlagDesc,
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

	validatorAccount, err := validatorHelper.GetAccount(params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptTimeout(150*time.Millisecond))
	if err != nil {
		return err
	}

	approveTxn, err := bridgeHelper.CreateApproveERC20Txn(params.amountValue,
		contracts.StakeManagerContract, params.stakeTokenAddr, true)
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, validatorAccount.Ecdsa)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("approve transaction failed on block %d", receipt.BlockNumber)
	}

	stakeFn := contractsapi.StakeStakeManagerFn{
		Amount: params.amountValue,
	}

	encoded, err := stakeFn.EncodeAbi()
	if err != nil {
		return err
	}

	stakeManagerAddr := ethgo.Address(contracts.StakeManagerContract)

	txn := bridgeHelper.CreateTransaction(validatorAccount.Ecdsa.Address(), &stakeManagerAddr, encoded, nil, true)

	receipt, err = txRelayer.SendTransaction(txn, validatorAccount.Ecdsa)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("staking transaction failed on block %d", receipt.BlockNumber)
	}

	result := &stakeResult{
		ValidatorAddress: validatorAccount.Ecdsa.Address().String(),
	}

	var (
		stakeAddedEvent contractsapi.StakeAddedEvent
		foundLog        bool
	)

	// check the logs to check for the result
	for _, log := range receipt.Logs {
		doesMatch, err := stakeAddedEvent.ParseLog(log)
		if err != nil {
			return err
		}

		if !doesMatch {
			continue
		}

		result.Amount = stakeAddedEvent.Amount
		result.ValidatorAddress = stakeAddedEvent.Validator.String()
		foundLog = true

		break
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that stake happened")
	}

	outputter.WriteCommandResult(result)

	return nil
}
