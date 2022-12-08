package initcontracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
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
		// RootValidatorSet contract address
		"tuple(address _address, uint256[4] blsKey, uint256 votingPower)[] newValidatorSet)")
)

const (
	contractsDeploymentTitle = "[ROOTCHAIN - CONTRACTS DEPLOYMENT]"
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
		"Root path for the smart contracts",
	)
	cmd.Flags().StringVar(
		&params.validatorPath,
		validatorPathFlag,
		defaultValidatorPath,
		"Validators path",
	)
	cmd.Flags().StringVar(
		&params.validatorPrefixPath,
		validatorPrefixPathFlag,
		defaultValidatorPrefixPath,
		"Validators prefix path",
	)
	cmd.Flags().StringVar(
		&params.genesisPath,
		genesisPathFlag,
		defaultGenesisPath,
		"Genesis configuration path",
	)
	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)",
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

	if err := deployContracts(outputter); err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy rootchain contracts: %w", err))

		return
	}

	outputter.SetCommandResult(&messageResult{
		Message: fmt.Sprintf("%s finished. All contracts are successfully deployed and initialized.",
			contractsDeploymentTitle),
	})
}

func getGenesisAlloc() (map[types.Address]*chain.GenesisAccount, error) {
	genesisFile, err := os.Open(params.genesisPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open genesis config file: %w", err)
	}

	genesisRaw, err := ioutil.ReadAll(genesisFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis config file: %w", err)
	}

	var chain *chain.Chain
	if err := json.Unmarshal(genesisRaw, &chain); err != nil {
		return nil, fmt.Errorf("failed to unmarshal genesis configuration: %w", err)
	}

	return chain.Genesis.Alloc, nil
}

func deployContracts(outputter command.OutputFormatter) error {
	// if the bridge contract is not created, we have to deploy all the contracts
	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPCAddress))
	if err != nil {
		return fmt.Errorf("failed to initialize tx relayer: %w", err)
	}

	// TODO: @Stefan-Ethernal Skip FundAccount part in follow up PR if in "dev" mode
	// fund account
	rootchainAdminAddr := ethgo.Address(helper.GetRootchainAdminAddr())
	txn := &ethgo.Transaction{To: &rootchainAdminAddr, Value: big.NewInt(1000000000000000000)}

	_, err = txRelayer.SendTransactionLocal(txn)
	if err != nil {
		return err
	}

	deployContracts := []struct {
		name     string
		path     string
		expected types.Address
	}{
		{
			name:     "StateSender",
			path:     "root/StateSender.sol",
			expected: helper.StateSenderAddress,
		},
		{
			name:     "CheckpointManager",
			path:     "root/CheckpointManager.sol",
			expected: helper.CheckpointManagerAddress,
		},
		{
			name:     "BLS",
			path:     "common/BLS.sol",
			expected: helper.BLSAddress,
		},
		{
			name:     "BN256G2",
			path:     "common/BN256G2.sol",
			expected: helper.BN256G2Address,
		},
	}

	for _, contract := range deployContracts {
		bytecode, err := readContractBytecode(params.contractsPath, contract.path, contract.name)
		if err != nil {
			return err
		}

		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: bytecode,
		}

		receipt, err := txRelayer.SendTransaction(txn, helper.GetRootchainAdminKey())
		if err != nil {
			return err
		}

		if types.Address(receipt.ContractAddress) != contract.expected {
			return fmt.Errorf("wrong deployed address for contract %s: expected %s but found %s",
				contract.name, contract.expected, receipt.ContractAddress)
		}

		outputter.WriteCommandResult(newDeployContractsResult(contract.name, contract.expected, receipt.TransactionHash))
	}

	if err := initializeCheckpointManager(txRelayer); err != nil {
		return err
	}

	outputter.WriteCommandResult(&messageResult{
		Message: fmt.Sprintf("%s CheckpointManager contract is initialized", contractsDeploymentTitle),
	})

	return nil
}

// initializeCheckpointManager invokes initialize function on CheckpointManager smart contract
func initializeCheckpointManager(txRelayer txrelayer.TxRelayer) error {
	allocs, err := getGenesisAlloc()
	if err != nil {
		return err
	}

	validatorSetMap, err := validatorSetToABISlice(allocs)
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

	receipt, err := txRelayer.SendTransaction(txn, helper.GetRootchainAdminKey())
	if err != nil {
		return fmt.Errorf("failed to send transaction to CheckpointManager. error: %w", err)
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return errors.New("failed to initialize CheckpointManager")
	}

	return nil
}

// initializeCheckpointManager invokes initialize function on CheckpointManager smart contract
func validatorSetToABISlice(allocs map[types.Address]*chain.GenesisAccount) ([]map[string]interface{}, error) {
	validatorsInfo, err := genesis.ReadValidatorsByRegexp(params.validatorPath, params.validatorPrefixPath)
	if err != nil {
		return nil, err
	}

	validatorSetMap := make([]map[string]interface{}, len(validatorsInfo))

	sort.Slice(validatorsInfo, func(i, j int) bool {
		return bytes.Compare(validatorsInfo[i].Address.Bytes(),
			validatorsInfo[j].Address.Bytes()) < 0
	})

	for i, validatorInfo := range validatorsInfo {
		genesisBalance, err := chain.GetGenesisAccountBalance(validatorInfo.Address, allocs)
		if err != nil {
			return nil, err
		}

		blsKey, err := validatorInfo.UnmarshalBLSPublicKey()
		if err != nil {
			return nil, err
		}

		validatorSetMap[i] = map[string]interface{}{
			"_address":    validatorInfo.Address,
			"blsKey":      blsKey.ToBigInt(),
			"votingPower": chain.ConvertWeiToTokensAmount(genesisBalance),
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
