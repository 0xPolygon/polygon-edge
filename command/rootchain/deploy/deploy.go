package deploy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	contractsDeploymentTitle = "[ROOTCHAIN - CONTRACTS DEPLOYMENT]"
	ProxySufix               = "Proxy"

	stateSenderName                   = "StateSender"
	checkpointManagerName             = "CheckpointManager"
	blsName                           = "BLS"
	bn256G2Name                       = "BN256G2"
	exitHelperName                    = "ExitHelper"
	rootERC20PredicateName            = "RootERC20Predicate"
	childERC20MintablePredicateName   = "ChildERC20MintablePredicate"
	rootERC20Name                     = "RootERC20"
	erc20TemplateName                 = "ERC20Template"
	rootERC721PredicateName           = "RootERC721Predicate"
	childERC721MintablePredicateName  = "ChildERC721MintablePredicate"
	erc721TemplateName                = "ERC721Template"
	rootERC1155PredicateName          = "RootERC1155Predicate"
	childERC1155MintablePredicateName = "ChildERC1155MintablePredicate"
	erc1155TemplateName               = "ERC1155Template"
	customSupernetManagerName         = "CustomSupernetManager"
	stakeManagerName                  = "StakeManager"
)

var (
	// params are the parameters of CLI command
	params deployParams

	// consensusCfg contains consensus protocol configuration parameters
	consensusCfg polybft.PolyBFTConfig

	// metadataPopulatorMap maps rootchain contract names to callback
	// which populates appropriate field in the RootchainMetadata
	metadataPopulatorMap = map[string]func(*polybft.RootchainConfig, types.Address){
		stateSenderName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.StateSenderAddress = addr
		},
		getProxyNameForImpl(checkpointManagerName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.CheckpointManagerAddress = addr
		},
		getProxyNameForImpl(blsName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.BLSAddress = addr
		},
		getProxyNameForImpl(bn256G2Name): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.BN256G2Address = addr
		},
		getProxyNameForImpl(exitHelperName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ExitHelperAddress = addr
		},
		getProxyNameForImpl(rootERC20PredicateName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC20PredicateAddress = addr
		},
		getProxyNameForImpl(childERC20MintablePredicateName): func(
			rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ChildMintableERC20PredicateAddress = addr
		},
		rootERC20Name: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootNativeERC20Address = addr
		},
		erc20TemplateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ChildERC20Address = addr
		},
		getProxyNameForImpl(rootERC721PredicateName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC721PredicateAddress = addr
		},
		getProxyNameForImpl(childERC721MintablePredicateName): func(
			rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ChildMintableERC721PredicateAddress = addr
		},
		erc721TemplateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ChildERC721Address = addr
		},
		getProxyNameForImpl(rootERC1155PredicateName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC1155PredicateAddress = addr
		},
		getProxyNameForImpl(childERC1155MintablePredicateName): func(
			rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ChildMintableERC1155PredicateAddress = addr
		},
		erc1155TemplateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ChildERC1155Address = addr
		},
		getProxyNameForImpl(customSupernetManagerName): func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.CustomSupernetManagerAddress = addr
		},
	}

	// initializersMap maps rootchain contract names to initializer function callbacks
	initializersMap = map[string]func(command.OutputFormatter, txrelayer.TxRelayer,
		*polybft.RootchainConfig, ethgo.Key) error{
		getProxyNameForImpl(customSupernetManagerName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			initParams := &contractsapi.InitializeCustomSupernetManagerFn{
				NewStakeManager:       config.StakeManagerAddress,
				NewBls:                config.BLSAddress,
				NewStateSender:        config.StateSenderAddress,
				NewMatic:              types.StringToAddress(params.stakeTokenAddr),
				NewChildValidatorSet:  contracts.ValidatorSetContract,
				NewExitHelper:         config.ExitHelperAddress,
				NewDomain:             signer.DomainValidatorSetString,
				NewRootERC20Predicate: config.RootERC20PredicateAddress,
			}

			return initContract(fmt, relayer, initParams,
				config.CustomSupernetManagerAddress, customSupernetManagerName, key)
		},
		getProxyNameForImpl(exitHelperName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			inputParams := &contractsapi.InitializeExitHelperFn{
				NewCheckpointManager: config.CheckpointManagerAddress,
			}

			return initContract(fmt, relayer, inputParams, config.ExitHelperAddress, exitHelperName, key)
		},
		getProxyNameForImpl(rootERC20PredicateName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {

			inputParams := &contractsapi.InitializeRootERC20PredicateFn{
				NewStateSender:         config.StateSenderAddress,
				NewExitHelper:          config.ExitHelperAddress,
				NewChildERC20Predicate: contracts.ChildERC20PredicateContract,
				NewChildTokenTemplate:  contracts.ChildERC20Contract,
				// map root native token address should be non-zero only if native token is non-mintable on a childchain
				NewNativeTokenRoot: config.RootNativeERC20Address,
			}

			return initContract(fmt, relayer, inputParams,
				config.RootERC20PredicateAddress, rootERC20PredicateName, key)
		},
		getProxyNameForImpl(childERC20MintablePredicateName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			initParams := &contractsapi.InitializeChildMintableERC20PredicateFn{
				NewStateSender:        config.StateSenderAddress,
				NewExitHelper:         config.ExitHelperAddress,
				NewRootERC20Predicate: contracts.RootMintableERC20PredicateContract,
				NewChildTokenTemplate: config.ChildERC20Address,
			}

			return initContract(fmt, relayer, initParams,
				config.ChildMintableERC20PredicateAddress, childERC20MintablePredicateName, key)
		},
		getProxyNameForImpl(rootERC721PredicateName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			initParams := &contractsapi.InitializeRootERC721PredicateFn{
				NewStateSender:          config.StateSenderAddress,
				NewExitHelper:           config.ExitHelperAddress,
				NewChildERC721Predicate: contracts.ChildERC721PredicateContract,
				NewChildTokenTemplate:   contracts.ChildERC721Contract,
			}

			return initContract(fmt, relayer, initParams,
				config.RootERC721PredicateAddress, rootERC721PredicateName, key)
		},
		getProxyNameForImpl(childERC721MintablePredicateName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			initParams := &contractsapi.InitializeChildMintableERC721PredicateFn{
				NewStateSender:         config.StateSenderAddress,
				NewExitHelper:          config.ExitHelperAddress,
				NewRootERC721Predicate: contracts.RootMintableERC721PredicateContract,
				NewChildTokenTemplate:  config.ChildERC721Address,
			}

			return initContract(fmt, relayer, initParams,
				config.ChildMintableERC721PredicateAddress, childERC721MintablePredicateName, key)
		},
		getProxyNameForImpl(rootERC1155PredicateName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			initParams := &contractsapi.InitializeRootERC1155PredicateFn{
				NewStateSender:           config.StateSenderAddress,
				NewExitHelper:            config.ExitHelperAddress,
				NewChildERC1155Predicate: contracts.ChildERC1155PredicateContract,
				NewChildTokenTemplate:    contracts.ChildERC1155Contract,
			}

			return initContract(fmt, relayer, initParams,
				config.RootERC1155PredicateAddress, rootERC1155PredicateName, key)
		},
		getProxyNameForImpl(childERC1155MintablePredicateName): func(fmt command.OutputFormatter,
			relayer txrelayer.TxRelayer,
			config *polybft.RootchainConfig,
			key ethgo.Key) error {
			initParams := &contractsapi.InitializeChildMintableERC1155PredicateFn{
				NewStateSender:          config.StateSenderAddress,
				NewExitHelper:           config.ExitHelperAddress,
				NewRootERC1155Predicate: contracts.RootMintableERC1155PredicateContract,
				NewChildTokenTemplate:   config.ChildERC1155Address,
			}

			return initContract(fmt, relayer, initParams,
				config.ChildMintableERC1155PredicateAddress, childERC1155MintablePredicateName, key)
		},
	}
)

