package deploy

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	contractsDeploymentTitle = "[ROOTCHAIN - CONTRACTS DEPLOYMENT]"

	stateSenderName          = "StateSender"
	checkpointManagerName    = "CheckpointManager"
	blsName                  = "BLS"
	bn256G2Name              = "BN256G2"
	exitHelperName           = "ExitHelper"
	rootERC20PredicateName   = "RootERC20Predicate"
	rootERC20Name            = "RootERC20"
	erc20TemplateName        = "ERC20Template"
	rootERC721PredicateName  = "RootERC721Predicate"
	rootERC721Name           = "RootERC721"
	erc721TemplateName       = "ERC721Template"
	rootERC1155PredicateName = "RootERC1155Predicate"
	rootERC1155Name          = "RootERC1155"
	erc1155TemplateName      = "ERC1155Template"
)

var (
	params deployParams

	// metadataPopulatorMap maps rootchain contract names to callback
	// which populates appropriate field in the RootchainMetadata
	metadataPopulatorMap = map[string]func(*polybft.RootchainConfig, types.Address){
		stateSenderName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.StateSenderAddress = addr
		},
		checkpointManagerName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.CheckpointManagerAddress = addr
		},
		blsName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.BLSAddress = addr
		},
		bn256G2Name: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.BN256G2Address = addr
		},
		exitHelperName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ExitHelperAddress = addr
		},
		rootERC20PredicateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC20PredicateAddress = addr
		},
		rootERC20Name: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootNativeERC20Address = addr
		},
		erc20TemplateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ERC20TemplateAddress = addr
		},
		rootERC721PredicateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC721PredicateAddress = addr
		},
		rootERC721Name: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC721Address = addr
		},
		erc721TemplateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC721TemplateAddress = addr
		},
		rootERC1155PredicateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC1155PredicateAddress = addr
		},
		rootERC1155Name: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.RootERC1155Address = addr
		},
		erc1155TemplateName: func(rootchainConfig *polybft.RootchainConfig, addr types.Address) {
			rootchainConfig.ERC1155TemplateAddress = addr
		},
	}
)

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
		genesisPathFlag,
		defaultGenesisPath,
		"genesis file path, which contains chain configuration",
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
		"existing root chain ERC 20 token address",
	)

	cmd.Flags().StringVar(
		&params.rootERC721TokenAddr,
		erc721AddrFlag,
		"",
		"existing root chain ERC 721 token address",
	)

	cmd.Flags().StringVar(
		&params.rootERC1155TokenAddr,
		erc1155AddrFlag,
		"",
		"existing root chain ERC 1155 token address",
	)

	cmd.Flags().BoolVar(
		&params.isTestMode,
		helper.TestModeFlag,
		false,
		"test indicates whether rootchain contracts deployer is hardcoded test account"+
			" (otherwise provided secrets are used to resolve deployer account)",
	)

	cmd.MarkFlagsMutuallyExclusive(helper.TestModeFlag, deployerKeyFlag)

	return cmd
}

func preRunCommand(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s started... Rootchain JSON RPC address %s.", contractsDeploymentTitle, params.jsonRPCAddress),
	})

	chainConfig, err := chain.ImportFromFile(params.genesisPath)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to read chain configuration: %w", err))

		return
	}

	consensusConfig, err := polybft.GetPolyBFTConfig(chainConfig)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to retrieve consensus configuration: %w", err))

		return
	}

	client, err := jsonrpc.NewClient(params.jsonRPCAddress)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize JSON RPC client for provided IP address: %s: %w",
			params.jsonRPCAddress, err))

		return
	}

	if consensusConfig.Bridge != nil {
		code, err := client.Eth().GetCode(ethgo.Address(consensusConfig.Bridge.StateSenderAddr), ethgo.Latest)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to check if rootchain contracts are deployed: %w", err))

			return
		} else if code != "0x" {
			outputter.SetCommandResult(&messageResult{
				Message: fmt.Sprintf("%s contracts are already deployed. Aborting.", contractsDeploymentTitle),
			})

			return
		}
	}

	rootchainCfg, err := deployContracts(outputter, client,
		chainConfig.Params.ChainID, consensusConfig.InitialValidatorSet)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy rootchain contracts: %w", err))

		return
	}

	// populate bridge configuration
	consensusConfig.Bridge = rootchainCfg.ToBridgeConfig()

	// set event tracker start blocks for rootchain contract(s) of interest
	blockNum, err := client.Eth().BlockNumber()
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to query rootchain latest block number: %w", err))

		return
	}

	consensusConfig.Bridge.EventTrackerStartBlocks = map[types.Address]uint64{
		rootchainCfg.StateSenderAddress: blockNum,
	}

	// write updated chain configuration
	chainConfig.Params.Engine[polybft.ConsensusName] = consensusConfig
	if err := cmdHelper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath); err != nil {
		outputter.SetError(fmt.Errorf("failed to save chain configuration bridge data: %w", err))

		return
	}

	outputter.SetCommandResult(&messageResult{
		Message: fmt.Sprintf("%s finished. All contracts are successfully deployed and initialized.",
			contractsDeploymentTitle),
	})
}

