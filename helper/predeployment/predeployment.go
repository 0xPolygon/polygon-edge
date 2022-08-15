package predeployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
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

// generateContractArtifact generates contract artifacts based on the
// passed in Smart Contract JSON ABI
func generateContractArtifact(filepath string) (*contractArtifact, error) {
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

// getArraySubstring fetches the substring of anything
// that is between [ and ]
func getArraySubstring(s string) string {
	// Find the first occurrence of the opening bracket
	i := strings.Index(s, "[")
	if i >= 0 {
		// Find last occurrence of the closing bracket
		j := strings.LastIndex(s, "]")
		if j >= 0 {
			return s[i+1 : j]
		}
	}

	return ""
}

// extractValue recursively extracts the values for subarrays
// and packs them
func extractValue(s string) interface{} {
	if !strings.Contains(s, "[") {
		return s
	}

	ret := make([]interface{}, 0)
	arraySubstring := getArraySubstring(s)

	if arraySubstring != "" {
		splitValues := strings.Split(arraySubstring, ",")

		for _, str := range splitValues {
			ret = append(ret, extractValue(str))
		}
	}

	return ret
}

// normalizeConstructorArguments cleans up the constructor arguments so that they
// can be interpreted by the ABI encoder
func normalizeConstructorArguments(constructorArgs []string) []interface{} {
	arguments := make([]interface{}, 0)

	for _, arg := range constructorArgs {
		arguments = append(arguments, extractValue(arg))
	}

	return arguments
}

// GenerateGenesisAccountFromFile generates an account that is going to be directly
// inserted into state
func GenerateGenesisAccountFromFile(
	filepath string,
	constructorArgs []string,
	predeployAddress types.Address,
) (*chain.GenesisAccount, error) {
	// Create the artifact from JSON
	artifact, err := generateContractArtifact(filepath)
	if err != nil {
		return nil, err
	}

	// Generate the contract ABI object
	contractABI, err := abi.NewABI(string(artifact.ABI))
	if err != nil {
		return nil, fmt.Errorf("unable to create contract ABI, %w", err)
	}

	// Encode the constructor params
	constructor, err := abi.Encode(
		// Constructor arguments are passed in as an array of values.
		// Structs are treated as sub-arrays with their corresponding values laid out
		// in ABI encoding
		normalizeConstructorArguments(constructorArgs),
		contractABI.Constructor.Inputs,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to encode constructor arguments, %w", err)
	}

	finalBytecode := append(artifact.Bytecode, constructor...)

	// Create an instance of the state
	st := itrie.NewState(itrie.NewMemoryStorage())

	// Create a snapshot
	snapshot := st.NewSnapshot()

	// Create a radix
	radix := state.NewTxn(st, snapshot)

	// Create the contract object for the EVM
	contract := runtime.NewContractCreation(
		1,
		types.ZeroAddress,
		types.ZeroAddress,
		predeployAddress,
		big.NewInt(0),
		math.MaxInt64,
		finalBytecode,
	)

	// Enable all forks
	config := chain.AllForksEnabled.At(0)

	// Create a transition
	transition := state.NewTransition(config, radix)

	// Run the transition through the EVM
	res := evm.NewEVM().Run(contract, transition, &config)
	if res.Err != nil {
		return nil, fmt.Errorf("EVM predeployment failed, %w", res.Err)
	}

	// After the execution finishes,
	// the state needs to be walked to collect all touched
	// storage slots
	storageMap := make(map[types.Hash]types.Hash)

	radix.GetRadix().Root().Walk(func(k []byte, v interface{}) bool {
		if types.BytesToAddress(k) != predeployAddress {
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

	transition.Commit()

	return &chain.GenesisAccount{
		Balance: transition.GetBalance(predeployAddress),
		Nonce:   transition.GetNonce(predeployAddress),
		Code:    artifact.DeployedBytecode,
		Storage: storageMap,
	}, nil
}