type deploymentResultInfo struct {
	RootchainCfg   *polybft.RootchainConfig
	SupernetID     int64
	CommandResults []command.CommandResult
}

// GetCommand returns the rootchain deploy command
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy",
		Short:   "Deploys and initializes required smart contracts on the rootchain",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	cmd.Flags().StringVar(
		&params.genesisPath,
		helper.GenesisPathFlag,
		helper.DefaultGenesisPath,
		helper.GenesisPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.deployerKey,
		deployerKeyFlag,
		"",
		"hex-encoded private key of the account which deploys rootchain contracts",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the JSON RPC rootchain IP address",
	)

	cmd.Flags().StringVar(
		&params.rootERC20TokenAddr,
		erc20AddrFlag,
		"",
		"existing root chain root native token address",
	)

	cmd.Flags().BoolVar(
		&params.isTestMode,
		helper.TestModeFlag,
		false,
		"test indicates whether rootchain contracts deployer is hardcoded test account"+
			" (otherwise provided secrets are used to resolve deployer account)",
	)

	cmd.Flags().StringVar(
		&params.stakeTokenAddr,
		helper.StakeTokenFlag,
		"",
		helper.StakeTokenFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.stakeManagerAddr,
		helper.StakeManagerFlag,
		"",
		helper.StakeManagerFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.proxyContractsAdmin,
		helper.ProxyContractsAdminFlag,
		"",
		helper.ProxyContractsAdminDesc,
	)

	cmd.MarkFlagsMutuallyExclusive(helper.TestModeFlag, deployerKeyFlag)
	_ = cmd.MarkFlagRequired(helper.StakeManagerFlag)
	_ = cmd.MarkFlagRequired(helper.StakeTokenFlag)

	return cmd
}

