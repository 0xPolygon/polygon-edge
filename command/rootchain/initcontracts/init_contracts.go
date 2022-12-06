package initcontracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	contractsDeploymentTitle = "[ROOTCHAIN - CONTRACTS DEPLOYMENT]"

	stateSenderName       = "StateSender"
	checkpointManagerName = "CheckpointManager"
	blsName               = "BLS"
	bn256G2Name           = "BN256G2"
)

var (
	params initContractsParams

	initCheckpointManager, _ = abi.NewMethod("function initialize(" +
		// BLS contract address
		"address newBls," +
		// BN256G2 contract address
		"address newBn256G2," +
		// domain used for BLS signing
		"bytes32 newDomain," +
		// rootchain validator set
		"tuple(address _address, uint256[4] blsKey, uint256 votingPower)[] newValidatorSet)")

	// metadataPopulatorMap maps rootchain contract names to callback
	// which populates appropriate field in the RootchainMetadata
	metadataPopulatorMap = map[string]func(*helper.RootchainManifest, types.Address){
		stateSenderName: func(metadata *helper.RootchainManifest, addr types.Address) {
			metadata.StateSenderAddress = addr
		},
		checkpointManagerName: func(metadata *helper.RootchainManifest, addr types.Address) {
			metadata.CheckpointManagerAddress = addr
		},
		blsName: func(metadata *helper.RootchainManifest, addr types.Address) {
			metadata.BLSAddress = addr
		},
		bn256G2Name: func(metadata *helper.RootchainManifest, addr types.Address) {
			metadata.BN256G2Address = addr
		},
	}
)

// GetCommand returns the rootchain emit command
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
		&params.contractsPath,
		contractsPathFlag,
		contracts.ContractsRootFolder,
		"Root file directory containing POS smart contracts",
	)

	cmd.Flags().StringVar(
		&params.genesisPath,
		genesisPathFlag,
		defaultGenesisPath,
		"Genesis configuration path",
	)

	cmd.Flags().StringVar(
		&params.manifestPath,
		params.manifestPath,
		defaultManifestPath,
		"Manifest file path, which contains rootchain metadata",
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
		"http://127.0.0.1:8545",
		"The JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)",
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

	code, err := client.Eth().GetCode(ethgo.Address(helper.StateSenderAddress), ethgo.Latest)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to check if rootchain contracts are deployed: %w", err))

		return
	} else if code != "0x" {
		outputter.SetCommandResult(&messageResult{
			Message: fmt.Sprintf("%s contracts are already deployed. Aborting.", contractsDeploymentTitle),
		})

		return
	}

	if err := helper.InitRootchainAdminKey(params.adminKey); err != nil {
		outputter.SetError(err)

		return
	}

	if err := deployContracts(outputter, client); err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy rootchain contracts: %w", err))

		return
	}

	outputter.SetCommandResult(&messageResult{
		Message: fmt.Sprintf("%s finished. All contracts are successfully deployed and initialized.",
			contractsDeploymentTitle),
	})
}

