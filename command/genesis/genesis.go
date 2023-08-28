package genesis

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis/predeploy"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:     "genesis",
		Short:   "Generates the genesis configuration file with the passed in parameters",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	setFlags(genesisCmd)
	setLegacyFlags(genesisCmd)

	genesisCmd.AddCommand(
		// genesis predeploy
		predeploy.GetCommand(),
	)

	return genesisCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.genesisPath,
		dirFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the directory for the Polygon Edge genesis data",
	)

	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlag,
		command.DefaultChainID,
		"the ID of the chain (only used for IBFT consensus)",
	)

	cmd.Flags().StringVar(
		&params.name,
		nameFlag,
		command.DefaultChainName,
		"the name for the chain",
	)

	cmd.Flags().StringArrayVar(
		&params.premine,
		premineFlag,
		[]string{},
		fmt.Sprintf(
			"the premined accounts and balances (format: <address>[:<balance>]). Default premined balance: %d",
			command.DefaultPremineBalance,
		),
	)

	cmd.Flags().Uint64Var(
		&params.blockGasLimit,
		blockGasLimitFlag,
		command.DefaultGenesisGasLimit,
		"the maximum amount of gas used by all transactions in a block",
	)

	cmd.Flags().StringVar(
		&params.burnContract,
		burnContractFlag,
		"",
		"the burn contract block and address (format: <block>:<address>[:<burn destination>])",
	)

	cmd.Flags().StringArrayVar(
		&params.bootnodes,
		command.BootnodeFlag,
		[]string{},
		"multiAddr URL for p2p discovery bootstrap. This flag can be used multiple times",
	)

	cmd.Flags().StringVar(
		&params.consensusRaw,
		command.ConsensusFlag,
		string(command.DefaultConsensus),
		"the consensus protocol to be used",
	)

	cmd.Flags().Uint64Var(
		&params.epochSize,
		epochSizeFlag,
		ibft.DefaultEpochSize,
		"the epoch size for the chain",
	)

	// IBFT Validators
	{
		cmd.Flags().StringVar(
			&params.rawIBFTValidatorType,
			command.IBFTValidatorTypeFlag,
			string(validators.BLSValidatorType),
			"the type of validators in IBFT",
		)

		cmd.Flags().StringVar(
			&params.validatorPrefixPath,
			command.IBFTValidatorPrefixFlag,
			"",
			"prefix path for validator folder directory. "+
				"Needs to be present if ibft-validator is omitted",
		)

		cmd.Flags().StringArrayVar(
			&params.ibftValidatorsRaw,
			command.IBFTValidatorFlag,
			[]string{},
			"addresses to be used as IBFT validators, can be used multiple times. "+
				"Needs to be present if ibft-validators-prefix-path is omitted",
		)

		// --ibft-validator-prefix-path & --ibft-validator can't be given at same time
		cmd.MarkFlagsMutuallyExclusive(command.IBFTValidatorPrefixFlag, command.IBFTValidatorFlag)
	}

	// PoS
	{
		cmd.Flags().BoolVar(
			&params.isPos,
			posFlag,
			false,
			"the flag indicating that the client should use Proof of Stake IBFT. Defaults to "+
				"Proof of Authority if flag is not provided or false",
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

	// PolyBFT
	{
		cmd.Flags().StringVar(
			&params.validatorsPath,
			validatorsPathFlag,
			"./",
			"root path containing polybft validators secrets",
		)

		cmd.Flags().StringVar(
			&params.validatorsPrefixPath,
			validatorsPrefixFlag,
			defaultValidatorPrefixPath,
			"folder prefix names for polybft validators secrets",
		)

		cmd.Flags().StringArrayVar(
			&params.validators,
			validatorsFlag,
			[]string{},
			"validators defined by user (format: <P2P multi address>:<ECDSA address>:<public BLS key>)",
		)

		cmd.MarkFlagsMutuallyExclusive(validatorsFlag, validatorsPathFlag)
		cmd.MarkFlagsMutuallyExclusive(validatorsFlag, validatorsPrefixFlag)

		cmd.Flags().Uint64Var(
			&params.sprintSize,
			sprintSizeFlag,
			defaultSprintSize,
			"the number of block included into a sprint",
		)

		cmd.Flags().DurationVar(
			&params.blockTime,
			blockTimeFlag,
			defaultBlockTime,
			"the predefined period which determines block creation frequency",
		)

		cmd.Flags().Uint64Var(
			&params.epochReward,
			epochRewardFlag,
			defaultEpochReward,
			"reward size for block sealing",
		)

		// regenesis flag that allows to start from non-empty database
		cmd.Flags().StringVar(
			&params.initialStateRoot,
			trieRootFlag,
			"",
			"trie root from the corresponding triedb",
		)

		cmd.Flags().StringVar(
			&params.nativeTokenConfigRaw,
			nativeTokenConfigFlag,
			"",
			"native token configuration, provided in the following format: "+
				"<name:symbol:decimals count:mintable flag:[mintable token owner address]>",
		)

		cmd.Flags().StringVar(
			&params.rewardTokenCode,
			rewardTokenCodeFlag,
			"",
			"hex encoded reward token byte code",
		)

		cmd.Flags().StringVar(
			&params.rewardWallet,
			rewardWalletFlag,
			"",
			"configuration of reward wallet in format <address:amount>",
		)

		cmd.Flags().Uint64Var(
			&params.blockTimeDrift,
			blockTimeDriftFlag,
			defaultBlockTimeDrift,
			"configuration for block time drift value (in seconds)",
		)
	}

	// Access Control Lists
	{
		cmd.Flags().StringArrayVar(
			&params.contractDeployerAllowListAdmin,
			contractDeployerAllowListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the contract deployer allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.contractDeployerAllowListEnabled,
			contractDeployerAllowListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the contract deployer allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.contractDeployerBlockListAdmin,
			contractDeployerBlockListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the contract deployer block list",
		)

		cmd.Flags().StringArrayVar(
			&params.contractDeployerBlockListEnabled,
			contractDeployerBlockListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the contract deployer block list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsAllowListAdmin,
			transactionsAllowListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the transactions allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsAllowListEnabled,
			transactionsAllowListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the transactions allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsBlockListAdmin,
			transactionsBlockListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the transactions block list",
		)

		cmd.Flags().StringArrayVar(
			&params.transactionsBlockListEnabled,
			transactionsBlockListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the transactions block list",
		)

		cmd.Flags().StringArrayVar(
			&params.bridgeAllowListAdmin,
			bridgeAllowListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the bridge allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.bridgeAllowListEnabled,
			bridgeAllowListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the bridge allow list",
		)

		cmd.Flags().StringArrayVar(
			&params.bridgeBlockListAdmin,
			bridgeBlockListAdminFlag,
			[]string{},
			"list of addresses to use as admin accounts in the bridge block list",
		)

		cmd.Flags().StringArrayVar(
			&params.bridgeBlockListEnabled,
			bridgeBlockListEnabledFlag,
			[]string{},
			"list of addresses to enable by default in the bridge block list",
		)
	}
}

// setLegacyFlags sets the legacy flags to preserve backwards compatibility
// with running partners
func setLegacyFlags(cmd *cobra.Command) {
	// Legacy chainid flag
	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlagLEGACY,
		command.DefaultChainID,
		"the ID of the chain",
	)

	_ = cmd.Flags().MarkHidden(chainIDFlagLEGACY)
}

func preRunCommand(cmd *cobra.Command, _ []string) error {
	if err := params.validateFlags(); err != nil {
		return err
	}

	helper.SetRequiredFlags(cmd, params.getRequiredFlags())

	return params.initRawParams()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	var err error

	if params.isPolyBFTConsensus() {
		err = params.generatePolyBftChainConfig(outputter)
	} else {
		err = params.generateGenesis()
	}

	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
