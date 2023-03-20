package initcontracts

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	contractsDeploymentTitle = "[ROOTCHAIN - CONTRACTS DEPLOYMENT]"

	stateSenderName        = "StateSender"
	checkpointManagerName  = "CheckpointManager"
	blsName                = "BLS"
	bn256G2Name            = "BN256G2"
	exitHelperName         = "ExitHelper"
	rootERC20PredicateName = "RootERC20Predicate"
	rootERC20Name          = "RootERC20"
	erc20TemplateName      = "ERC20Template"
)

var (
	params initContractsParams

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
	}
)

// GetCommand returns the rootchain init-contracts command
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init-contracts",
		Short:   "Deploys and initializes required smart contracts on the rootchain",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	cmd.Flags().StringVar(
		&params.manifestPath,
		manifestPathFlag,
		defaultManifestPath,
		"manifest file path, which contains metadata",
	)

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
		&params.jsonRPCAddress,
		jsonRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the JSON RPC rootchain IP address",
	)

	cmd.Flags().StringVar(
		&params.rootERC20TokenAddr,
		rootchainERC20Flag,
		"",
		"existing root chain ERC20 token address",
	)

	cmd.Flags().BoolVar(
		&params.isTestMode,
		helper.TestModeFlag,
		false,
		"test indicates whether rootchain contracts deployer is hardcoded test account"+
			" (otherwise provided secrets are used to resolve deployer account)",
	)

	cmd.MarkFlagsMutuallyExclusive(
		helper.TestModeFlag,
		polybftsecrets.AccountDirFlag,
		polybftsecrets.AccountConfigFlag)

	return cmd
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s started...", contractsDeploymentTitle),
	})

	var deployerKey ethgo.Key

	if !params.isTestMode {
		secretsManager, err := polybftsecrets.GetSecretsManager(params.accountDir, params.accountConfig, true)
		if err != nil {
			outputter.SetError(err)

			return
		}

		deployerAccount, err := wallet.NewAccountFromSecret(secretsManager)
		if err != nil {
			outputter.SetError(err)

			return
		}

		deployerKey = deployerAccount.Ecdsa
	} else {
		rootchainKey, err := helper.GetRootchainTestPrivKey()
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to initialize root chain private key: %w", err))

			return
		}

		deployerKey = rootchainKey
	}

	client, err := jsonrpc.NewClient(params.jsonRPCAddress)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize JSON RPC client for provided IP address: %s: %w",
			params.jsonRPCAddress, err))

		return
	}

	manifest, err := polybft.LoadManifest(params.manifestPath)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to read manifest: %w", err))

		return
	}

	if manifest.RootchainConfig != nil {
		code, err := client.Eth().GetCode(ethgo.Address(manifest.RootchainConfig.StateSenderAddress), ethgo.Latest)
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

	if err := deployContracts(outputter, client, manifest, deployerKey); err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy rootchain contracts: %w", err))

		return
	}

	outputter.SetCommandResult(&messageResult{
		Message: fmt.Sprintf("%s finished. All contracts are successfully deployed and initialized.",
			contractsDeploymentTitle),
	})
}