// deployContracts deploys and initializes rootchain smart contracts
func deployContracts(outputter command.OutputFormatter, client *jsonrpc.Client,
	chainID int64, initialValidators []*polybft.Validator) (*polybft.RootchainConfig, error) {
	// if the bridge contract is not created, we have to deploy all the contracts
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tx relayer: %w", err)
	}

	deployerKey, err := helper.GetRootchainPrivateKey(params.deployerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize deployer key: %w", err)
	}

	if params.isTestMode {
		deployerAddr := deployerKey.Address()
		txn := &ethgo.Transaction{To: &deployerAddr, Value: ethgo.Ether(1)}

		if _, err = txRelayer.SendTransactionLocal(txn); err != nil {
			return nil, err
		}
	}

	type contractInfo struct {
		name     string
		artifact *artifact.Artifact
	}

	deployContracts := []*contractInfo{
		{
			name:     stateSenderName,
			artifact: contractsapi.StateSender,
		},
		{
			name:     checkpointManagerName,
			artifact: contractsapi.CheckpointManager,
		},
		{
			name:     blsName,
			artifact: contractsapi.BLS,
		},
		{
			name:     bn256G2Name,
			artifact: contractsapi.BLS256,
		},
		{
			name:     exitHelperName,
			artifact: contractsapi.ExitHelper,
		},
		{
			name:     rootERC20PredicateName,
			artifact: contractsapi.RootERC20Predicate,
		},
		{
			name:     erc20TemplateName,
			artifact: contractsapi.ChildERC20,
		},
		{
			name:     rootERC721PredicateName,
			artifact: contractsapi.RootERC721Predicate,
		},
		{
			name:     erc721TemplateName,
			artifact: contractsapi.ChildERC721,
		},
		{
			name:     rootERC1155PredicateName,
			artifact: contractsapi.RootERC1155Predicate,
		},
		{
			name:     erc1155TemplateName,
			artifact: contractsapi.ChildERC1155,
		},
	}
	rootchainConfig := &polybft.RootchainConfig{
		JSONRPCAddr: params.jsonRPCAddress,
	}

	if params.rootERC20TokenAddr != "" {
		// use existing root chain ERC20 token
		if err := populateExistingTokenAddr(client.Eth(),
			params.rootERC20TokenAddr, rootERC20Name, rootchainConfig); err != nil {
			return nil, err
		}
	} else {
		// deploy MockERC20 as a default root chain ERC20 token
		deployContracts = append(deployContracts,
			&contractInfo{name: rootERC20Name, artifact: contractsapi.RootERC20})
	}

	if params.rootERC721TokenAddr != "" {
		// use existing root chain ERC721 token
		if err := populateExistingTokenAddr(client.Eth(),
			params.rootERC721TokenAddr, rootERC721Name, rootchainConfig); err != nil {
			return nil, err
		}
	} else {
		// deploy MockERC721 as a default root chain ERC721 token
		deployContracts = append(deployContracts,
			&contractInfo{name: rootERC721Name, artifact: contractsapi.RootERC721})
	}

	if params.rootERC1155TokenAddr != "" {
		// use existing root chain ERC1155 token
		if err := populateExistingTokenAddr(client.Eth(),
			params.rootERC1155TokenAddr, rootERC1155Name, rootchainConfig); err != nil {
			return nil, err
		}
	} else {
		// deploy MockERC1155 as a default root chain ERC1155 token
		deployContracts = append(deployContracts,
			&contractInfo{name: rootERC1155Name, artifact: contractsapi.RootERC1155})
	}

	for _, contract := range deployContracts {
		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: contract.artifact.Bytecode,
		}

		receipt, err := txRelayer.SendTransaction(txn, deployerKey)
		if err != nil {
			return nil, err
		}

		if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
			return nil, fmt.Errorf("deployment of %s contract failed", contract.name)
		}

		contractAddr := types.Address(receipt.ContractAddress)

		populatorFn, ok := metadataPopulatorMap[contract.name]
		if !ok {
			return nil, fmt.Errorf("rootchain metadata populator not registered for contract '%s'", contract.name)
		}

		populatorFn(rootchainConfig, contractAddr)

		outputter.WriteCommandResult(newDeployContractsResult(contract.name, contractAddr, receipt.TransactionHash))
	}

	// init CheckpointManager
	if err := initializeCheckpointManager(outputter, txRelayer, chainID,
		initialValidators, rootchainConfig, deployerKey); err != nil {
		return nil, err
	}

	// init ExitHelper
	if err := initializeExitHelper(outputter, txRelayer, rootchainConfig, deployerKey); err != nil {
		return nil, err
	}

	// init RootERC20Predicate
	if err := initializeRootERC20Predicate(outputter, txRelayer, rootchainConfig, deployerKey); err != nil {
		return nil, err
	}

	// init RootERC721Predicate
	if err := initializeRootERC721Predicate(outputter, txRelayer, rootchainConfig, deployerKey); err != nil {
		return nil, err
	}

	// init RootERC1155Predicate
	if err := initializeRootERC1155Predicate(outputter, txRelayer, rootchainConfig, deployerKey); err != nil {
		return rootchainConfig, err
	}

	return rootchainConfig, nil
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

