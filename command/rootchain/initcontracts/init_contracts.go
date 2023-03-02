package initcontracts

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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

	// defaultAllowanceValue is value which is assigned to the RootERC20Predicate spender
	defaultAllowanceValue = uint64(1e19)
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

	setFlags(cmd)

	return cmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.manifestPath,
		manifestPathFlag,
		defaultManifestPath,
		"Manifest file path, which contains metadata",
	)

	cmd.Flags().StringVar(
		&params.adminKey,
		adminKeyFlag,
		helper.DefaultPrivateKeyRaw,
		"Hex encoded private key of the account which deploys rootchain contracts",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the JSON RPC rootchain IP address (e.g. "+txrelayer.DefaultRPCAddress+")",
	)
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

	if err := helper.InitRootchainPrivateKey(params.adminKey); err != nil {
		outputter.SetError(err)

		return
	}

	if err := deployContracts(outputter, client, manifest); err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy rootchain contracts: %w", err))

		return
	}

	outputter.SetCommandResult(&messageResult{
		Message: fmt.Sprintf("%s finished. All contracts are successfully deployed and initialized.",
			contractsDeploymentTitle),
	})
}

func deployContracts(outputter command.OutputFormatter, client *jsonrpc.Client, manifest *polybft.Manifest) error {
	// if the bridge contract is not created, we have to deploy all the contracts
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	if err != nil {
		return fmt.Errorf("failed to initialize tx relayer: %w", err)
	}

	rootchainAdminKey := helper.GetRootchainPrivateKey()
	// if admin key is equal to the test private key, then we assume we are working in dev mode
	// and therefore need to fund that account
	if helper.IsTestMode(params.adminKey) {
		// fund account
		rootchainAdminAddr := rootchainAdminKey.Address()
		txn := &ethgo.Transaction{To: &rootchainAdminAddr, Value: big.NewInt(1000000000000000000)}
		_, err = txRelayer.SendTransactionLocal(txn)

		if err != nil {
			return err
		}
	}

	deployContracts := []struct {
		name     string
		artifact *artifact.Artifact
	}{
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
			name:     "RootERC20",
			artifact: contractsapi.RootERC20,
		},
		{
			name:     "ERC20Template",
			artifact: contractsapi.ChildERC20,
		},
	}

	rootchainConfig := &polybft.RootchainConfig{}
	manifest.RootchainConfig = rootchainConfig

	for _, contract := range deployContracts {
		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: contract.artifact.Bytecode,
		}

		receipt, err := txRelayer.SendTransaction(txn, rootchainAdminKey)
		if err != nil {
			return err
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
	if err := initializeCheckpointManager(txRelayer, manifest); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, checkpointManagerName),
	})

	// init ExitHelper
	if err := initializeExitHelper(txRelayer, rootchainConfig); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, exitHelperName),
	})

	// init RootERC20Predicate
	if err := initializeRootERC20Predicate(txRelayer, rootchainConfig); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s %s contract is initialized", contractsDeploymentTitle, rootERC20PredicateName),
	})

	if helper.IsTestMode(params.adminKey) {
		// approve RootERC20Predicate
		if err := approveERC20Predicate(txRelayer, rootchainConfig); err != nil {
			return err
		}

		outputter.WriteCommandResult(&messageResult{
			Message: fmt.Sprintf("%s %s contract is approved for spender of %s",
				contractsDeploymentTitle, rootERC20PredicateName, rootERC20Name),
		})
	}

	return nil
}

// initializeCheckpointManager invokes initialize function on "CheckpointManager" smart contract
func initializeCheckpointManager(
	txRelayer txrelayer.TxRelayer,
	manifest *polybft.Manifest) error {
	validatorSet, err := validatorSetToABISlice(manifest.GenesisValidators)
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

	return sendTransaction(txRelayer, txn, checkpointManagerName)
}

// initializeExitHelper invokes initialize function on "ExitHelper" smart contract
func initializeExitHelper(txRelayer txrelayer.TxRelayer, rootchainConfig *polybft.RootchainConfig) error {
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

	return sendTransaction(txRelayer, txn, exitHelperName)
}

// initializeRootERC20Predicate invokes initialize function on "RootERC20Predicate" smart contract
func initializeRootERC20Predicate(txRelayer txrelayer.TxRelayer, rootchainConfig *polybft.RootchainConfig) error {
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

	return sendTransaction(txRelayer, txn, rootERC20PredicateName)
}

// approveERC20Predicate sends approve transaction to ERC20 token so that it is able to spend given root ERC20 token
func approveERC20Predicate(txRelayer txrelayer.TxRelayer, config *polybft.RootchainConfig) error {
	approveFnParams := &contractsapi.ApproveFunction{
		Spender: config.RootERC20PredicateAddress,
		Amount:  new(big.Int).SetUint64(defaultAllowanceValue),
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode parameters for RootERC20.approve. error: %w", err)
	}

	addr := ethgo.Address(config.RootNativeERC20Address)
	txn := &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}

	return sendTransaction(txRelayer, txn, rootERC20Name)
}

// sendTransaction sends provided transaction
func sendTransaction(txRelayer txrelayer.TxRelayer, txn *ethgo.Transaction, contractName string) error {
	receipt, err := txRelayer.SendTransaction(txn, helper.GetRootchainPrivateKey())
	if err != nil {
		return fmt.Errorf("failed to send transaction to %s contract (%s). error: %w", contractName, txn.To.Address(), err)
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return fmt.Errorf("transaction execution failed on %s contract", contractName)
	}

	return nil
}

// validatorSetToABISlice converts given validators to generic map
// which is used for ABI encoding validator set being sent to the rootchain contract
func validatorSetToABISlice(validators []*polybft.Validator) ([]*contractsapi.Validator, error) {
	genesisValidators := make([]*polybft.Validator, len(validators))
	copy(genesisValidators, validators)
	sort.Slice(genesisValidators, func(i, j int) bool {
		return bytes.Compare(genesisValidators[i].Address.Bytes(), genesisValidators[j].Address.Bytes()) < 0
	})

	accSet := make(polybft.AccountSet, len(genesisValidators))

	for i, validatorInfo := range genesisValidators {
		blsKey, err := validatorInfo.UnmarshalBLSPublicKey()
		if err != nil {
			return nil, err
		}

		accSet[i] = &polybft.ValidatorMetadata{
			Address:     validatorInfo.Address,
			BlsKey:      blsKey,
			VotingPower: new(big.Int).Set(validatorInfo.Balance),
		}
	}

	return accSet.ToAPIBinding(), nil
}
