package genesis

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:     "genesis",
		Short:   "Generates the genesis configuration file with the passed in parameters",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(genesisCmd)

	setFlags(genesisCmd)
	setLegacyFlags(genesisCmd)
	setRequiredFlags(genesisCmd)

	return genesisCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		dirFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		fmt.Sprintf(
			"the directory for the Polygon Edge genesis data. Default: %s",
			fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		),
	)

	cmd.Flags().StringVar(
		&params.name,
		nameFlag,
		command.DefaultChainName,
		fmt.Sprintf(
			"the name for the chain. Default: %s",
			command.DefaultChainName,
		),
	)

	cmd.Flags().StringVar(
		&params.consensusRaw,
		command.ConsensusFlag,
		string(command.DefaultConsensus),
		fmt.Sprintf(
			"the consensus protocol to be used. Default: %s",
			command.DefaultConsensus,
		),
	)

	cmd.Flags().StringVar(
		&params.validatorPrefixPath,
		ibftValidatorPrefixFlag,
		"",
		"prefix path for validator folder directory. "+
			"Needs to be present if ibft-validator is omitted",
	)

	cmd.Flags().StringArrayVar(
		&params.premine,
		premineFlag,
		[]string{},
		fmt.Sprintf(
			"the premined accounts and balances (format: <address>:<balance>). Default premined balance: %s",
			command.DefaultPremineBalance,
		),
	)

	cmd.Flags().StringArrayVar(
		&params.bootnodes,
		command.BootnodeFlag,
		[]string{},
		"multiAddr URL for p2p discovery bootstrap. This flag can be used multiple times",
	)

	cmd.Flags().StringArrayVar(
		&params.ibftValidatorsRaw,
		ibftValidatorFlag,
		[]string{},
		"addresses to be used as IBFT validators, can be used multiple times. "+
			"Needs to be present if ibft-validators-prefix-path is omitted",
	)

	cmd.Flags().BoolVar(
		&params.isPos,
		posFlag,
		false,
		"the flag indicating that the client should use Proof of Stake IBFT. Defaults to "+
			"Proof of Authority if flag is not provided or false",
	)

	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlag,
		command.DefaultChainID,
		fmt.Sprintf(
			"the ID of the chain. Default: %d",
			command.DefaultChainID,
		),
	)

	cmd.Flags().Uint64Var(
		&params.epochSize,
		epochSizeFlag,
		ibft.DefaultEpochSize,
		fmt.Sprintf(
			"the epoch size for the chain. Default %d",
			ibft.DefaultEpochSize,
		),
	)

	cmd.Flags().Uint64Var(
		&params.blockGasLimit,
		blockGasLimitFlag,
		command.DefaultGenesisGasLimit,
		fmt.Sprintf(
			"the maximum amount of gas used by all transactions in a block. Default: %d",
			command.DefaultGenesisGasLimit,
		),
	)
	cmd.Flags().Uint64Var(
		&params.minNumValidators,
		minValidatorCount,
		1,
		"the minimum number of validators in the validator set for PoS",
	)
	cmd.Flags().Uint64Var(
		&params.maxNumValidators,
		maxValidatorCount,
		common.MaxSafeJSInt,
		"the maximum number of validators in the validator set for PoS",
	)
}

// setLegacyFlags sets the legacy flags to preserve backwards compatibility
// with running partners
func setLegacyFlags(cmd *cobra.Command) {
	// Legacy chainid flag
	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlagLEGACY,
		command.DefaultChainID,
		fmt.Sprintf(
			"the ID of the chain. Default: %d",
			command.DefaultChainID,
		),
	)

	_ = cmd.Flags().MarkHidden(chainIDFlagLEGACY)
}

func setRequiredFlags(cmd *cobra.Command) {
	for _, requiredFlag := range params.getRequiredFlags() {
		_ = cmd.MarkFlagRequired(requiredFlag)
	}
}

func runPreRun(_ *cobra.Command, _ []string) error {
	if err := params.validateFlags(); err != nil {
		return err
	}

	return params.initRawParams()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.generateGenesis(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
