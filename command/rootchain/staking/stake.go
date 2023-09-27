package staking

import (
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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
		Short:   "Stakes the amount sent for validator on rootchain",
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
		&params.stakeManagerAddr,
		rootHelper.StakeManagerFlag,
		"",
		rootHelper.StakeManagerFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.amount,
		sidechainHelper.AmountFlag,
		"",
		"amount to stake",
	)

	cmd.Flags().Int64Var(
		&params.supernetID,
		supernetIDFlag,
		0,
		"ID of supernet provided by stake manager on supernet registration",
	)

	cmd.Flags().StringVar(
		&params.stakeTokenAddr,
		rootHelper.StakeTokenFlag,
		"",
		rootHelper.StakeTokenFlagDesc,
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

	approveTxn, err := rootHelper.CreateApproveERC20Txn(params.amountValue,
		types.StringToAddress(params.stakeManagerAddr), types.StringToAddress(params.stakeTokenAddr), true)
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

	stakeFn := contractsapi.StakeForStakeManagerFn{
		ID:     new(big.Int).SetInt64(params.supernetID),
		Amount: params.amountValue,
	}

	encoded, err := stakeFn.EncodeAbi()
	if err != nil {
		return err
	}

	stakeManagerAddr := ethgo.Address(types.StringToAddress(params.stakeManagerAddr))

	txn := rootHelper.CreateTransaction(validatorAccount.Ecdsa.Address(), &stakeManagerAddr, encoded, nil, true)

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
