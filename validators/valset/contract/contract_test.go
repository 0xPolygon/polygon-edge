package contract

import (
	"errors"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var (
	errTest = errors.New("error by test")

	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")

	testBLSPubKey1 = newTestBLSKeyBytes()
	testBLSPubKey2 = newTestBLSKeyBytes()

	testPredeployParams = stakingHelper.PredeployParams{
		MinValidatorCount: 0,
		MaxValidatorCount: 10,
	}
	testBlockGasLimit uint64 = 10000000
)

func newTestBLSKeyBytes() validators.BLSValidatorPublicKey {
	key, err := crypto.GenerateBLSKey()
	if err != nil {
		return nil
	}

	pubKey, err := key.GetPublicKey()
	if err != nil {
		return nil
	}

	buf, err := pubKey.MarshalBinary()
	if err != nil {
		return nil
	}

	return buf
}

type mockExecutor struct {
	BeginTxnFn func(types.Hash, *types.Header, types.Address) (*state.Transition, error)
}

func (m *mockExecutor) BeginTxn(
	hash types.Hash,
	header *types.Header,
	address types.Address,
) (*state.Transition, error) {
	return m.BeginTxnFn(hash, header, address)
}

type mockSigner struct {
	signer.Signer

	TypeVal    validators.ValidatorType
	AddressVal types.Address
}

func (m *mockSigner) Type() validators.ValidatorType {
	return m.TypeVal
}

func (m *mockSigner) Address() types.Address {
	return m.AddressVal
}

func newTestTransitionWithPredeployedStakingContract(
	t *testing.T,
	validators validators.Validators,
) *state.Transition {
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
		&types.Header{
			// Set enough block gas limit for query
			GasLimit: testBlockGasLimit,
		},
		types.ZeroAddress,
	)
	assert.NoError(t, err)

	contractState, err := stakingHelper.PredeployStakingSC(
		validators,
		testPredeployParams,
	)

	assert.NoError(t, err)

	assert.NoError(
		t,
		transition.SetAccountDirectly(staking.AddrStakingContract, contractState),
	)

	return transition
}

func NewTestContractSet(
	blockchain valset.HeaderGetter,
	executor Executor,
	getSigner valset.SignerGetter,
) *ContractValidatorSet {
	return &ContractValidatorSet{
		logger:     hclog.NewNullLogger(),
		epochSize:  valset.TestEpochSize,
		blockchain: blockchain,
		executor:   executor,
		getSigner:  getSigner,
	}
}

func TestContractValidatorSetSourceType(t *testing.T) {
	s := NewTestContractSet(nil, nil, nil)

	assert.Equal(t, valset.Contract, s.SourceType())
}

func TestContractValidatorSetInitialize(t *testing.T) {
	s := NewTestContractSet(nil, nil, nil)

	assert.NoError(t, s.Initialize())
}

