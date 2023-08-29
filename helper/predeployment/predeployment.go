package predeployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"

	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	errABINotFound              = errors.New("abi field not found in specified JSON")
	errBytecodeNotFound         = errors.New("bytecode field not found in specified JSON")
	errDeployedBytecodeNotFound = errors.New("deployed bytecode field not found in specified JSON")
)

const (
	abiValue              = "abi"
	deployedBytecodeValue = "deployedBytecode"
	bytecodeValue         = "bytecode"
)

type contractArtifact struct {
	ABI              []byte // the ABI of the Smart Contract
	Bytecode         []byte // the raw bytecode of the Smart Contract
	DeployedBytecode []byte // the deployed bytecode of the Smart Contract
}

// loadContractArtifact loads contract artifacts based on the
// passed in Smart Contract JSON ABI from json file
func loadContractArtifact(filepath string) (*contractArtifact, error) {
	// Read from the ABI from the JSON file
	jsonRaw, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	// Fill out the fields in the JSON file
	var jsonResult map[string]interface{}
	if err = json.Unmarshal(jsonRaw, &jsonResult); err != nil {
		return nil, err
	}

	// Parse the ABI
	abiRaw, ok := jsonResult[abiValue]
	if !ok {
		return nil, errABINotFound
	}

	abiBytes, err := json.Marshal(abiRaw)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ABI to JSON, %w", err)
	}

	// Parse the bytecode
	bytecode, ok := jsonResult[bytecodeValue].(string)
	if !ok {
		return nil, errBytecodeNotFound
	}

	hexBytecode, err := hex.DecodeHex(bytecode)
	if err != nil {
		return nil, fmt.Errorf("unable to decode bytecode, %w", err)
	}

	// Parse deployed bytecode
	deployedBytecode, ok := jsonResult[deployedBytecodeValue].(string)
	if !ok {
		return nil, errDeployedBytecodeNotFound
	}

	hexDeployedBytecode, err := hex.DecodeHex(deployedBytecode)
	if err != nil {
		return nil, fmt.Errorf("unable to decode deployed bytecode, %w", err)
	}

	return &contractArtifact{
		ABI:              abiBytes,
		Bytecode:         hexBytecode,
		DeployedBytecode: hexDeployedBytecode,
	}, nil
}

// getModifiedStorageMap fetches the modified storage map for the specified address
func getModifiedStorageMap(radix *state.Txn, address types.Address) map[types.Hash]types.Hash {
	storageMap := make(map[types.Hash]types.Hash)

	radix.GetRadix().Root().Walk(func(k []byte, v interface{}) bool {
		if types.BytesToAddress(k) != address {
			// Ignore all addresses that are not the one the predeployment
			// is meant to run for
			return false
		}

		obj, _ := v.(*state.StateObject)
		obj.Txn.Root().Walk(func(k []byte, v interface{}) bool {
			val, _ := v.([]byte)
			storageMap[types.BytesToHash(k)] = types.BytesToHash(val)

			return false
		})

		return true
	})

	return storageMap
}

func getPredeployAccount(address types.Address, input []byte, chainID int64) (*chain.GenesisAccount, error) {
	// Create an instance of the state
	st := itrie.NewState(itrie.NewMemoryStorage())

	// Create a snapshot
	snapshot := st.NewSnapshot()

	// Create a radix
	radix := state.NewTxn(snapshot)

	// Create the contract object for the EVM
	contract := runtime.NewContractCreation(
		1,
		types.ZeroAddress,
		types.ZeroAddress,
		address,
		big.NewInt(0),
		math.MaxInt64,
		input,
	)

	// Enable all forks
	config := chain.AllForksEnabled.At(0)

	// Create a transition
	transition := state.NewTransition(config, snapshot, radix)
	transition.ContextPtr().ChainID = chainID

	// Run the transition through the EVM
	res := evm.NewEVM().Run(contract, transition, &config)
	if res.Err != nil {
		return nil, fmt.Errorf("EVM predeployment failed, %w", res.Err)
	}

	// After the execution finishes,
	// the state needs to be walked to collect all touched all storage slots
	storageMap := getModifiedStorageMap(radix, address)

	_, _, err := transition.Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to commit the state changes: %w", err)
	}

	return &chain.GenesisAccount{
		Balance: transition.GetBalance(address),
		Nonce:   transition.GetNonce(address),
		Code:    res.ReturnValue,
		Storage: storageMap,
	}, nil
}

// GenerateGenesisAccountFromFile generates an account that is going to be directly
// inserted into state
func GenerateGenesisAccountFromFile(
	filepath string,
	constructorArgs []string,
	predeployAddress types.Address,
	chainID int64,
) (*chain.GenesisAccount, error) {
	// Create the artifact from JSON
	artifact, err := loadContractArtifact(filepath)
	if err != nil {
		return nil, err
	}

	// Generate the contract ABI object
	contractABI, err := abi.NewABI(string(artifact.ABI))
	if err != nil {
		return nil, fmt.Errorf("unable to create contract ABI, %w", err)
	}

	finalBytecode := artifact.Bytecode
	constructorInfo := contractABI.Constructor

	if constructorInfo != nil {
		// Constructor arguments are passed in as an array of values.
		// Structs are treated as sub-arrays with their corresponding values laid out
		// in ABI encoding
		parsedArguments, err := ParseArguments(constructorArgs)
		if err != nil {
			return nil, err
		}

		// Encode the constructor params
		constructor, err := abi.Encode(
			parsedArguments,
			contractABI.Constructor.Inputs,
		)
		if err != nil {
			return nil, fmt.Errorf("unable to encode constructor arguments, %w", err)
		}

		finalBytecode = append(artifact.Bytecode, constructor...)
	}

	return getPredeployAccount(predeployAddress, finalBytecode, chainID)
}