func preRunCommand(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	outputter.WriteCommandResult(&helper.MessageResult{
		Message: fmt.Sprintf("%s started... Rootchain JSON RPC address %s.", contractsDeploymentTitle, params.jsonRPCAddress),
	})

	chainConfig, err := chain.ImportFromFile(params.genesisPath)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to read chain configuration: %w", err))

		return
	}

	client, err := jsonrpc.NewClient(params.jsonRPCAddress)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize JSON RPC client for provided IP address: %s: %w",
			params.jsonRPCAddress, err))

		return
	}

	if consensusCfg.Bridge != nil {
		code, err := client.Eth().GetCode(ethgo.Address(consensusCfg.Bridge.StateSenderAddr), ethgo.Latest)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to check if rootchain contracts are deployed: %w", err))

			return
		} else if code != "0x" {
			outputter.SetCommandResult(&helper.MessageResult{
				Message: fmt.Sprintf("%s contracts are already deployed. Aborting.", contractsDeploymentTitle),
			})

			return
		}
	}

	// set event tracker start blocks for rootchain contract(s) of interest
	// the block number should be queried before deploying contracts so that no events during deployment
	// and initialization are missed
	blockNum, err := client.Eth().BlockNumber()
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to query rootchain latest block number: %w", err))

		return
	}

	deploymentResultInfo, err := deployContracts(outputter, client,
		chainConfig.Params.ChainID, consensusCfg.InitialValidatorSet, cmd.Context())
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy rootchain contracts: %w", err))
		outputter.SetCommandResult(command.Results(deploymentResultInfo.CommandResults))

		return
	}

	// populate bridge configuration
	bridgeConfig := deploymentResultInfo.RootchainCfg.ToBridgeConfig()
	if consensusCfg.Bridge != nil {
		// only true if stake-manager-deploy command was executed
		// users can still deploy stake manager manually
		// only used for e2e tests
		bridgeConfig.StakeTokenAddr = consensusCfg.Bridge.StakeTokenAddr
	}

	consensusCfg.Bridge = bridgeConfig

	consensusCfg.Bridge.EventTrackerStartBlocks = map[types.Address]uint64{
		deploymentResultInfo.RootchainCfg.StateSenderAddress: blockNum,
	}
	consensusCfg.SupernetID = deploymentResultInfo.SupernetID

	// write updated consensus configuration
	chainConfig.Params.Engine[polybft.ConsensusName] = consensusCfg

	if err := cmdHelper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath); err != nil {
		outputter.SetError(fmt.Errorf("failed to save chain configuration bridge data: %w", err))

		return
	}

	deploymentResultInfo.CommandResults = append(deploymentResultInfo.CommandResults, &helper.MessageResult{
		Message: fmt.Sprintf("%s finished. All contracts are successfully deployed and initialized.",
			contractsDeploymentTitle),
	})
	outputter.SetCommandResult(command.Results(deploymentResultInfo.CommandResults))
}

