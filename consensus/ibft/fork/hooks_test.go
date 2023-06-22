package fork

import (
	"errors"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

type mockHeaderModifierStore struct {
	store.ValidatorStore

	ModifyHeaderFunc  func(*types.Header, types.Address) error
	VerifyHeaderFunc  func(*types.Header) error
	ProcessHeaderFunc func(*types.Header) error
}

func (m *mockHeaderModifierStore) ModifyHeader(header *types.Header, addr types.Address) error {
	return m.ModifyHeaderFunc(header, addr)
}

func (m *mockHeaderModifierStore) VerifyHeader(header *types.Header) error {
	return m.VerifyHeaderFunc(header)
}

func (m *mockHeaderModifierStore) ProcessHeader(header *types.Header) error {
	return m.ProcessHeaderFunc(header)
}

type mockUpdatableStore struct {
	store.ValidatorStore

	UpdateValidatorStoreFunc func(validators.Validators, uint64) error
}

func (m *mockUpdatableStore) UpdateValidatorSet(validators validators.Validators, height uint64) error {
	return m.UpdateValidatorStoreFunc(validators, height)
}

func Test_registerHeaderModifierHooks(t *testing.T) {
	t.Parallel()

	t.Run("should do nothing if validator store doesn't implement HeaderModifier", func(t *testing.T) {
		t.Parallel()

		type invalidValidatorStoreMock struct {
			store.ValidatorStore
		}

		hooks := &hook.Hooks{}
		mockStore := &invalidValidatorStoreMock{}

		registerHeaderModifierHooks(hooks, mockStore)

		assert.Equal(
			t,
			&hook.Hooks{},
			hooks,
		)
	})

	t.Run("should register functions to the hooks", func(t *testing.T) {
		t.Parallel()

		var (
			header = &types.Header{
				Number: 100,
				Hash:   types.BytesToHash(crypto.Keccak256([]byte{0x10, 0x0})),
			}
			addr = types.StringToAddress("1")

			err1 = errors.New("error 1")
			err2 = errors.New("error 1")
			err3 = errors.New("error 1")
		)

		hooks := &hook.Hooks{}
		mockStore := &mockHeaderModifierStore{
			ModifyHeaderFunc: func(h *types.Header, a types.Address) error {
				assert.Equal(t, header, h)
				assert.Equal(t, addr, a)

				return err1
			},
			VerifyHeaderFunc: func(h *types.Header) error {
				assert.Equal(t, header, h)

				return err2
			},
			ProcessHeaderFunc: func(h *types.Header) error {
				assert.Equal(t, header, h)

				return err3
			},
		}

		registerHeaderModifierHooks(hooks, mockStore)

		assert.Nil(t, hooks.ShouldWriteTransactionFunc)
		assert.Nil(t, hooks.VerifyBlockFunc)
		assert.Nil(t, hooks.PreCommitStateFunc)
		assert.Nil(t, hooks.PostInsertBlockFunc)

		assert.Equal(
			t,
			hooks.ModifyHeader(header, addr),
			err1,
		)
		assert.Equal(
			t,
			hooks.VerifyHeader(header),
			err2,
		)
		assert.Equal(
			t,
			hooks.ProcessHeader(header),
			err3,
		)
	})
}

func Test_registerUpdateValidatorsHooks(t *testing.T) {
	t.Parallel()

	var (
		vals = validators.NewECDSAValidatorSet(
			validators.NewECDSAValidator(types.StringToAddress("1")),
			validators.NewECDSAValidator(types.StringToAddress("2")),
		)
	)

	t.Run("should do nothing if validator store doesn't implement Updatable", func(t *testing.T) {
		t.Parallel()

		type invalidValidatorStoreMock struct {
			store.ValidatorStore
		}

		hooks := &hook.Hooks{}
		mockStore := &invalidValidatorStoreMock{}

		registerUpdateValidatorsHooks(hooks, mockStore, vals, 0)

		assert.Equal(
			t,
			&hook.Hooks{},
			hooks,
		)
	})

	t.Run("should register UpdateValidatorSet to the hooks", func(t *testing.T) {
		t.Parallel()

		var (
			fromHeight uint64 = 10
			err               = errors.New("test")

			block = &types.Block{
				Header:       &types.Header{},
				Transactions: []*types.Transaction{},
				Uncles:       []*types.Header{},
			}
		)

		hooks := &hook.Hooks{}
		mockStore := &mockUpdatableStore{
			UpdateValidatorStoreFunc: func(v validators.Validators, h uint64) error {
				assert.Equal(t, vals, v)
				assert.Equal(t, fromHeight, h)

				return err
			},
		}

		registerUpdateValidatorsHooks(hooks, mockStore, vals, fromHeight)

		assert.Nil(t, hooks.ModifyHeaderFunc)
		assert.Nil(t, hooks.VerifyHeaderFunc)
		assert.Nil(t, hooks.ProcessHeaderFunc)
		assert.Nil(t, hooks.ShouldWriteTransactionFunc)
		assert.Nil(t, hooks.VerifyBlockFunc)
		assert.Nil(t, hooks.PreCommitStateFunc)

		// case 1: the block number is not the one before fromHeight
		assert.NoError(
			t,
			hooks.PostInsertBlockFunc(block),
		)

		// case 2: the block number is the one before fromHeight
		block.Header.Number = fromHeight - 1

		assert.Equal(
			t,
			hooks.PostInsertBlockFunc(block),
			err,
		)
	})
}

func Test_registerTxInclusionGuardHooks(t *testing.T) {
	t.Parallel()

	epochSize := uint64(10)
	hooks := &hook.Hooks{}

	registerTxInclusionGuardHooks(hooks, epochSize)

	assert.Nil(t, hooks.ModifyHeaderFunc)
	assert.Nil(t, hooks.VerifyHeaderFunc)
	assert.Nil(t, hooks.ProcessHeaderFunc)
	assert.Nil(t, hooks.PreCommitStateFunc)
	assert.Nil(t, hooks.PostInsertBlockFunc)

	var (
		cases = map[uint64]bool{
			0:               true,
			epochSize - 1:   true,
			epochSize:       false,
			epochSize + 1:   true,
			epochSize*2 - 1: true,
			epochSize * 2:   false,
			epochSize*2 + 1: true,
		}

		blockWithoutTransactions = &types.Block{
			Header:       &types.Header{},
			Transactions: []*types.Transaction{},
		}

		blockWithTransactions = &types.Block{
			Header: &types.Header{},
			Transactions: []*types.Transaction{
				{
					Nonce: 0,
				},
			},
		}
	)

	for h, ok := range cases {
		assert.Equal(
			t,
			ok,
			hooks.ShouldWriteTransactions(h),
		)

		blockWithTransactions.Header.Number = h
		blockWithoutTransactions.Header.Number = h

		if ok {
			assert.NoError(t, hooks.VerifyBlock(blockWithoutTransactions))
			assert.NoError(t, hooks.VerifyBlock(blockWithTransactions))
		} else {
			assert.NoError(t, hooks.VerifyBlock(blockWithoutTransactions))
			assert.ErrorIs(t, ErrTxInLastEpochOfBlock, hooks.VerifyBlock(blockWithTransactions))
		}
	}
}

func newTestTransition(
	t *testing.T,
) *state.Transition {
	t.Helper()

	st := itrie.NewState(itrie.NewMemoryStorage())

	ex := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
		BurnContract: map[uint64]types.Address{
			0: types.ZeroAddress,
		},
	}, st, hclog.NewNullLogger())

	rootHash, err := ex.WriteGenesis(nil, types.Hash{})
	assert.NoError(t, err)

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