// initializeCheckpointManager invokes initialize function on "CheckpointManager" smart contract
func initializeCheckpointManager(
	cmdOutput command.OutputFormatter,
	txRelayer txrelayer.TxRelayer,
	chainID int64,
	validators []*polybft.Validator,
	rootchainCfg *polybft.RootchainConfig,
	deployerKey ethgo.Key) error {
	validatorSet, err := validatorSetToABISlice(cmdOutput, validators)
	if err != nil {
		return fmt.Errorf("failed to convert validators to map: %w", err)
	}

	initialize := contractsapi.InitializeCheckpointManagerFn{
		ChainID_:        big.NewInt(chainID),
		NewBls:          rootchainCfg.BLSAddress,
		NewBn256G2:      rootchainCfg.BN256G2Address,
		NewValidatorSet: validatorSet,
	}

	input, err := initialize.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode parameters for CheckpointManager.initialize. error: %w", err)
	}

	if err := sendTransaction(txRelayer, ethgo.Address(rootchainCfg.CheckpointManagerAddress),
		input, checkpointManagerName, deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, checkpointManagerName),
	})

	return nil
}

// initializeExitHelper invokes initialize function on "ExitHelper" smart contract
func initializeExitHelper(cmdOutput command.OutputFormatter,
	txRelayer txrelayer.TxRelayer,
	rootchainConfig *polybft.RootchainConfig,
	deployerKey ethgo.Key) error {
	input, err := contractsapi.ExitHelper.Abi.GetMethod("initialize").
		Encode([]interface{}{rootchainConfig.CheckpointManagerAddress})
	if err != nil {
		return fmt.Errorf("failed to encode parameters for ExitHelper.initialize. error: %w", err)
	}

	if err := sendTransaction(txRelayer, ethgo.Address(rootchainConfig.ExitHelperAddress),
		input, exitHelperName, deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, exitHelperName),
	})

	return nil
}

// initializeRootERC20Predicate invokes initialize function on "RootERC20Predicate" smart contract
func initializeRootERC20Predicate(cmdOutput command.OutputFormatter, txRelayer txrelayer.TxRelayer,
	rootchainConfig *polybft.RootchainConfig, deployerKey ethgo.Key) error {
	rootERC20PredicateParams := &contractsapi.InitializeRootERC20PredicateFn{
		NewStateSender:         rootchainConfig.StateSenderAddress,
		NewExitHelper:          rootchainConfig.ExitHelperAddress,
		NewChildERC20Predicate: contracts.ChildERC20PredicateContract,
		NewChildTokenTemplate:  rootchainConfig.ERC20TemplateAddress,
		NativeTokenRootAddress: rootchainConfig.RootNativeERC20Address,
	}

	input, err := rootERC20PredicateParams.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode parameters for RootERC20Predicate.initialize. error: %w", err)
	}

	if err := sendTransaction(txRelayer, ethgo.Address(rootchainConfig.RootERC20PredicateAddress),
		input, rootERC20PredicateName, deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(
		&messageResult{
			Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, rootERC20PredicateName),
		})

	return nil
}