// deployContracts deploys and initializes rootchain smart contracts
func deployContracts(outputter command.OutputFormatter, client *jsonrpc.Client, chainID int64,
	initialValidators []*validator.GenesisValidator, cmdCtx context.Context) (deploymentResultInfo, error) {
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client), txrelayer.WithWriter(outputter))
	if err != nil {
		return deploymentResultInfo{RootchainCfg: nil, SupernetID: 0, CommandResults: nil},
			fmt.Errorf("failed to initialize tx relayer: %w", err)
	}

	deployerKey, err := helper.DecodePrivateKey(params.deployerKey)
	if err != nil {
		return deploymentResultInfo{RootchainCfg: nil, SupernetID: 0, CommandResults: nil},
			fmt.Errorf("failed to initialize deployer key: %w", err)
	}

	if params.isTestMode {
		deployerAddr := deployerKey.Address()

		txn := helper.CreateTransaction(ethgo.ZeroAddress, &deployerAddr, nil, ethgo.Ether(1), true)
		if _, err = txRelayer.SendTransactionLocal(txn); err != nil {
			return deploymentResultInfo{RootchainCfg: nil, SupernetID: 0, CommandResults: nil}, err
		}
	}

	type contractInfo struct {
		name            string
		artifact        *artifact.Artifact
		hasProxy        bool
		byteCodeBuilder func() ([]byte, error)
	}

	rootchainConfig := &polybft.RootchainConfig{
		JSONRPCAddr: params.jsonRPCAddress,
		// update stake manager address in genesis in case if stake manager was deployed manually
		StakeManagerAddress: types.StringToAddress(params.stakeManagerAddr),
	}

	tokenContracts := []*contractInfo{}

	// deploy root ERC20 token only if non-mintable native token flavor is used on a child chain
	if !consensusCfg.NativeTokenConfig.IsMintable {
		if params.rootERC20TokenAddr != "" {
			// use existing root chain ERC20 token
			if err := populateExistingTokenAddr(client.Eth(),
				params.rootERC20TokenAddr, rootERC20Name, rootchainConfig); err != nil {
				return deploymentResultInfo{RootchainCfg: nil, SupernetID: 0, CommandResults: nil}, err
			}
		} else {
			// deploy MockERC20 as a root chain root native token
			tokenContracts = append(tokenContracts,
				&contractInfo{name: rootERC20Name, artifact: contractsapi.RootERC20})
		}
	}

	allContracts := []*contractInfo{
		{
			name:     stateSenderName,
			artifact: contractsapi.StateSender,
		},
		{
			name:     checkpointManagerName,
			artifact: contractsapi.CheckpointManager,
			hasProxy: true,
			byteCodeBuilder: func() ([]byte, error) {
				constructorFn := &contractsapi.CheckpointManagerConstructorFn{
					Initiator: types.Address(deployerKey.Address()),
				}

				input, err := constructorFn.EncodeAbi()
				if err != nil {
					return nil, err
				}

				return append(contractsapi.CheckpointManager.Bytecode, input...), nil
			},
		},
		{
			name:     blsName,
			artifact: contractsapi.BLS,
			hasProxy: true,
		},
		{
			name:     bn256G2Name,
			artifact: contractsapi.BLS256,
			hasProxy: true,
		},
		{
			name:     exitHelperName,
			artifact: contractsapi.ExitHelper,
			hasProxy: true,
		},
		{
			name:     rootERC20PredicateName,
			artifact: contractsapi.RootERC20Predicate,
			hasProxy: true,
		},
		{
			name:     childERC20MintablePredicateName,
			artifact: contractsapi.ChildMintableERC20Predicate,
			hasProxy: true,
		},
		{
			name:     erc20TemplateName,
			artifact: contractsapi.ChildERC20,
		},
		{
			name:     rootERC721PredicateName,
			artifact: contractsapi.RootERC721Predicate,
			hasProxy: true,
		},
		{
			name:     childERC721MintablePredicateName,
			artifact: contractsapi.ChildMintableERC721Predicate,
			hasProxy: true,
		},
		{
			name:     erc721TemplateName,
			artifact: contractsapi.ChildERC721,
		},
		{
			name:     rootERC1155PredicateName,
			artifact: contractsapi.RootERC1155Predicate,
			hasProxy: true,
		},
		{
			name:     childERC1155MintablePredicateName,
			artifact: contractsapi.ChildMintableERC1155Predicate,
			hasProxy: true,
		},
		{
			name:     erc1155TemplateName,
			artifact: contractsapi.ChildERC1155,
		},
		{
			name:     customSupernetManagerName,
			artifact: contractsapi.CustomSupernetManager,
			hasProxy: true,
		},
	}

	allContracts = append(tokenContracts, allContracts...)

	g, ctx := errgroup.WithContext(cmdCtx)
	results := make(map[string]*deployContractResult, len(allContracts))
	resultsLock := sync.Mutex{}
	proxyAdmin := types.StringToAddress(params.proxyContractsAdmin)

	for _, contract := range allContracts {
		contract := contract

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				bytecode := contract.artifact.Bytecode
				if contract.byteCodeBuilder != nil {
					bytecode, err = contract.byteCodeBuilder()
					if err != nil {
						return err
					}
				}

				txn := helper.CreateTransaction(ethgo.ZeroAddress, nil, bytecode, nil, true)

				receipt, err := txRelayer.SendTransaction(txn, deployerKey)
				if err != nil {
					return fmt.Errorf("failed sending %s contract deploy transaction: %w", contract.name, err)
				}

				if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
					return fmt.Errorf("deployment of %s contract failed", contract.name)
				}

				deployResults := make([]*deployContractResult, 0, 2)
				implementationAddress := types.Address(receipt.ContractAddress)

				deployResults = append(deployResults, newDeployContractsResult(contract.name,
					implementationAddress,
					receipt.TransactionHash,
					receipt.GasUsed))

				if contract.hasProxy {
					proxyContractName := getProxyNameForImpl(contract.name)

					receipt, err := helper.DeployProxyContract(
						txRelayer, deployerKey, proxyContractName, proxyAdmin, implementationAddress)
					if err != nil {
						return err
					}

					if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
						return fmt.Errorf("deployment of %s contract failed", proxyContractName)
					}

					deployResults = append(deployResults, newDeployContractsResult(proxyContractName,
						types.Address(receipt.ContractAddress),
						receipt.TransactionHash,
						receipt.GasUsed))
				}

				resultsLock.Lock()
				defer resultsLock.Unlock()

				for _, deployResult := range deployResults {
					results[deployResult.Name] = deployResult
				}

				return nil
			}
		})
	}

	if err := g.Wait(); err != nil {
		return collectResultsOnError(results), err
	}

	commandResults := make([]command.CommandResult, 0, len(results))

	for _, result := range results {
		commandResults = append(commandResults, result)

		populatorFn, exists := metadataPopulatorMap[result.Name]
		if !exists {
			continue
		}

		populatorFn(rootchainConfig, result.Address)
	}

	g, ctx = errgroup.WithContext(cmdCtx)

	for contractName := range results {
		contractName := contractName

		initializer, exists := initializersMap[contractName]
		if !exists {
			continue
		}

		g.Go(func() error {
			select {
			case <-cmdCtx.Done():
				return cmdCtx.Err()
			default:
				return initializer(outputter, txRelayer, rootchainConfig, deployerKey)
			}
		})
	}

	if err := g.Wait(); err != nil {
		return deploymentResultInfo{RootchainCfg: nil, SupernetID: 0, CommandResults: nil}, err
	}

	// register supernets manager on stake manager
	supernetID, err := registerChainOnStakeManager(txRelayer, rootchainConfig, deployerKey)
	if err != nil {
		return deploymentResultInfo{RootchainCfg: nil, SupernetID: 0, CommandResults: nil}, err
	}

	return deploymentResultInfo{
		RootchainCfg:   rootchainConfig,
		SupernetID:     supernetID,
		CommandResults: commandResults}, nil
}

