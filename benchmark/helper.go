package benchmark

import (
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
)

var (
	singleContCalcFunc    = contractsapi.TestBenchmarkSingle.Abi.Methods["compute"]
	singleContGetFunc     = contractsapi.TestBenchmarkSingle.Abi.Methods["getValue"]
	singleContSetFunc     = contractsapi.TestBenchmarkSingle.Abi.Methods["addValue"]
	multiContSetAAddrFunc = contractsapi.TestBenchmarkA.Abi.Methods["setContractAddr"]
	multiContSetBAddrFunc = contractsapi.TestBenchmarkA.Abi.Methods["setContractAddr"]
	multiContFnA          = contractsapi.TestBenchmarkA.Abi.Methods["fnA"]
)

// deployContractOnRootAndChild deploys contract code on both root and child chain
func deployContractOnRootAndChild(
	b *testing.B,
	childTxRelayer txrelayer.TxRelayer,
	rootTxRelayer txrelayer.TxRelayer,
	sender ethgo.Key,
	byteCode []byte) (ethgo.Address, ethgo.Address) {
	b.Helper()

	// deploy contract on the child chain
	contractChildAddr := deployContract(b, childTxRelayer, sender, byteCode)

	// deploy contract on the root chain
	contractRootAddr := deployContract(b, rootTxRelayer, sender, byteCode)

	return contractChildAddr, contractRootAddr
}

// deployContract deploys contract code for the given relayer
func deployContract(b *testing.B, txRelayer txrelayer.TxRelayer, sender ethgo.Key, byteCode []byte) ethgo.Address {
	b.Helper()

	txn := &ethgo.Transaction{
		To:    nil, // contract deployment
		Input: byteCode,
	}

	receipt, err := txRelayer.SendTransaction(txn, sender)
	require.NoError(b, err)
	require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
	require.NotEqual(b, ethgo.ZeroAddress, receipt.ContractAddress)

	return receipt.ContractAddress
}

// getTxInput returns input for sending tx, given the abi encoded method and call parameters
func getTxInput(b *testing.B, method *abi.Method, args interface{}) []byte {
	b.Helper()

	var (
		input []byte
		err   error
	)

	if args != nil {
		input, err = method.Encode(args)
	} else {
		input = method.ID()
	}

	require.NoError(b, err)

	return input
}

// setContractDependencyAddress calls setContract function on caller contract, to set address of the callee contract
func setContractDependencyAddress(b *testing.B, txRelayer txrelayer.TxRelayer, callerContractAddr ethgo.Address,
	calleeContractAddr ethgo.Address, setContractAbiMethod *abi.Method, sender ethgo.Key) {
	b.Helper()

	input := getTxInput(b, setContractAbiMethod, []interface{}{calleeContractAddr})
	receipt, err := txRelayer.SendTransaction(
		&ethgo.Transaction{
			To:    &callerContractAddr,
			Input: input,
		}, sender)
	require.NoError(b, err)
	require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
}

// getPrivateKey initializes a private key from provided raw private key
func getPrivateKey(b *testing.B, privateKeyRaw string) ethgo.Key {
	b.Helper()

	dec, err := hex.DecodeString(privateKeyRaw)
	require.NoError(b, err)

	privateKey, err := wallet.NewWalletFromPrivKey(dec)
	require.NoError(b, err)

	return privateKey
}

func transitionDeployContract(b *testing.B, transition *state.Transition, byteCode []byte,
	sender types.Address) types.Address {
	b.Helper()

	deployResult := transition.Create2(sender, byteCode, big.NewInt(0), 1e9)
	require.NoError(b, deployResult.Err)

	return deployResult.Address
}

func transitionCallContract(b *testing.B, transition *state.Transition, contractAddress types.Address,
	sender types.Address, input []byte) *runtime.ExecutionResult {
	b.Helper()

	result := transition.Call2(sender, contractAddress, input, big.NewInt(0), 1e9)
	require.NoError(b, result.Err)

	return result
}

func newTestTransition(b *testing.B, alloc map[types.Address]*chain.GenesisAccount, disk bool) *state.Transition {
	b.Helper()

	var st *itrie.State

	if disk {
		testDir := createTestTempDirectory(b)
		stateStorage, err := itrie.NewLevelDBStorage(filepath.Join(testDir, "trie"), hclog.NewNullLogger())
		require.NoError(b, err)

		st = itrie.NewState(stateStorage)
	} else {
		st = itrie.NewState(itrie.NewMemoryStorage())
	}

	ex := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
		BurnContract: map[uint64]string{
			0: types.ZeroAddress.String(),
		},
	}, st, hclog.NewNullLogger())

	rootHash, err := ex.WriteGenesis(alloc, types.Hash{})
	require.NoError(b, err)

	ex.GetHash = func(h *types.Header) state.GetHashByNumber {
		return func(i uint64) types.Hash {
			return rootHash
		}
	}

	transition, err := ex.BeginTxn(
		rootHash,
		&types.Header{},
		types.ZeroAddress,
	)
	require.NoError(b, err)

	return transition
}

func createTestTempDirectory(b *testing.B) string {
	b.Helper()

	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		b.Logf("failed to create temp directory, err=%+v", err)

		b.FailNow()
	}

	b.Cleanup(func() {
		os.RemoveAll(path)
	})

	return path
}
