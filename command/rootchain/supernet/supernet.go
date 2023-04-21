package supernet

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

var params supernetParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "supernet",
		Short:   "Supernet initialization & finalization command",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(registerCmd)

	return registerCmd
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
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
		&params.supernetManagerAddress,
		rootHelper.SupernetManagerFlag,
		"",
		rootHelper.SupernetManagerFlagDesc,
	)

	cmd.Flags().BoolVar(
		&params.finalizeGenesisSet,
		finalizeGenesisSetFlag,
		false,
		"indicates if genesis validator set should be finalized on rootchain",
	)

	cmd.Flags().BoolVar(
		&params.enableStaking,
		enableStakingFlag,
		false,
		"indicates if genesis validator set should be finalized on rootchain",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)

	helper.RegisterJSONRPCFlag(cmd)
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ownerKey, err := rootHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return fmt.Errorf("enlist validator failed: %w", err)
	}

	gasPrice, err := rootHelper.GetGasPriceOnRoot(params.jsonRPC)
	if err != nil {
		return err
	}

	supernetAddr := ethgo.Address(types.StringToAddress(params.supernetManagerAddress))

	if params.finalizeGenesisSet {
		encoded, err := contractsapi.CustomSupernetManager.Abi.Methods["finalizeGenesis"].Encode([]interface{}{})
		if err != nil {
			return err
		}

		txn := &ethgo.Transaction{
			From:     ownerKey.Address(),
			Input:    encoded,
			To:       &supernetAddr,
			GasPrice: gasPrice,
		}

		receipt, err := txRelayer.SendTransaction(txn, ownerKey)
		if err != nil {
			return fmt.Errorf("finalizing genesis validator set failed. Error: %w", err)
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			return fmt.Errorf("finalizing genesis validator set transaction failed on block %d", receipt.BlockNumber)
		}
	}

	if params.enableStaking {
		encoded, err := contractsapi.CustomSupernetManager.Abi.Methods["enableStaking"].Encode([]interface{}{})
		if err != nil {
			return err
		}

		txn := &ethgo.Transaction{
			From:     ownerKey.Address(),
			Input:    encoded,
			To:       &supernetAddr,
			GasPrice: gasPrice,
		}

		receipt, err := txRelayer.SendTransaction(txn, ownerKey)
		if err != nil {
			return fmt.Errorf("enabling staking on supernet manager failed. Error: %w", err)
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			return fmt.Errorf("enable staking transaction failed on block %d", receipt.BlockNumber)
		}
	}

	result := &supernetResult{
		isGenesisSetFinalized: params.finalizeGenesisSet,
		isStakingEnabled:      params.enableStaking,
	}

	outputter.WriteCommandResult(result)

	return nil
}
