package polybft

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/contract"
	"github.com/umbracle/ethgo/testutil"
)

func TestSystemState_GetNextCommittedIndex(t *testing.T) {
	t.Parallel()

	var sideChainBridgeABI, _ = abi.NewMethod(
		"function setNextCommittedIndex(uint256 _index) public payable",
	)

	cc := &testutil.Contract{}
	cc.AddCallback(func() string {
		return `
		uint256 public lastCommittedId;
		
		function setNextCommittedIndex(uint256 _index) public payable {
			lastCommittedId = _index;
		}`
	})

	solcContract, err := cc.Compile()
	require.NoError(t, err)

	bin, err := hex.DecodeString(solcContract.Bin)
	require.NoError(t, err)

	transition := newTestTransition(t, nil)

	// deploy a contract
	result := transition.Create2(types.Address{}, bin, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)

	provider := &stateProvider{
		transition: transition,
	}

	systemState := NewSystemState(contracts.ValidatorSetContract, result.Address, provider)

	expectedNextCommittedIndex := uint64(45)
	input, err := sideChainBridgeABI.Encode([1]interface{}{expectedNextCommittedIndex})
	assert.NoError(t, err)

	_, err = provider.Call(ethgo.Address(result.Address), input, &contract.CallOpts{})
	assert.NoError(t, err)

	nextCommittedIndex, err := systemState.GetNextCommittedIndex()
	assert.NoError(t, err)
	assert.Equal(t, expectedNextCommittedIndex+1, nextCommittedIndex)
}

func TestSystemState_GetEpoch(t *testing.T) {
	t.Parallel()

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

	transition := newTestTransition(t, nil)

	// deploy a contract
	result := transition.Create2(types.Address{}, bin, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)

	provider := &stateProvider{
		transition: transition,
	}

	systemState := NewSystemState(result.Address, contracts.StateReceiverContract, provider)

	expectedEpoch := uint64(50)
	input, err := setEpochMethod.Encode([1]interface{}{expectedEpoch})
	require.NoError(t, err)

	_, err = provider.Call(ethgo.Address(result.Address), input, &contract.CallOpts{})
	require.NoError(t, err)

	epoch, err := systemState.GetEpoch()
	require.NoError(t, err)
	require.Equal(t, expectedEpoch, epoch)
}

func TestStateProvider_Txn_NotSupported(t *testing.T) {
	t.Parallel()

	transition := newTestTransition(t, nil)

	provider := &stateProvider{
		transition: transition,
	}

	_, err := provider.Txn(ethgo.ZeroAddress, createTestKey(t), []byte{0x1})
	require.ErrorIs(t, err, errSendTxnUnsupported)
}

func newTestTransition(t *testing.T, alloc map[types.Address]*chain.GenesisAccount) *state.Transition {
	t.Helper()

	st := itrie.NewState(itrie.NewMemoryStorage())

	ex := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
		BurnContract: map[uint64]types.Address{
			0: types.ZeroAddress,
		},
	}, st, hclog.NewNullLogger())

	rootHash, err := ex.WriteGenesis(alloc, types.Hash{})
	require.NoError(t, err)

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
