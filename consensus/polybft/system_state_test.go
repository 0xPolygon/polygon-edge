package polybft

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
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

func TestSystemState_GetValidatorSet(t *testing.T) {
	t.Parallel()

	cc := &testutil.Contract{}
	cc.AddCallback(func() string {
		return `

		function getCurrentValidatorSet() public returns (address[] memory) {
			address[] memory addresses = new address[](1);
			addresses[0] = address(1);
			return addresses;
		}

		function getValidator(
			address validator
		)
			external
			view
			returns (
				uint256[4] memory blsKey,
				uint256 stake,
				uint256 totalStake,
				uint256 commission,
				uint256 withdrawableRewards,
				bool active
			)
		{
			blsKey = [
				1708568697487735112380375954529256823287318886168633341382922712646533763844,
				14713639476280042449606484361428781226013866637570951139712205035697871856089,
				16798350082249088544573448433070681576641749462807627179536437108134609634615,
				21427200503135995176566340351867145775962083994845221446131416289459495591422
			];
			stake = 10;
			totalStake = 15;
			commission = 20;
			withdrawableRewards = 30;
			active = true;
		}
		`
	})

	solcContract, err := cc.Compile()
	assert.NoError(t, err)

	bin, err := hex.DecodeString(solcContract.Bin)
	assert.NoError(t, err)

	transition := newTestTransition(t, nil)

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
	assert.Equal(t, new(big.Int).SetUint64(15), validators[0].VotingPower)
}

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

	systemState := NewSystemState(&PolyBFTConfig{StateReceiverAddr: result.Address}, provider)

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

func TestStateProvider_Txn_NotSupported(t *testing.T) {
	t.Parallel()

	transition := newTestTransition(t, nil)

	provider := &stateProvider{
		transition: transition,
	}

	require.PanicsWithError(t, errSendTxnUnsupported.Error(),
		func() { _, _ = provider.Txn(ethgo.ZeroAddress, createTestKey(t), []byte{0x1}) })
}

func newTestTransition(t *testing.T, alloc map[types.Address]*chain.GenesisAccount) *state.Transition {
	t.Helper()

	st := itrie.NewState(itrie.NewMemoryStorage())

	ex := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
	}, st, hclog.NewNullLogger())

	rootHash := ex.WriteGenesis(alloc)

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

func Test_buildLogsFromReceipts(t *testing.T) {
	t.Parallel()

	defaultHeader := &types.Header{
		Number: 100,
	}

	type args struct {
		entry  []*types.Receipt
		header *types.Header
	}

	data := map[string]interface{}{
		"Hash":   defaultHeader.Hash,
		"Number": defaultHeader.Number,
	}

	dataArray, err := json.Marshal(&data)
	require.NoError(t, err)

	tests := []struct {
		name string
		args args
		want []*types.Log
	}{
		{
			name: "no entries provided",
		},
		{
			name: "successfully created logs",
			args: args{
				entry: []*types.Receipt{
					{
						Logs: []*types.Log{
							{
								Address: types.BytesToAddress([]byte{0, 1}),
								Topics:  nil,
								Data:    dataArray,
							},
						},
					},
				},
				header: defaultHeader,
			},
			want: []*types.Log{
				{
					Address: types.BytesToAddress([]byte{0, 1}),
					Topics:  nil,
					Data:    dataArray,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.EqualValuesf(t,
				tt.want,
				buildLogsFromReceipts(tt.args.entry, tt.args.header),
				"buildLogsFromReceipts(%v, %v)", tt.args.entry, tt.args.header,
			)
		})
	}
}