// populateExistingTokenAddr checks whether given token is deployed on the provided address.
// If it is, then its address is set to the rootchain config, otherwise an error is returned
func populateExistingTokenAddr(eth *jsonrpc.Eth, tokenAddr, tokenName string,
	rootchainCfg *polybft.RootchainConfig) error {
	addr := types.StringToAddress(tokenAddr)

	code, err := eth.GetCode(ethgo.Address(addr), ethgo.Latest)
	if err != nil {
		return fmt.Errorf("failed to check is %s token deployed: %w", tokenName, err)
	} else if code == "0x" {
		return fmt.Errorf("%s token is not deployed on provided address %s", tokenName, tokenAddr)
	}

	populatorFn, ok := metadataPopulatorMap[tokenName]
	if !ok {
		return fmt.Errorf("root chain metadata populator not registered for contract '%s'", tokenName)
	}

	populatorFn(rootchainCfg, addr)

	return nil
}

// registerChainOnStakeManager registers child chain and its supernet manager on rootchain
func registerChainOnStakeManager(txRelayer txrelayer.TxRelayer,
	rootchainCfg *polybft.RootchainConfig, deployerKey ethgo.Key) (int64, error) {
	registerChainFn := &contractsapi.RegisterChildChainStakeManagerFn{
		Manager: rootchainCfg.CustomSupernetManagerAddress,
	}

	encoded, err := registerChainFn.EncodeAbi()
	if err != nil {
		return 0, fmt.Errorf("failed to encode parameters for registering child chain on supernets. error: %w", err)
	}

	receipt, err := helper.SendTransaction(txRelayer, ethgo.Address(rootchainCfg.StakeManagerAddress),
		encoded, stakeManagerName, deployerKey)
	if err != nil {
		return 0, err
	}

	var (
		childChainRegisteredEvent contractsapi.ChildManagerRegisteredEvent
		found                     bool
		supernetID                int64
	)

	for _, log := range receipt.Logs {
		doesMatch, err := childChainRegisteredEvent.ParseLog(log)
		if err != nil {
			return 0, err
		}

		if !doesMatch {
			continue
		}

		supernetID = childChainRegisteredEvent.ID.Int64()
		found = true

		break
	}

	if !found {
		return 0, errors.New("could not find a log that child chain was registered on stake manager")
	}

	return supernetID, nil
}

