package registration

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

var params registerParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "register-validator",
		Short:   "registers a whitelisted validator to supernet manager on rootchain",
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
		&params.supernetManagerAddress,
		rootHelper.SupernetManagerFlag,
		"",
		rootHelper.SupernetManagerFlagDesc,
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

	rootChainID, err := txRelayer.Client().Eth().ChainID()
	if err != nil {
		return err
	}

	newValidatorAccount, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	koskSignature, err := signer.MakeKOSKSignature(
		newValidatorAccount.Bls, newValidatorAccount.Address(),
		rootChainID.Int64(), signer.DomainValidatorSet, types.StringToAddress(params.supernetManagerAddress))
	if err != nil {
		return err
	}

	receipt, err := registerValidator(txRelayer, newValidatorAccount, koskSignature)
	if err != nil {
		return err
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return errors.New("register validator transaction failed")
	}

	result := &registerResult{}
	foundLog := false

	var validatorRegisteredEvent contractsapi.ValidatorRegisteredEvent
	for _, log := range receipt.Logs {
		doesMatch, err := validatorRegisteredEvent.ParseLog(log)
		if err != nil {
			return err
		}

		if !doesMatch {
			continue
		}

		koskSignatureRaw, err := koskSignature.Marshal()
		if err != nil {
			return err
		}

		result.koskSignature = hex.EncodeToString(koskSignatureRaw)
		result.validatorAddress = validatorRegisteredEvent.Validator.String()

		foundLog = true

		break
	}

	if !foundLog {
		return fmt.Errorf("could not find an appropriate log in receipt that registration happened")
	}

	outputter.WriteCommandResult(result)

	return nil
}

func registerValidator(sender txrelayer.TxRelayer, account *wallet.Account,
	signature *bls.Signature) (*ethgo.Receipt, error) {
	sigMarshal, err := signature.ToBigInt()
	if err != nil {
		return nil, fmt.Errorf("register validator failed: %w", err)
	}

	registerFn := &contractsapi.RegisterCustomSupernetManagerFn{
		Signature: sigMarshal,
		Pubkey:    account.Bls.PublicKey().ToBigInt(),
	}

	input, err := registerFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("register validator failed: %w", err)
	}

	supernetAddr := ethgo.Address(types.StringToAddress(params.supernetManagerAddress))
	txn := rootHelper.CreateTransaction(ethgo.ZeroAddress, &supernetAddr, input, nil, true)

	return sender.SendTransaction(txn, account.Ecdsa)
}
