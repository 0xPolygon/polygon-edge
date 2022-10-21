package polybft

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/contract"
	"github.com/umbracle/ethgo/testutil"
)

func TestSystemState_GetValidatorSet(t *testing.T) {
	cc := &testutil.Contract{}

	cc.AddCallback(func() string {
		return `

		struct Validator {
			uint256[4] id;
			uint256 sender;
			uint256 receiver;
			uint256 data;
		}

		function getCurrentValidatorSet() public returns (address[] memory) {
			address[] memory addresses = new address[](1);
			addresses[0] = address(1);
			return addresses;
		}

		function getValidator(address) public returns (Validator memory){
			uint[4] memory key = [
				1708568697487735112380375954529256823287318886168633341382922712646533763844,
				14713639476280042449606484361428781226013866637570951139712205035697871856089,
				16798350082249088544573448433070681576641749462807627179536437108134609634615,
				21427200503135995176566340351867145775962083994845221446131416289459495591422
			];
			return Validator(key, 0, 0, 0);
		}

		`
	})

	solcContract, err := cc.Compile()
	assert.NoError(t, err)

	bin, err := hex.DecodeString(solcContract.Bin)
	assert.NoError(t, err)

	transition := newTestTransition(t)

	// deploy a contract
	result := transition.Create2(types.Address{}, bin, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)

	provider := &stateProvider{
		transition: transition,
	}

	st := NewSystemState(&PolyBFTConfig{ValidatorSetAddr: result.Address}, provider)
	validators, err := st.GetValidatorSet()
	assert.NoError(t, err)
	assert.Equal(t, types.Address(ethgo.HexToAddress("1")), validators[0].Address)
}

func TestSystemState_GetNextExecutionAndCommittedIndex(t *testing.T) {
	var sideChainBridgeABI, _ = abi.NewABIFromList([]string{
		"function setNextExecutionIndex(uint256 _index) public payable",
		"function setNextCommittedIndex(uint256 _index) public payable",
	})

	cc := &testutil.Contract{}
	cc.AddCallback(func() string {
		return `
		uint256 public counter;
		uint256 public lastCommittedId;
		
		function setNextExecutionIndex(uint256 _index) public payable {
			counter = _index;
		}
		function setNextCommittedIndex(uint256 _index) public payable {
			lastCommittedId = _index;
		}`
	})

	solcContract, err := cc.Compile()
	require.NoError(t, err)

	bin, err := hex.DecodeString(solcContract.Bin)
	require.NoError(t, err)

	transition := newTestTransition(t)

	// deploy a contract
	result := transition.Create2(types.Address{}, bin, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)

	provider := &stateProvider{
		transition: transition,
	}

	systemState := NewSystemState(&PolyBFTConfig{StateReceiverAddr: result.Address}, provider)
	nextExecutionIndex, err := systemState.GetNextExecutionIndex()
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), nextExecutionIndex)

	expectedNextExecutionIndex := uint64(30)
	input, err := sideChainBridgeABI.GetMethod("setNextExecutionIndex").Encode([1]interface{}{expectedNextExecutionIndex})
	assert.NoError(t, err)

	_, err = provider.Call(ethgo.Address(result.Address), input, &contract.CallOpts{})
	assert.NoError(t, err)

	nextExecutionIndex, err = systemState.GetNextExecutionIndex()
	assert.NoError(t, err)
	assert.Equal(t, expectedNextExecutionIndex+1, nextExecutionIndex)

	expectedNextCommittedIndex := uint64(45)
	input, err = sideChainBridgeABI.GetMethod("setNextCommittedIndex").Encode([1]interface{}{expectedNextCommittedIndex})
	assert.NoError(t, err)

	_, err = provider.Call(ethgo.Address(result.Address), input, &contract.CallOpts{})
	assert.NoError(t, err)

	nextCommittedIndex, err := systemState.GetNextCommittedIndex()
	assert.NoError(t, err)
	assert.Equal(t, expectedNextCommittedIndex+1, nextCommittedIndex)
}

func TestSystemState_GetEpoch(t *testing.T) {
	setEpochMethod, err := abi.NewMethod("function setEpoch(uint256 _epochId) public payable")
	require.NoError(t, err)

	cc := &testutil.Contract{}
	cc.AddCallback(func() string {
		return `
			uint256 public currentEpochId;
			
			function setEpoch(uint256 _epochId) public payable {
				currentEpochId = _epochId;
			}
			`
	})

	solcContract, err := cc.Compile()
	require.NoError(t, err)

	bin, err := hex.DecodeString(solcContract.Bin)
	require.NoError(t, err)

	transition := newTestTransition(t)

	// deploy a contract
	result := transition.Create2(types.Address{}, bin, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)

	provider := &stateProvider{
		transition: transition,
	}

	systemState := NewSystemState(&PolyBFTConfig{ValidatorSetAddr: result.Address}, provider)

	expectedEpoch := uint64(50)
	input, err := setEpochMethod.Encode([1]interface{}{expectedEpoch})
	require.NoError(t, err)

	_, err = provider.Call(ethgo.Address(result.Address), input, &contract.CallOpts{})
	require.NoError(t, err)

	epoch, err := systemState.GetEpoch()
	require.NoError(t, err)
	require.Equal(t, expectedEpoch, epoch)
}

func TestStateProvider_Txn_Panics(t *testing.T) {
	transition := newTestTransition(t)

	provider := &stateProvider{
		transition: transition,
	}

	key := createTestKey(t)
	require.Panics(t, func() { _, _ = provider.Txn(ethgo.ZeroAddress, key, []byte{0x1}) })
}

func newTestTransition(t *testing.T) *state.Transition {
	t.Helper()

	st := itrie.NewState(itrie.NewMemoryStorage())

	ex := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
	}, st, hclog.NewNullLogger())

	rootHash := ex.WriteGenesis(nil)

	ex.SetRuntime(evm.NewEVM())
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
	assert.NoError(t, err)

	return transition
}