// initContract initializes arbitrary contract with given parameters deployed on a given address
func initContract(cmdOutput command.OutputFormatter, txRelayer txrelayer.TxRelayer,
	initInputFn contractsapi.StateTransactionInput, contractAddr types.Address,
	contractName string, deployerKey ethgo.Key) error {
	input, err := initInputFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode initialization params for %s.initialize. error: %w",
			contractName, err)
	}

	if _, err := helper.SendTransaction(txRelayer, ethgo.Address(contractAddr),
		input, contractName, deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(
		&helper.MessageResult{
			Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, contractName),
		})

	return nil
}

func collectResultsOnError(results map[string]*deployContractResult) deploymentResultInfo {
	commandResults := make([]command.CommandResult, 0, len(results)+1)
	messageResult := helper.MessageResult{Message: "[ROOTCHAIN - DEPLOY] Successfully deployed the following contracts\n"}

	for _, result := range results {
		if result != nil {
			// In case an error happened, some of the indices may not be populated.
			// Filter those out.
			commandResults = append(commandResults, result)
		}
	}

	commandResults = append([]command.CommandResult{messageResult}, commandResults...)

	return deploymentResultInfo{
		RootchainCfg:   nil,
		SupernetID:     0,
		CommandResults: commandResults}
}

func getProxyNameForImpl(input string) string {
	return input + ProxySufix
}
