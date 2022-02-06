package genesis

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
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
	setRequiredFlags(genesisCmd)

	return genesisCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		dirFlag,
		fmt.Sprintf("./%s", helper.GenesisFileName),
		fmt.Sprintf(
			"sets the directory for the Polygon Edge genesis data. Default: %s",
			fmt.Sprintf("./%s", helper.GenesisFileName),
		),
	)

	cmd.Flags().StringVar(
		&params.name,
		nameFlag,
		helper.DefaultChainName,
		fmt.Sprintf(
			"sets the name for the chain. Default: %s",
			helper.DefaultChainName,
		),
	)

	cmd.Flags().StringVar(
		&params.consensusRaw,
		consensusFlag,
		string(helper.DefaultConsensus),
		fmt.Sprintf(
			"sets consensusRaw protocol. Default: %s",
			helper.DefaultConsensus,
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
			"sets the premined accounts and balances (format: <address>:<balance>). Default premined balance: %s",
			helper.DefaultPremineBalance,
		),
	)

	cmd.Flags().StringArrayVar(
		&params.bootnodes,
		bootnodeFlag,
		[]string{},
		"multiAddr URL for p2p discovery bootstrap. This flag can be used multiple times",
	)

	cmd.Flags().StringArrayVar(
		&params.ibftValidatorsRaw,
		ibftValidatorFlag,
		[]string{},
		"sets passed in addresses as IBFT validators. "+
			"Needs to be present if ibft-validators-prefix-path is omitted",
	)

	cmd.Flags().BoolVar(
		&params.isPos,
		posFlag,
		false,
		"sets the flag indicating that the client should use Proof of Stake IBFT. Defaults to "+
			"Proof of Authority if flag is not provided or false",
	)

	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlag,
		helper.DefaultChainID,
		fmt.Sprintf(
			"sets the ID of the chain. Default: %d",
			helper.DefaultChainID,
		),
	)

	cmd.Flags().Uint64Var(
		&params.epochSize,
		epochSizeFlag,
		ibft.DefaultEpochSize,
		fmt.Sprintf(
			"sets the epoch size for the chain. Default %d",
			ibft.DefaultEpochSize,
		),
	)

	cmd.Flags().Uint64Var(
		&params.blockGasLimit,
		blockGasLimitFlag,
		helper.GenesisGasLimit,
		fmt.Sprintf(
			"the maximum amount of gas used by all transactions in a block. Default: %d",
			helper.GenesisGasLimit,
		),
	)
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
	outputter := output.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.generateGenesis(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
