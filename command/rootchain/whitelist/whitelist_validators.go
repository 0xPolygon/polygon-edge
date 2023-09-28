package whitelist

import (
	"fmt"
	"time"

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

var params whitelistParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "whitelist-validators",
		Short:   "whitelist new validators",
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
		&params.privateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		polybftsecrets.PrivateKeyFlagDesc,
	)

	cmd.Flags().StringSliceVar(
		&params.newValidatorAddresses,
		newValidatorAddressesFlag,
		[]string{},
		"account addresses of a possible validators",
	)

	cmd.Flags().StringVar(
		&params.supernetManagerAddress,
		rootHelper.SupernetManagerFlag,
		"",
		rootHelper.SupernetManagerFlagDesc,
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)

	helper.RegisterJSONRPCFlag(cmd)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ecdsaKey, err := rootHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptTimeout(150*time.Millisecond))
	if err != nil {
		return fmt.Errorf("whitelist validator failed. Could not create tx relayer: %w", err)
	}

	whitelistFn := &contractsapi.WhitelistValidatorsCustomSupernetManagerFn{
		Validators_: stringSliceToAddressSlice(params.newValidatorAddresses),
	}

	encoded, err := whitelistFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("whitelist validator failed. Could not abi encode whitelist function: %w", err)
	}

	supernetAddr := ethgo.Address(types.StringToAddress(params.supernetManagerAddress))
	txn := rootHelper.CreateTransaction(ecdsaKey.Address(), &supernetAddr, encoded, nil, true)

	receipt, err := txRelayer.SendTransaction(txn, ecdsaKey)
	if err != nil {
		return fmt.Errorf("whitelist validator failed %w", err)
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("whitelist validator transaction failed on block %d", receipt.BlockNumber)
	}

	var (
		whitelistEvent contractsapi.AddedToWhitelistEvent
		result         = &whitelistResult{}
	)

	for _, log := range receipt.Logs {
		doesMatch, err := whitelistEvent.ParseLog(log)
		if !doesMatch {
			continue
		}

		if err != nil {
			return err
		}

		result.NewValidatorAddresses = append(result.NewValidatorAddresses, whitelistEvent.Validator.String())
	}

	if len(result.NewValidatorAddresses) != len(params.newValidatorAddresses) {
		return fmt.Errorf("whitelist of validators did not pass successfully")
	}

	outputter.WriteCommandResult(result)

	return nil
}

func stringSliceToAddressSlice(addrs []string) []ethgo.Address {
	res := make([]ethgo.Address, len(addrs))
	for indx, addr := range addrs {
		res[indx] = ethgo.Address(types.StringToAddress(addr))
	}

	return res
}