func initializeRootERC721Predicate(cmdOutput command.OutputFormatter, txRelayer txrelayer.TxRelayer,
	rootchainConfig *polybft.RootchainConfig, deployerKey ethgo.Key) error {
	rootERC721PredicateParams := contractsapi.InitializeRootERC721PredicateFn{
		NewStateSender:          rootchainConfig.StateSenderAddress,
		NewExitHelper:           rootchainConfig.ExitHelperAddress,
		NewChildERC721Predicate: contracts.ChildERC721PredicateContract,
		NewChildTokenTemplate:   rootchainConfig.RootERC721TemplateAddress,
	}

	input, err := rootERC721PredicateParams.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode parameters for RootERC721Predicate.initialize. error: %w", err)
	}

	if err := sendTransaction(txRelayer, ethgo.Address(rootchainConfig.RootERC721PredicateAddress),
		input, rootERC721PredicateName, deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, rootERC721PredicateName),
	})

	return nil
}

// initializeRootERC1155Predicate invokes initialize function on RootERC1155Predicate smart contract
func initializeRootERC1155Predicate(cmdOutput command.OutputFormatter, txRelayer txrelayer.TxRelayer,
	rootchainConfig *polybft.RootchainConfig, deployerKey ethgo.Key) error {
	rootERC1155PredicateParams := &contractsapi.InitializeRootERC1155PredicateFn{
		NewStateSender:           rootchainConfig.StateSenderAddress,
		NewExitHelper:            rootchainConfig.ExitHelperAddress,
		NewChildERC1155Predicate: contracts.ChildERC1155PredicateContract,
		NewChildTokenTemplate:    rootchainConfig.ERC1155TemplateAddress,
	}

	input, err := rootERC1155PredicateParams.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode parameters for RootERC1155Predicate.initialize. error: %w", err)
	}

	if err := sendTransaction(txRelayer, ethgo.Address(rootchainConfig.RootERC1155PredicateAddress),
		input, rootERC1155PredicateName, deployerKey); err != nil {
		return err
	}

	cmdOutput.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, rootERC1155PredicateName),
	})

	return nil
}

// sendTransaction sends provided transaction
func sendTransaction(txRelayer txrelayer.TxRelayer, addr ethgo.Address, input []byte, contractName string,
	deployerKey ethgo.Key) error {
	txn := &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}

	receipt, err := txRelayer.SendTransaction(txn, deployerKey)
	if err != nil {
		return fmt.Errorf("failed to send transaction to %s contract (%s). error: %w", contractName, txn.To.Address(), err)
	}

	if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
		return fmt.Errorf("transaction execution failed on %s contract", contractName)
	}

	return nil
}

// validatorSetToABISlice converts given validators to generic map
// which is used for ABI encoding validator set being sent to the rootchain contract
func validatorSetToABISlice(o command.OutputFormatter,
	validators []*polybft.Validator) ([]*contractsapi.Validator, error) {
	accSet := make(polybft.AccountSet, len(validators))

	if _, err := o.Write([]byte(fmt.Sprintf("%s [VALIDATORS]\n", contractsDeploymentTitle))); err != nil {
		return nil, err
	}

	for i, validator := range validators {
		if _, err := o.Write([]byte(fmt.Sprintf("%v\n", validator))); err != nil {
			return nil, err
		}

		blsKey, err := validator.UnmarshalBLSPublicKey()
		if err != nil {
			return nil, err
		}

		accSet[i] = &polybft.ValidatorMetadata{
			Address:     validator.Address,
			BlsKey:      blsKey,
			VotingPower: new(big.Int).Set(validator.Stake),
		}
	}

	hash, err := accSet.Hash()
	if err != nil {
		return nil, err
	}

	if _, err := o.Write([]byte(fmt.Sprintf("%s Validators hash: %s\n", contractsDeploymentTitle, hash))); err != nil {
		return nil, err
	}

	return accSet.ToAPIBinding(), nil
}
