package stakemanager

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"
)

var params stakeManagerDeployParams

func GetCommand() *cobra.Command {
	stakeMgrDeployCmd := &cobra.Command{
		Use:     "stake-manager-deploy",
		Short:   "Command for deploying stake manager contract on rootchain",
		PreRunE: preRunCommand,
		RunE:    runCommand,
	}

	setFlags(stakeMgrDeployCmd)

	return stakeMgrDeployCmd
}

func preRunCommand(cmd *cobra.Command, _ []string) error {
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
		&params.genesisPath,
		rootHelper.GenesisPathFlag,
		rootHelper.DefaultGenesisPath,
		rootHelper.GenesisPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.stakeTokenAddress,
		rootHelper.StakeTokenFlag,
		"",
		rootHelper.StakeTokenFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.proxyContractsAdmin,
		rootHelper.ProxyContractsAdminFlag,
		"",
		rootHelper.ProxyContractsAdminDesc,
	)

	cmd.Flags().BoolVar(
		&params.isTestMode,
		rootHelper.TestModeFlag,
		false,
		"indicates if command is run in test mode. If test mode is used contract will be deployed using test account "+
			"and a test stake ERC20 token will be deployed to be used for staking",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)
	cmd.MarkFlagsMutuallyExclusive(rootHelper.TestModeFlag, polybftsecrets.PrivateKeyFlag)
	cmd.MarkFlagsMutuallyExclusive(rootHelper.TestModeFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(rootHelper.TestModeFlag, polybftsecrets.AccountDirFlag)
	cmd.MarkFlagsMutuallyExclusive(rootHelper.TestModeFlag, rootHelper.StakeTokenFlag)

	helper.RegisterJSONRPCFlag(cmd)
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	var (
		deployerKey ethgo.Key
		err         error
	)

	if params.isTestMode {
		deployerKey, err = rootHelper.DecodePrivateKey("")
	} else {
		deployerKey, err = rootHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	}

	if err != nil {
		return fmt.Errorf("failed to get deployer key: %w", err)
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return fmt.Errorf("deploying stake manager failed: %w", err)
	}

	if params.isTestMode {
		// fund deployer so that he can deploy contracts
		deployerAddr := deployerKey.Address()
		txn := rootHelper.CreateTransaction(ethgo.ZeroAddress, &deployerAddr, nil, ethgo.Ether(1), true)

		if _, err = txRelayer.SendTransactionLocal(txn); err != nil {
			return fmt.Errorf("failed to send local transaction: %w", err)
		}
	}

	var (
		stakeManagerAddress types.Address
		stakeTokenAddress   = types.StringToAddress(params.stakeTokenAddress)
		g, ctx              = errgroup.WithContext(cmd.Context())
	)

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// deploy stake manager
			contractAddress, err := deployContract(txRelayer, deployerKey,
				contractsapi.StakeManager, "StakeManager")
			if err != nil {
				return err
			}

			// deploy stake manager proxy
			receipt, err := rootHelper.DeployProxyContract(txRelayer, deployerKey, "StakeManagerProxy",
				types.StringToAddress(params.proxyContractsAdmin), contractAddress)
			if err != nil {
				return err
			}

			if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
				return errors.New("deployment of StakeManagerProxy contract failed")
			}

			outputter.WriteCommandResult(&rootHelper.MessageResult{
				Message: "[STAKEMANAGER - DEPLOY] Successfully deployed StakeManager contract",
			})

			stakeManagerAddress = types.Address(receipt.ContractAddress)

			return nil
		}
	})

	if params.isTestMode {
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// this is only used for testing, we are deploying a test ERC20 token to use for staking
				// this should not be used in production
				// deploy stake manager
				contractAddress, err := deployContract(txRelayer, deployerKey,
					contractsapi.RootERC20, "MockERC20StakeToken")
				if err != nil {
					return err
				}

				outputter.WriteCommandResult(&rootHelper.MessageResult{
					Message: "[STAKEMANAGER - DEPLOY] Successfully deployed MockERC20StakeToken contract",
				})

				stakeTokenAddress = contractAddress

				return nil
			}
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// initialize stake manager
	if err := initializeStakeManager(outputter, txRelayer,
		stakeManagerAddress, stakeTokenAddress, deployerKey); err != nil {
		return fmt.Errorf("could not initialize stake manager contract. Error: %w", err)
	}

	// update stake manager address in genesis
	chainConfig, err := chain.ImportFromFile(params.genesisPath)
	if err != nil {
		return fmt.Errorf("failed to read chain configuration: %w", err)
	}

	consensusConfig, err := polybft.GetPolyBFTConfig(chainConfig)
	if err != nil {
		return fmt.Errorf("failed to retrieve consensus configuration: %w", err)
	}

	if consensusConfig.Bridge == nil {
		consensusConfig.Bridge = &polybft.BridgeConfig{}
	}

	consensusConfig.Bridge.StakeManagerAddr = stakeManagerAddress
	consensusConfig.Bridge.StakeTokenAddr = stakeTokenAddress

	// write updated chain configuration
	chainConfig.Params.Engine[polybft.ConsensusName] = consensusConfig

	if err = helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath); err != nil {
		return fmt.Errorf("failed to save chain configuration bridge data: %w", err)
	}

	return nil
}

// initializeStakeManager invokes initialize function on StakeManager contract
func initializeStakeManager(cmdOutput command.OutputFormatter,
	txRelayer txrelayer.TxRelayer,
	stakeManagerAddress types.Address,
	stakeTokenAddress types.Address,
	deployerKey ethgo.Key) error {
	initFn := &contractsapi.InitializeStakeManagerFn{NewStakingToken: stakeTokenAddress}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return err
	}

	if _, err := rootHelper.SendTransaction(txRelayer, ethgo.Address(stakeManagerAddress),
		input, "StakeManager", deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(&rootHelper.MessageResult{
		Message: "[STAKEMANAGER - DEPLOY] StakeManager contract is initialized",
	})

	return nil
}

func deployContract(txRelayer txrelayer.TxRelayer, deployerKey ethgo.Key,
	artifact *artifact.Artifact, contractName string) (types.Address, error) {
	txn := rootHelper.CreateTransaction(ethgo.ZeroAddress, nil, artifact.Bytecode, nil, true)

	receipt, err := txRelayer.SendTransaction(txn, deployerKey)
	if err != nil {
		return types.ZeroAddress,
			fmt.Errorf("failed sending %s contract deploy transaction: %w", contractName, err)
	}

	if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
		return types.ZeroAddress, fmt.Errorf("deployment of %s contract failed", contractName)
	}

	return types.Address(receipt.ContractAddress), nil
}
