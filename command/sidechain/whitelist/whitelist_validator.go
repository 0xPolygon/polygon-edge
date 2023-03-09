package whitelist

import (
	"fmt"
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
	whitelistFn       = contractsapi.ChildValidatorSet.Abi.Methods["addToWhitelist"]
	whitelistEventABI = contractsapi.ChildValidatorSet.Abi.Events["AddedToWhitelist"]
)

var params whitelistParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "whitelist-validator",
		Short:   "whitelist a new validator",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(registerCmd)

	return registerCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.accountDir,
		polybftsecrets.DataPathFlag,
		"",
		polybftsecrets.DataPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.ConfigFlag,
		"",
		polybftsecrets.ConfigFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.newValidatorAddress,
		newValidatorAddressFlag,
		"",
		"account address of a possible validator",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.DataPathFlag, polybftsecrets.ConfigFlag)
	helper.RegisterJSONRPCFlag(cmd)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ownerAccount, err := sidechainHelper.GetAccount(params.accountDir, params.configPath)
	if err != nil {
		return fmt.Errorf("enlist validator failed: %w", err)
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptTimeout(150*time.Millisecond))
	if err != nil {
		return fmt.Errorf("enlist validator failed: %w", err)
	}

	encoded, err := whitelistFn.Encode([]interface{}{
		[]types.Address{types.StringToAddress(params.newValidatorAddress)},
	})

	txn := &ethgo.Transaction{
		From:     ownerAccount.Ecdsa.Address(),
		Input:    encoded,
		To:       (*ethgo.Address)(&contracts.ValidatorSetContract),
		GasPrice: sidechainHelper.DefaultGasPrice,
	}

	receipt, err := txRelayer.SendTransaction(txn, ownerAccount.Ecdsa)
	if err != nil {
		return fmt.Errorf("enlist validator failed %w", err)
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("enlist validator transaction failed on block %d", receipt.BlockNumber)
	}

	result := &enlistResult{}
	foundLog := false

	for _, log := range receipt.Logs {
		if whitelistEventABI.Match(log) {
			event, err := whitelistEventABI.ParseLog(log)
			if err != nil {
				return err
			}

			result.newValidatorAddress = event["validator"].(ethgo.Address).String() //nolint:forcetypeassert
			foundLog = true

			break
		}
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that enlistment happened")
	}

	outputter.WriteCommandResult(result)

	return nil
}
