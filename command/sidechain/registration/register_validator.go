package registration

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

var (
	stakeManager         = contracts.ValidatorSetContract
	stakeFn              = contractsapi.ChildValidatorSet.Abi.Methods["stake"]
	newValidatorEventABI = contractsapi.ChildValidatorSet.Abi.Events["NewValidator"]
	stakeEventABI        = contractsapi.ChildValidatorSet.Abi.Events["Staked"]
)

var params registerParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "register-validator",
		Short:   "Registers and stake an enlisted validator",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(registerCmd)

	return registerCmd
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
		&params.stake,
		stakeFlag,
		"",
		"stake represents amount which is going to be staked by the new validator account",
	)

	cmd.Flags().Int64Var(
		&params.chainID,
		chainIDFlag,
		command.DefaultChainID,
		"the ID of the chain",
	)

	helper.RegisterJSONRPCFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountConfigFlag, polybftsecrets.AccountDirFlag)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	secretsManager, err := polybftsecrets.GetSecretsManager(params.accountDir, params.accountConfig, true)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return err
	}

	newValidatorAccount, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	sRaw, err := secretsManager.GetSecret(secrets.ValidatorBLSSignature)
	if err != nil {
		return err
	}

	sb, err := hex.DecodeString(string(sRaw))
	if err != nil {
		return err
	}

	blsSignature, err := bls.UnmarshalSignature(sb)
	if err != nil {
		return err
	}

	receipt, err := registerValidator(txRelayer, newValidatorAccount, blsSignature)
	if err != nil {
		return err
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return errors.New("register validator transaction failed")
	}

	result := &registerResult{}
	foundLog := false

	for _, log := range receipt.Logs {
		if newValidatorEventABI.Match(log) {
			event, err := newValidatorEventABI.ParseLog(log)
			if err != nil {
				return err
			}

			result.validatorAddress = event["validator"].(ethgo.Address).String() //nolint:forcetypeassert
			result.stakeResult = "No stake parameters have been submitted"
			result.amount = 0
			foundLog = true

			break
		}
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that registration happened")
	}

	if params.stake != "" {
		receipt, err = stake(txRelayer, newValidatorAccount)
		if err != nil {
			result.stakeResult = fmt.Sprintf("Failed to execute stake transaction: %s", err.Error())
		} else {
			populateStakeResults(receipt, result)
		}
	}

	outputter.WriteCommandResult(result)

	return nil
}

func stake(sender txrelayer.TxRelayer, account *wallet.Account) (*ethgo.Receipt, error) {
	if stakeFn == nil {
		return nil, errors.New("failed to create stake ABI function")
	}

	input, err := stakeFn.Encode([]interface{}{})
	if err != nil {
		return nil, err
	}

	stake, err := types.ParseUint256orHex(&params.stake)
	if err != nil {
		return nil, err
	}

	txn := &ethgo.Transaction{
		Input: input,
		To:    (*ethgo.Address)(&stakeManager),
		Value: stake,
	}

	return sender.SendTransaction(txn, account.Ecdsa)
}

func populateStakeResults(receipt *ethgo.Receipt, result *registerResult) {
	if receipt.Status != uint64(types.ReceiptSuccess) {
		result.stakeResult = "Stake transaction failed"

		return
	}

	// check the logs to verify stake
	for _, log := range receipt.Logs {
		if stakeEventABI.Match(log) {
			event, err := stakeEventABI.ParseLog(log)
			if err != nil {
				result.stakeResult = "Failed to parse stake log"

				return
			}

			result.amount = event["amount"].(*big.Int).Uint64() //nolint:forcetypeassert
			result.stakeResult = "Stake succeeded"

			return
		}
	}

	result.stakeResult = "Could not find an appropriate log in receipt that stake happened"
}

func registerValidator(sender txrelayer.TxRelayer, account *wallet.Account,
	signature *bls.Signature) (*ethgo.Receipt, error) {
	sigMarshal, err := signature.ToBigInt()
	if err != nil {
		return nil, fmt.Errorf("register validator failed: %w", err)
	}

	registerFn := &contractsapi.RegisterFunction{
		Signature: sigMarshal,
		Pubkey:    account.Bls.PublicKey().ToBigInt(),
	}

	input, err := registerFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("register validator failed: %w", err)
	}

	txn := &ethgo.Transaction{
		Input: input,
		To:    (*ethgo.Address)(&stakeManager),
	}

	return sender.SendTransaction(txn, account.Ecdsa)
}
