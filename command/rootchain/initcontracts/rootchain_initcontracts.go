package initcontracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
)

const receiptStatusOk = 0

var (
	params initContractsParams

	initCheckpointManager, _ = abi.NewMethod("function initialize(" +
		// BLS contract address
		"address blsContract," +
		// BN256G2 contract address
		"address bn256g2Contract," +
		// RootValidatorSet contract address
		"address rootValidatorSetContract," +
		// domain used for BLS signing
		"bytes32 domain)")

	initRootValidatorSet, _ = abi.NewMethod("function initialize(" +
		// governance account address
		"address governance," +
		// CheckpointManager contract address
		"address newCheckpointManager," +
		// genesis validator set addresses
		"address[] validatorAddresses," +
		// genesis validator set public keys
		"uint256[4][] validatorPubkeys)")

	bn256P, _ = new(big.Int).SetString("21888242871839275222246405745257275088696311157297823662689037894645226208583", 10)
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
		defaultContractsPath,
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
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if ok, err := helper.ExistsCode(contracts.SystemCaller); err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy: %w", err))

		return
	} else if ok {
		outputter.SetCommandResult(&result{AlreadyDeployed: true})

		return
	}

	if err := deployContracts(); err != nil {
		outputter.SetError(fmt.Errorf("failed to deploy: %w", err))

		return
	}

	outputter.SetCommandResult(&result{AlreadyDeployed: false})
}

func deployContracts() error {
	// if the bridge contract is not created, we have to deploy all the contracts
	// fund account
	if _, err := helper.FundAccount(helper.GetDefAccount()); err != nil {
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
			expected: helper.RootchainBridgeAddress,
		},
		{
			name:     "CheckpointManager",
			path:     "root/CheckpointManager.sol",
			expected: helper.CheckpointManagerAddress,
		},
		{
			name:     "RootValidatorSet",
			path:     "root/RootValidatorSet.sol",
			expected: helper.RootValidatorSetAddress,
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

	pendingNonce, err := helper.GetPendingNonce(helper.GetDefAccount())
	if err != nil {
		return err
	}

	for i, contract := range deployContracts {
		bytecode, err := readContractBytecode(params.contractsPath, contract.path, contract.name)
		if err != nil {
			return err
		}

		txn := &ethgo.Transaction{
			To:    nil, // contract deployment
			Input: bytecode,
		}

		receipt, err := helper.SendTxn(pendingNonce+uint64(i), txn)
		if err != nil {
			return err
		}

		if types.Address(receipt.ContractAddress) != contract.expected {
			return fmt.Errorf("wrong deployed address for contract %s: expected %s but found %s",
				contract.name, contract.expected, receipt.ContractAddress)
		}
	}

	pendingNonce += uint64(len(deployContracts))

	if err := initializeCheckpointManager(pendingNonce); err != nil {
		return err
	}

	pendingNonce += 1

	if err := initializeRootValidatorSet(pendingNonce); err != nil {
		return err
	}

	return nil
}

// initializeCheckpointManager invokes initialize function on CheckpointManager smart contract
func initializeCheckpointManager(nonce uint64) error {
	initCheckpointInput, err := initCheckpointManager.Encode(
		[]interface{}{
			helper.BLSAddress,
			helper.BN256G2Address,
			helper.RootValidatorSetAddress,
			bn256P.Bytes(),
		})

	if err != nil {
		return fmt.Errorf("failed to encode parameters for CheckpointManager.initialize. error: %w", err)
	}

	checkpointManagerAddress := ethgo.Address(helper.CheckpointManagerAddress)
	txn := &ethgo.Transaction{
		To:    &checkpointManagerAddress,
		Input: initCheckpointInput,
	}

	receipt, err := helper.SendTxn(nonce, txn)
	if err != nil {
		return fmt.Errorf("failed to send transaction to CheckpointManager. error: %w", err)
	}

	if receipt.Status != receiptStatusOk {
		return errors.New("failed to initialize CheckpointManager")
	}

	return nil
}

// initializeCheckpointManager invokes initialize function on CheckpointManager smart contract
func initializeRootValidatorSet(nonce uint64) error {
	validatorsInfo, err := genesis.ReadValidatorsByRegexp(path.Dir(params.validatorPath), params.validatorPrefixPath)
	if err != nil {
		return err
	}

	validatorPubKeys := make([][4]*big.Int, len(validatorsInfo))
	validatorAddresses := make([]types.Address, len(validatorsInfo))

	for i, valid := range validatorsInfo {
		pubKeyBig, err := valid.Account.Bls.PublicKey().ToBigInt()
		if err != nil {
			return fmt.Errorf("failed to encode pubkey for RootValidatorSet.initialize. error: %w", err)
		}

		validatorAddresses[i] = types.Address(valid.Account.Ecdsa.Address())
		validatorPubKeys[i] = pubKeyBig
	}

	initRootValidatorSetInput, err := initRootValidatorSet.Encode([]interface{}{
		helper.GetDefAccount(),
		helper.CheckpointManagerAddress,
		validatorAddresses,
		validatorPubKeys,
	})
	if err != nil {
		return fmt.Errorf("failed to encode parameters for RootValidatorSet.initialize. error: %w", err)
	}

	checkpointManagerAddress := ethgo.BytesToAddress(helper.RootValidatorSetAddress.Bytes())
	txn := &ethgo.Transaction{
		To:    &checkpointManagerAddress,
		Input: initRootValidatorSetInput,
	}

	receipt, err := helper.SendTxn(nonce, txn)
	if err != nil {
		return fmt.Errorf("failed to send transaction to RootValidatorSet. error: %w", err)
	}

	if receipt.Status != receiptStatusOk {
		return errors.New("failed to initialize RootValidatorSet")
	}

	return nil
}

func readContractBytecode(rootPath, contractPath, contractName string) ([]byte, error) {
	_, fileName := filepath.Split(contractPath)

	absolutePath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(absolutePath, contractPath, strings.TrimSuffix(fileName, ".sol")+".json")

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var artifact struct {
		// Abi              *abi.ABI
		Bytecode         string
		DeployedBytecode string
	}

	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}

	return hex.MustDecodeHex(artifact.DeployedBytecode), nil
}
