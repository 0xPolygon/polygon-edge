package registration

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/command"
	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	"github.com/0xPolygon/polygon-edge/command/helper"
	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
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
		&params.amount,
		polybftsecrets.AmountFlag,
		"0",
		polybftsecrets.AmountFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.stakeToken,
		polybftsecrets.StakeTokenFlag,
		contracts.NativeERC20TokenContract.String(),
		polybftsecrets.StakeTokenFlagDesc,
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

	validatorAccount, err := validatorHelper.GetAccount(params.accountDir, params.accountConfig)
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

	if params.amountValue.Cmp(big.NewInt(0)) > 0 {
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
	}

	koskSignature, err := signer.MakeKOSKSignature(
		validatorAccount.Bls, validatorAccount.Address(),
		rootChainID.Int64(), signer.DomainValidatorSet, contracts.StakeManagerContract)
	if err != nil {
		return err
	}

	receipt, err := registerValidator(txRelayer, validatorAccount, koskSignature)
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

		result.KoskSignature = hex.EncodeToString(koskSignatureRaw)
		result.ValidatorAddress = validatorRegisteredEvent.Validator.String()
		result.Amount = validatorRegisteredEvent.Amount

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

	registerFn := &contractsapi.RegisterStakeManagerFn{
		Signature:   sigMarshal,
		Pubkey:      account.Bls.PublicKey().ToBigInt(),
		StakeAmount: params.amountValue,
	}

	input, err := registerFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("register validator failed: %w", err)
	}

	stakeManagerAddr := ethgo.Address(contracts.StakeManagerContract)
	txn := bridgeHelper.CreateTransaction(ethgo.ZeroAddress, &stakeManagerAddr, input, nil, true)

	return sender.SendTransaction(txn, account.Ecdsa)
}