func TestContractValidatorSetGetValidators(t *testing.T) {
	t.Run("should throw error when header not found", func(t *testing.T) {
		s := NewTestContractSet(
			&valset.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return nil, false
				},
			},
			nil,
			func(x uint64) (signer.Signer, error) {
				return signer.NewSigner(nil), nil
			},
		)

		res, err := s.GetValidators(0, 0)

		assert.Nil(t, res)
		assert.Error(t, err)
	})

	t.Run("should throw error getSigner throws error", func(t *testing.T) {
		s := NewTestContractSet(
			&valset.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return &types.Header{}, true
				},
			},
			nil,
			func(x uint64) (signer.Signer, error) {
				return nil, errTest
			},
		)

		res, err := s.GetValidators(0, 0)

		assert.Nil(t, res)
		assert.ErrorIs(t, errTest, err)
	})

	t.Run("should throw error when getSigner throws ErrSignerNotFound", func(t *testing.T) {
		s := NewTestContractSet(
			&valset.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return &types.Header{}, true
				},
			},
			nil,
			func(x uint64) (signer.Signer, error) {
				return nil, nil
			},
		)

		res, err := s.GetValidators(0, 0)

		assert.Nil(t, res)
		assert.ErrorIs(t, ErrSignerNotFound, err)
	})

	t.Run("should throw error when executor throws error", func(t *testing.T) {
		s := NewTestContractSet(
			&valset.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return &types.Header{}, true
				},
			},
			&mockExecutor{
				func(h1 types.Hash, h2 *types.Header, a types.Address) (*state.Transition, error) {
					return nil, errTest
				},
			},
			func(x uint64) (signer.Signer, error) {
				return signer.NewSigner(nil), nil
			},
		)

		res, err := s.GetValidators(0, 0)

		assert.Nil(t, res)
		assert.ErrorIs(t, errTest, err)
	})

	t.Run("should fetch ECDSA Validators", func(t *testing.T) {
		var (
			stateRoot = types.StringToHash("1")
			header    = &types.Header{
				StateRoot: stateRoot,
			}

			signerAddr = types.StringToAddress("2")
			signerType = validators.ECDSAValidatorType

			vals = &validators.ECDSAValidators{
				validators.NewECDSAValidator(addr1),
				validators.NewECDSAValidator(addr2),
			}

			transition = newTestTransitionWithPredeployedStakingContract(
				t,
				vals,
			)
		)

		s := NewTestContractSet(
			&valset.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return header, true
				},
			},
			&mockExecutor{
				BeginTxnFn: func(hash types.Hash, head *types.Header, addr types.Address) (*state.Transition, error) {
					t.Helper()

					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, header, head)
					assert.Equal(t, types.ZeroAddress, addr)

					return transition, nil
				},
			},
			func(x uint64) (signer.Signer, error) {
				return &mockSigner{
					TypeVal:    signerType,
					AddressVal: signerAddr,
				}, nil
			},
		)

		res, err := s.GetValidators(0, 0)

		assert.Equal(t, vals, res)
		assert.NoError(t, err)
	})

	t.Run("should fetch BLS Validators", func(t *testing.T) {
		var (
			stateRoot = types.StringToHash("1")
			header    = &types.Header{
				StateRoot: stateRoot,
			}

			signerAddr = types.StringToAddress("2")
			signerType = validators.BLSValidatorType

			vals = &validators.BLSValidators{
				validators.NewBLSValidator(addr1, testBLSPubKey1),
				validators.NewBLSValidator(addr2, testBLSPubKey2),
			}

			transition = newTestTransitionWithPredeployedStakingContract(
				t,
				vals,
			)
		)

		s := NewTestContractSet(
			&valset.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return header, true
				},
			},
			&mockExecutor{
				func(hash types.Hash, head *types.Header, addr types.Address) (*state.Transition, error) {
					t.Helper()

					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, header, head)
					assert.Equal(t, types.ZeroAddress, addr)

					return transition, nil
				},
			},
			func(x uint64) (signer.Signer, error) {
				return &mockSigner{
					TypeVal:    signerType,
					AddressVal: signerAddr,
				}, nil
			},
		)

		res, err := s.GetValidators(0, 0)

		assert.Equal(t, vals, res)
		assert.NoError(t, err)
	})
}

func Test_calculateFetchingHeight(t *testing.T) {
	tests := []struct {
		name      string
		height    uint64
		epochSize uint64
		expected  uint64
	}{
		{
			name:      "height is 0",
			height:    0,
			epochSize: 10,
			expected:  0,
		},
		{
			name:      "height is 1",
			height:    1,
			epochSize: 10,
			expected:  0,
		},
		{
			name:      "height is the end of the first epoch",
			height:    9,
			epochSize: 10,
			expected:  0,
		},
		{
			name:      "height is the beginning of the second epoch",
			height:    10,
			epochSize: 10,
			expected:  9, // the end of the first epoch
		},
		{
			name:      "height is the next of beginning of the second epoch",
			height:    11,
			epochSize: 10,
			expected:  9, // the end of the first epoch
		},
		{
			name:      "height is the end of the second epoch",
			height:    19,
			epochSize: 10,
			expected:  9, // the end of the first epoch
		},
		{
			name:      "height is the beginning of the third epoch",
			height:    20,
			epochSize: 10,
			expected:  19, // the end of the first epoch
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				calculateFetchingHeight(test.height, test.epochSize, 0),
			)
		})
	}
}
