package fund

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params fundParams
)

// GetCommand returns the rootchain fund command
func GetCommand() *cobra.Command {
	rootchainFundCmd := &cobra.Command{
		Use:     "fund",
		Short:   "Fund validator account with given tokens amount",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	setFlags(rootchainFundCmd)

	return rootchainFundCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dataDir,
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.amount,
		amountFlag,
		"",
		"tokens amount which is funded to validator on a root chain",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the rootchain JSON RPC endpoint",
	)

	cmd.Flags().StringVar(
		&params.nativeRootTokenAddr,
		helper.NativeRootTokenFlag,
		"",
		helper.NativeRootTokenFlagDesc,
	)

	cmd.Flags().BoolVar(
		&params.mintRootToken,
		mintRootTokenFlag,
		false,
		"indicates if root token deployer should mint root tokens to given validators",
	)

	cmd.Flags().StringVar(
		&params.deployerPrivateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		polybftsecrets.PrivateKeyFlagDesc,
	)

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
}

func preRunCommand(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize tx relayer: %w", err))

		return
	}

	if err := params.initSecretsManager(); err != nil {
		outputter.SetError(err)

		return
	}

	validatorAddr, err := params.getValidatorAccount()
	if err != nil {
		outputter.SetError(err)

		return
	}

	gasPrice, err := txRelayer.Client().Eth().GasPrice()
	if err != nil {
		outputter.SetError(err)

		return
	}

	fundAddr := ethgo.Address(validatorAddr)
	txn := &ethgo.Transaction{
		To:       &fundAddr,
		Value:    params.amountValue,
		GasPrice: gasPrice,
	}

	receipt, err := txRelayer.SendTransactionLocal(txn)
	if err != nil {
		outputter.SetError(err)

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		_, _ = outputter.Write([]byte(fmt.Sprintf("failed to fund validator '%s'", validatorAddr.String())))

		return
	}

	result := &result{
		ValidatorAddr: validatorAddr,
		TxHash:        types.Hash(receipt.TransactionHash),
	}

	if params.mintRootToken {
		depositorKey, err := helper.GetRootchainPrivateKey(params.deployerPrivateKey)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to initialize depositor private key: %w", err))

			return
		}

		rootTokenAddr := types.StringToAddress(params.nativeRootTokenAddr)

		// mint tokens to validator, so he is able to send them
		mintTxn, err := helper.CreateMintTxn(validatorAddr, rootTokenAddr, params.amountValue)
		if err != nil {
			outputter.SetError(fmt.Errorf("mint transaction creation failed for validator: %s. err: %w", validatorAddr, err))

			return
		}

		mintTxn.GasPrice, err = txRelayer.Client().Eth().GasPrice()
		if err != nil {
			outputter.SetError(err)

			return
		}

		receipt, err := txRelayer.SendTransaction(mintTxn, depositorKey)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to send mint transaction to depositor %s. err: %w", validatorAddr, err))

			return
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			outputter.SetError(fmt.Errorf("failed to mint tokens to depositor %s", validatorAddr))

			return
		}

		result.IsMinted = true
	}

	outputter.SetCommandResult(command.Results([]command.CommandResult{result}))
}
