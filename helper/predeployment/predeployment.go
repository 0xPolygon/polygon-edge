package predeployment

import (
	"fmt"
	"math"
	"math/big"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
)

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

func getPredeployAccount(address types.Address, input []byte,
	chainID int64, deployer types.Address) (*chain.GenesisAccount, error) {
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
		deployer,
		address,
		big.NewInt(0),
		math.MaxInt64,
		input,
	)

	// Enable all forks
	config := chain.AllForksEnabled.At(0)

	// Create a transition
	transition := state.NewTransition(hclog.NewNullLogger(), config, snapshot, radix)
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
func GenerateGenesisAccountFromFile(scArtifact *contracts.Artifact, constructorArgs []string,
	predeployAddress types.Address, chainID int64, deployer types.Address,
) (*chain.GenesisAccount, error) {
	finalBytecode := scArtifact.Bytecode
	constructorInfo := scArtifact.Abi.Constructor

	if constructorInfo != nil {
		// Constructor arguments are passed in as an array of values.
		// Structs are treated as sub-arrays with their corresponding values laid out
		// in ABI encoding
		parsedArguments, err := ParseArguments(constructorArgs)
		if err != nil {
			return nil, err
		}

		// Encode the constructor params
		constructor, err := abi.Encode(parsedArguments, constructorInfo.Inputs)
		if err != nil {
			return nil, fmt.Errorf("unable to encode constructor arguments, %w", err)
		}

		finalBytecode = append(scArtifact.Bytecode, constructor...)
	}

	return getPredeployAccount(predeployAddress, finalBytecode, chainID, deployer)
}