func deployContracts(outputter command.OutputFormatter, client *jsonrpc.Client) error {
	// if the bridge contract is not created, we have to deploy all the contracts
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	if err != nil {
		return fmt.Errorf("failed to initialize tx relayer: %w", err)
	}

	rootchainAdminKey := helper.GetRootchainAdminKey()
	// if admin key is not provided, then we assume we are working in dev mode
	// and therefore use default key which needs to be funded first
	if params.adminKey == "" {
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
		path     string
		expected types.Address
	}{
		{
			name:     stateSenderName,
			path:     "root/StateSender.sol",
			expected: helper.StateSenderAddress,
		},
		{
			name:     checkpointManagerName,
			path:     "root/CheckpointManager.sol",
			expected: helper.CheckpointManagerAddress,
		},
		{
			name:     blsName,
			path:     "common/BLS.sol",
			expected: helper.BLSAddress,
		},
		{
			name:     bn256G2Name,
			path:     "common/BN256G2.sol",
			expected: helper.BN256G2Address,
		},
	}

	rootchainMeta := &helper.RootchainManifest{}

	for _, contract := range deployContracts {
		bytecode, err := readContractBytecode(params.contractsPath, contract.path, contract.name)
		if err != nil {
			return err
		}

		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: bytecode,
		}

		receipt, err := txRelayer.SendTransaction(txn, rootchainAdminKey)
		if err != nil {
			return err
		}

		contractAddr := types.Address(receipt.ContractAddress)
		if contractAddr != contract.expected {
			return fmt.Errorf("wrong deployed address for contract %s: expected %s but found %s",
				contract.name, contract.expected, receipt.ContractAddress)
		}

		populatorFn, ok := metadataPopulatorMap[contract.name]
		if !ok {
			return fmt.Errorf("rootchain metadata populator not registered for contract '%s'", contract.name)
		}

		populatorFn(rootchainMeta, contractAddr)

		outputter.WriteCommandResult(newDeployContractsResult(contract.name, contract.expected, receipt.TransactionHash))
	}

	if err := rootchainMeta.Save(params.manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest data: %w", err)
	}

	if err := initializeCheckpointManager(txRelayer, rootchainAdminKey); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s CheckpointManager contract is initialized", contractsDeploymentTitle),
	})

	return nil
}

// initializeCheckpointManager invokes initialize function on CheckpointManager smart contract
func initializeCheckpointManager(txRelayer txrelayer.TxRelayer, rootchainAdminKey ethgo.Key) error {
	validatorSetMap, err := validatorSetToABISlice()
	if err != nil {
		return fmt.Errorf("failed to convert validators to map: %w", err)
	}

	initCheckpointInput, err := initCheckpointManager.Encode(
		[]interface{}{
			helper.BLSAddress,
			helper.BN256G2Address,
			bls.GetDomain(),
			validatorSetMap,
		})

	if err != nil {
		return fmt.Errorf("failed to encode parameters for CheckpointManager.initialize. error: %w", err)
	}

	checkpointManagerAddress := ethgo.Address(helper.CheckpointManagerAddress)
	txn := &ethgo.Transaction{
		To:    &checkpointManagerAddress,
		Input: initCheckpointInput,
	}

	receipt, err := txRelayer.SendTransaction(txn, rootchainAdminKey)
	if err != nil {
		return fmt.Errorf("failed to send transaction to CheckpointManager. error: %w", err)
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return errors.New("failed to initialize CheckpointManager")
	}

	return nil
}

// initializeCheckpointManager invokes initialize function on CheckpointManager smart contract
func validatorSetToABISlice() ([]map[string]interface{}, error) {
	chainConfig, err := chain.ImportFromFile(params.genesisPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis configuration %w", err)
	}

	polyBFTConfig, err := polybft.GetPolyBFTConfig(chainConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal polybft config: %w", err)
	}

	genesisValidators := make([]*polybft.Validator, len(polyBFTConfig.InitialValidatorSet))
	copy(genesisValidators, polyBFTConfig.InitialValidatorSet)
	sort.Slice(genesisValidators, func(i, j int) bool {
		return bytes.Compare(genesisValidators[i].Address.Bytes(), genesisValidators[j].Address.Bytes()) < 0
	})

	validatorSetMap := make([]map[string]interface{}, len(polyBFTConfig.InitialValidatorSet))

	for i, validatorInfo := range polyBFTConfig.InitialValidatorSet {
		blsKey, err := validatorInfo.UnmarshalBLSPublicKey()
		if err != nil {
			return nil, err
		}

		validatorSetMap[i] = map[string]interface{}{
			"_address":    validatorInfo.Address,
			"blsKey":      blsKey.ToBigInt(),
			"votingPower": chain.ConvertWeiToTokensAmount(validatorInfo.Balance),
		}
	}

	return validatorSetMap, nil
}

func readContractBytecode(rootPath, contractPath, contractName string) ([]byte, error) {
	_, fileName := filepath.Split(contractPath)

	absolutePath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(absolutePath, contractPath, strings.TrimSuffix(fileName, ".sol")+".json")

	data, err := ioutil.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}

	var artifact struct {
		Bytecode string `json:"bytecode"`
	}

	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}

	return hex.MustDecodeHex(artifact.Bytecode), nil
}