func Test_registerStakingContractDeploymentHooks(t *testing.T) {
	t.Parallel()

	hooks := &hook.Hooks{}
	fork := &IBFTFork{
		Deployment: &common.JSONNumber{
			Value: 10,
		},
	}

	registerStakingContractDeploymentHooks(hooks, fork)

	assert.Nil(t, hooks.ShouldWriteTransactionFunc)
	assert.Nil(t, hooks.ModifyHeaderFunc)
	assert.Nil(t, hooks.VerifyHeaderFunc)
	assert.Nil(t, hooks.ProcessHeaderFunc)
	assert.Nil(t, hooks.PostInsertBlockFunc)

	txn := newTestTransition(t)

	// deployment should not happen
	assert.NoError(
		t,
		hooks.PreCommitState(&types.Header{Number: 5}, txn),
	)

	assert.False(
		t,
		txn.AccountExists(staking.AddrStakingContract),
	)

	// should deploy contract
	assert.NoError(
		t,
		hooks.PreCommitState(&types.Header{Number: 10}, txn),
	)

	assert.True(
		t,
		txn.AccountExists(staking.AddrStakingContract),
	)

	// should update only bytecode (if contract is deployed again, it returns error)
	assert.NoError(
		t,
		hooks.PreCommitState(&types.Header{Number: 10}, txn),
	)

	assert.True(
		t,
		txn.AccountExists(staking.AddrStakingContract),
	)
}

func Test_getPreDeployParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fork   *IBFTFork
		params stakingHelper.PredeployParams
	}{
		{
			name: "should use the given heights",
			fork: &IBFTFork{
				MinValidatorCount: &common.JSONNumber{Value: 10},
				MaxValidatorCount: &common.JSONNumber{Value: 20},
			},
			params: stakingHelper.PredeployParams{
				MinValidatorCount: 10,
				MaxValidatorCount: 20,
			},
		},
		{
			name: "should use the default values",
			fork: &IBFTFork{},
			params: stakingHelper.PredeployParams{
				MinValidatorCount: stakingHelper.MinValidatorCount,
				MaxValidatorCount: stakingHelper.MaxValidatorCount,
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.params,
				getPreDeployParams(test.fork),
			)
		})
	}
}
