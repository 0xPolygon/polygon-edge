package validators

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	params          validatorInfoParams
	getValidatorABI = abi.MustNewMethod("function getValidator(address)" +
		" returns (tuple(uint256[4] blsKey, uint256 stake, uint256 totalStake," +
		"uint256 commission, uint256 withdrawableRewards, bool active))")
	validatorABI = abi.MustNewType("tuple(uint256[4] blsKey, uint256 stake, uint256 totalStake," +
		"uint256 commission, uint256 withdrawableRewards, bool active)")
)

func GetCommand() *cobra.Command {
	validatorInfoCmd := &cobra.Command{
		Use:     "validator-info",
		Short:   "Gets validator info",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	helper.RegisterJSONRPCFlag(validatorInfoCmd)
	setFlags(validatorInfoCmd)

	return validatorInfoCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.accountDir,
		sidechainHelper.AccountDirFlag,
		"",
		"the directory path where validator key is stored",
	)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	validatorAccount, err := sidechainHelper.GetAccountFromDir(params.accountDir)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return err
	}

	encode, err := getValidatorABI.Encode([]interface{}{validatorAccount.Ecdsa.Address()})
	if err != nil {
		return err
	}

	response, err := txRelayer.Call(ethgo.Address(contracts.SystemCaller),
		ethgo.Address(contracts.ValidatorSetContract), encode)
	if err != nil {
		return err
	}

	byteResponse, err := hex.DecodeHex(response)
	if err != nil {
		return fmt.Errorf("unable to decode hex response, %w", err)
	}

	decoded, err := validatorABI.Decode(byteResponse)
	if err != nil {
		return err
	}

	decodedMap, ok := decoded.(map[string]interface{})
	if !ok {
		return fmt.Errorf("could not convert decoded result to map")
	}

	outputter.WriteCommandResult(&validatorsInfoResult{
		address:             validatorAccount.Ecdsa.Address().String(),
		stake:               decodedMap["stake"].(*big.Int).Uint64(),               //nolint:forcetypeassert
		totalStake:          decodedMap["totalStake"].(*big.Int).Uint64(),          //nolint:forcetypeassert
		commission:          decodedMap["commission"].(*big.Int).Uint64(),          //nolint:forcetypeassert
		withdrawableRewards: decodedMap["withdrawableRewards"].(*big.Int).Uint64(), //nolint:forcetypeassert
	})

	return nil
}
