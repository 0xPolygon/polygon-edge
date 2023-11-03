package premine

import (
	"fmt"
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
	params premineParams
)

func GetCommand() *cobra.Command {
	premineCmd := &cobra.Command{
		Use: "premine",
		Short: "Premine native root token to the caller, which determines genesis balances. " +
			"This command is used in case Supernets native token is rootchain originated.",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	helper.RegisterJSONRPCFlag(premineCmd)
	setFlags(premineCmd)

	return premineCmd
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
		&params.privateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		polybftsecrets.PrivateKeyFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.customSupernetManager,
		rootHelper.SupernetManagerFlag,
		"",
		rootHelper.SupernetManagerFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.nativeTokenRoot,
		rootHelper.Erc20TokenFlag,
		"",
		"address of root erc20 native token",
	)

	cmd.Flags().StringVar(
		&params.rootERC20Predicate,
		rootERC20PredicateFlag,
		"",
		"address of root erc20 predicate",
	)

	cmd.Flags().StringVar(
		&params.amount,
		sidechainHelper.AmountFlag,
		"",
		"amount to premine",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ownerKey, err := rootHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptTimeout(150*time.Millisecond))
	if err != nil {
		return err
	}

	approveTxn, err := rootHelper.CreateApproveERC20Txn(
		params.amountValue,
		types.StringToAddress(params.customSupernetManager),
		types.StringToAddress(params.nativeTokenRoot), true)
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, ownerKey)
	if err != nil {
		return err
	}

	if receipt == nil || receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("approve transaction failed on block %d", receipt.BlockNumber)
	}

	premineFn := &contractsapi.AddGenesisBalanceCustomSupernetManagerFn{
		Amount: params.amountValue,
	}

	premineInput, err := premineFn.EncodeAbi()
	if err != nil {
		return err
	}

	supernetManagerAddr := ethgo.Address(types.StringToAddress(params.customSupernetManager))
	txn := rootHelper.CreateTransaction(ownerKey.Address(), &supernetManagerAddr, premineInput, nil, true)

	receipt, err = txRelayer.SendTransaction(txn, ownerKey)
	if err != nil {
		return err
	}

	if receipt == nil || receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("premine transaction failed on block %d", receipt.BlockNumber)
	}

	outputter.WriteCommandResult(&premineResult{
		Address: ownerKey.Address().String(),
		Amount:  params.amountValue,
	})

	return nil
}