func deployContracts(outputter command.OutputFormatter, client *jsonrpc.Client,
	manifest *polybft.Manifest, deployerKey ethgo.Key) error {
	// if the bridge contract is not created, we have to deploy all the contracts
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	if err != nil {
		return fmt.Errorf("failed to initialize tx relayer: %w", err)
	}

	// if running in test mode, we need to fund deployer account
	if params.isTestMode {
		fundAmountRaw := strings.TrimPrefix(command.DefaultPremineBalance, "0x")

		fundAmount, ok := new(big.Int).SetString(fundAmountRaw, 16)
		if !ok {
			return fmt.Errorf("failed to parse provided fund amount: %s", fundAmountRaw)
		}

		// fund account
		deployerAddress := deployerKey.Address()

		txn := &ethgo.Transaction{To: &deployerAddress, Value: fundAmount}
		if _, err := txRelayer.SendTransactionLocal(txn); err != nil {
			return err
		}
	}

	rootchainConfig := &polybft.RootchainConfig{}
	manifest.RootchainConfig = rootchainConfig

	type contractInfo struct {
		name     string
		artifact *artifact.Artifact
	}

	deployContracts := []*contractInfo{
		{
			name:     "StateSender",
			artifact: contractsapi.StateSender,
		},
		{
			name:     "CheckpointManager",
			artifact: contractsapi.CheckpointManager,
		},
		{
			name:     "BLS",
			artifact: contractsapi.BLS,
		},
		{
			name:     "BN256G2",
			artifact: contractsapi.BLS256,
		},
		{
			name:     "ExitHelper",
			artifact: contractsapi.ExitHelper,
		},
		{
			name:     "RootERC20Predicate",
			artifact: contractsapi.RootERC20Predicate,
		},
		{
			name:     "ERC20Template",
			artifact: contractsapi.ChildERC20,
		},
	}

	if params.rootERC20TokenAddr != "" {
		// use existing root chain ERC20 token
		addr := types.StringToAddress(params.rootERC20TokenAddr)

		code, err := client.Eth().GetCode(ethgo.Address(addr), ethgo.Latest)
		if err != nil {
			return fmt.Errorf("failed to check is root chain ERC20 token deployed: %w", err)
		} else if code == "0x" {
			return fmt.Errorf("root chain ERC20 token is not deployed on provided address %s", params.rootERC20TokenAddr)
		}

		populatorFn, ok := metadataPopulatorMap["RootERC20"]
		if !ok {
			return fmt.Errorf("root chain metadata populator not registered for contract 'RootERC20'")
		}

		populatorFn(manifest.RootchainConfig, addr)
	} else {
		// deploy MockERC20 as default root chain ERC20 token
		deployContracts = append(deployContracts, &contractInfo{name: "RootERC20", artifact: contractsapi.RootERC20})
	}

	for _, contract := range deployContracts {
		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: contract.artifact.Bytecode,
		}

		receipt, err := txRelayer.SendTransaction(txn, deployerKey)
		if err != nil {
			return err
		}

		if receipt == nil || receipt.Status != uint64(types.ReceiptSuccess) {
			return fmt.Errorf("deployment of %s contract failed", contract.name)
		}

		contractAddr := types.Address(receipt.ContractAddress)

		populatorFn, ok := metadataPopulatorMap[contract.name]
		if !ok {
			return fmt.Errorf("rootchain metadata populator not registered for contract '%s'", contract.name)
		}

		populatorFn(manifest.RootchainConfig, contractAddr)

		outputter.WriteCommandResult(newDeployContractsResult(contract.name, contractAddr, receipt.TransactionHash))
	}

	if err := manifest.Save(params.manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest data: %w", err)
	}

	// init CheckpointManager
	if err := initializeCheckpointManager(outputter, txRelayer, manifest, deployerKey); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, checkpointManagerName),
	})

	// init ExitHelper
	if err := initializeExitHelper(txRelayer, rootchainConfig, deployerKey); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, exitHelperName),
	})

	// init RootERC20Predicate
	if err := initializeRootERC20Predicate(txRelayer, rootchainConfig, deployerKey); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, rootERC20PredicateName),
	})

	return nil
}

// initializeCheckpointManager invokes initialize function on "CheckpointManager" smart contract
func initializeCheckpointManager(
	o command.OutputFormatter,
	txRelayer txrelayer.TxRelayer,
	manifest *polybft.Manifest,
	deployerKey ethgo.Key) error {
	validatorSet, err := validatorSetToABISlice(o, manifest.GenesisValidators)
	if err != nil {
		return fmt.Errorf("failed to convert validators to map: %w", err)
	}

	initialize := contractsapi.InitializeCheckpointManagerFunction{
		ChainID_:        big.NewInt(manifest.ChainID),
		NewBls:          manifest.RootchainConfig.BLSAddress,
		NewBn256G2:      manifest.RootchainConfig.BN256G2Address,
		NewValidatorSet: validatorSet,
	}

	initCheckpointInput, err := initialize.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode parameters for CheckpointManager.initialize. error: %w", err)
	}

	addr := ethgo.Address(manifest.RootchainConfig.CheckpointManagerAddress)
	txn := &ethgo.Transaction{
		To:    &addr,
		Input: initCheckpointInput,
	}

	return sendTransaction(txRelayer, txn, checkpointManagerName, deployerKey)
}

// initializeExitHelper invokes initialize function on "ExitHelper" smart contract
func initializeExitHelper(txRelayer txrelayer.TxRelayer, rootchainConfig *polybft.RootchainConfig,
	deployerKey ethgo.Key) error {
	input, err := contractsapi.ExitHelper.Abi.GetMethod("initialize").
		Encode([]interface{}{rootchainConfig.CheckpointManagerAddress})
	if err != nil {
		return fmt.Errorf("failed to encode parameters for ExitHelper.initialize. error: %w", err)
	}

	addr := ethgo.Address(rootchainConfig.ExitHelperAddress)
	txn := &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}

	return sendTransaction(txRelayer, txn, exitHelperName, deployerKey)
}

// initializeRootERC20Predicate invokes initialize function on "RootERC20Predicate" smart contract
func initializeRootERC20Predicate(txRelayer txrelayer.TxRelayer, rootchainConfig *polybft.RootchainConfig,
	deployerKey ethgo.Key) error {
	rootERC20PredicateParams := &contractsapi.InitializeRootERC20PredicateFunction{
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

	addr := ethgo.Address(rootchainConfig.RootERC20PredicateAddress)
	txn := &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}

	return sendTransaction(txRelayer, txn, rootERC20PredicateName, deployerKey)
}

// sendTransaction sends provided transaction
func sendTransaction(txRelayer txrelayer.TxRelayer, txn *ethgo.Transaction, contractName string,
	deployerKey ethgo.Key) error {
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
			VotingPower: new(big.Int).Set(validator.Balance),
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
