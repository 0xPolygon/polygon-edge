package premine

import (
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	"github.com/0xPolygon/polygon-edge/command/helper"
	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

var (
	params premineParams
)

func GetCommand() *cobra.Command {
	premineCmd := &cobra.Command{
		Use: "premine",
		Short: "Premine native root token to the caller, which determines genesis balances. " +
			"This command is used in case Blade native token is rootchain originated.",
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
		&params.nativeTokenRoot,
		bridgeHelper.Erc20TokenFlag,
		"",
		"address of root erc20 native token",
	)

	cmd.Flags().StringVar(
		&params.bladeManager,
		bridgeHelper.BladeManagerFlag,
		"",
		bridgeHelper.BladeManagerFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.premineAmount,
		premineAmountFlag,
		"",
		"amount to premine as a non-staked balance",
	)

	cmd.Flags().StringVar(
		&params.stakedAmount,
		stakedAmountFlag,
		"",
		"amount to premine as a staked balance",
	)

	cmd.Flags().Uint64Var(
		&params.txTimeout,
		bridgeHelper.TxTimeoutFlag,
		5000,
		"timeout for receipts in milliseconds",
	)

	cmd.Flags().Uint64Var(
		&params.txPollFreq,
		bridgeHelper.TxPollFreqFlag,
		150,
		"frequency in milliseconds for poll transactions",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)
	_ = cmd.MarkFlagRequired(bridgeHelper.BladeManagerFlag)
	_ = cmd.MarkFlagRequired(bridgeHelper.Erc20TokenFlag)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ownerKey, err := bridgeHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptsPollFreq(time.Duration(params.txPollFreq*uint64(time.Millisecond))),
		txrelayer.WithReceiptsTimeout(time.Duration(params.txTimeout*uint64(time.Millisecond))))
	if err != nil {
		return err
	}

	approveTxn, err := bridgeHelper.CreateApproveERC20Txn(
		new(big.Int).Add(params.premineAmountValue, params.stakedValue),
		params.bladeManagerAddr,
		params.nativeTokenRootAddr, true)
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, ownerKey)
	if err != nil {
		return fmt.Errorf("approve transaction failed to be sent. %w", err)
	}

	if receipt == nil || receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("approve transaction failed on block %d", receipt.BlockNumber)
	}

	premineFn := &contractsapi.AddGenesisBalanceBladeManagerFn{
		NonStakeAmount: params.premineAmountValue,
		StakeAmount:    params.stakedValue,
	}

	premineInput, err := premineFn.EncodeAbi()
	if err != nil {
		return err
	}

	bladeManagerAddr := types.StringToAddress(params.bladeManager)
	txn := bridgeHelper.CreateTransaction(ownerKey.Address(), &bladeManagerAddr, premineInput, nil, false)

	receipt, err = txRelayer.SendTransaction(txn, ownerKey)
	if err != nil {
		return fmt.Errorf("premine transaction failed to be sent. %w", err)
	}

	if receipt == nil || receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("premine transaction failed on block %d", receipt.BlockNumber)
	}

	outputter.WriteCommandResult(&premineResult{
		Address:         ownerKey.Address().String(),
		NonStakedAmount: params.premineAmountValue,
		StakedAmount:    params.stakedValue,
	})

	return nil
}
