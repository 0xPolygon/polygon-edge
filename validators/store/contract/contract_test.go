package contract

import (
	"errors"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/stretchr/testify/assert"
)

var (
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

func newTestCache(t *testing.T, size int) *lru.Cache {
	t.Helper()

	cache, err := lru.New(size)
	assert.NoError(t, err)

	return cache
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

func newTestTransition(
	t *testing.T,
) *state.Transition {
	t.Helper()

	st := itrie.NewState(itrie.NewMemoryStorage())

	ex := state.NewExecutor(&chain.Params{
		Forks: chain.AllForksEnabled,
	}, st, hclog.NewNullLogger())

	rootHash := ex.WriteGenesis(nil)

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

	return transition
}

func newTestTransitionWithPredeployedStakingContract(
	t *testing.T,
	validators validators.Validators,
) *state.Transition {
	t.Helper()

	transition := newTestTransition(t)

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

func newTestContractValidatorStore(
	t *testing.T,
	blockchain store.HeaderGetter,
	executor Executor,
	cacheSize int,
) *ContractValidatorStore {
	t.Helper()

	var cache *lru.Cache
	if cacheSize > 0 {
		cache = newTestCache(t, cacheSize)
	}

	return &ContractValidatorStore{
		logger:            hclog.NewNullLogger(),
		blockchain:        blockchain,
		executor:          executor,
		validatorSetCache: cache,
	}
}

func TestNewContractValidatorStore(t *testing.T) {
	t.Parallel()

	var (
		logger     = hclog.NewNullLogger()
		blockchain = store.HeaderGetter(
			&store.MockBlockchain{},
		)
		executor = Executor(
			&mockExecutor{},
		)
	)

	tests := []struct {
		name        string
		cacheSize   int
		expectedRes *ContractValidatorStore
		expectedErr error
	}{
		{
			name:      "should return store",
			cacheSize: 1,
			expectedRes: &ContractValidatorStore{
				logger:            logger,
				blockchain:        blockchain,
				executor:          executor,
				validatorSetCache: newTestCache(t, 1),
			},
			expectedErr: nil,
		},
		{
			name:      "should return store without cache if cache size is zero",
			cacheSize: 0,
			expectedRes: &ContractValidatorStore{
				logger:     logger,
				blockchain: blockchain,
				executor:   executor,
			},
			expectedErr: nil,
		},
		{
			name:      "should return store without cache if cache size is negative",
			cacheSize: -1,
			expectedRes: &ContractValidatorStore{
				logger:     logger,
				blockchain: blockchain,
				executor:   executor,
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := NewContractValidatorStore(
				logger,
				blockchain,
				executor,
				test.cacheSize,
			)

			assert.Equal(t, test.expectedRes, res)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
		})
	}
}

func TestContractValidatorStoreSourceType(t *testing.T) {
	t.Parallel()

	s := &ContractValidatorStore{}

	assert.Equal(t, store.Contract, s.SourceType())
}

func TestContractValidatorStoreGetValidators(t *testing.T) {
	t.Parallel()

	var (
		stateRoot = types.StringToHash("1")
		header    = &types.Header{
			StateRoot: stateRoot,
		}

		ecdsaValidators = validators.NewECDSAValidatorSet(
			validators.NewECDSAValidator(addr1),
			validators.NewECDSAValidator(addr2),
		)

		blsValidators = validators.NewBLSValidatorSet(
			validators.NewBLSValidator(addr1, testBLSPubKey1),
			validators.NewBLSValidator(addr2, testBLSPubKey2),
		)

		transitionForECDSAValidators = newTestTransitionWithPredeployedStakingContract(
			t,
			ecdsaValidators,
		)

		transitionForBLSValidators = newTestTransitionWithPredeployedStakingContract(
			t,
			blsValidators,
		)
	)

	tests := []struct {
		name          string
		blockchain    store.HeaderGetter
		executor      Executor
		cacheSize     int
		initialCaches map[uint64]interface{}

		// input
		validatorType validators.ValidatorType
		height        uint64

		// output
		expectedRes validators.Validators
		expectedErr error
		// caches after calling GetValidators
		finalCaches map[uint64]interface{}
	}{
		{
			name:       "should return error when loadCachedValidatorSet failed",
			blockchain: nil,
			executor:   nil,
			cacheSize:  1,
			initialCaches: map[uint64]interface{}{
				0: string("fake"),
			},
			height:      0,
			expectedRes: nil,
			expectedErr: ErrInvalidValidatorsTypeAssertion,
			finalCaches: map[uint64]interface{}{
				0: string("fake"),
			},
		},
		{
			name:       "should return validators if cache exists",
			blockchain: nil,
			executor:   nil,
			cacheSize:  1,
			initialCaches: map[uint64]interface{}{
				0: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(addr1),
				),
			},
			height: 0,
			expectedRes: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(addr1),
			),
			expectedErr: nil,
			finalCaches: map[uint64]interface{}{
				0: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(addr1),
				),
			},
		},
		{
			name: "should return error if header not found",
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, uint64(1), height)

					return nil, false
				},
			},
			executor:      nil,
			cacheSize:     1,
			initialCaches: map[uint64]interface{}{},
			height:        1,
			expectedRes:   nil,
			expectedErr:   errors.New("header not found at 1"),
			finalCaches:   map[uint64]interface{}{},
		},
		{
			name: "should return error if FetchValidators failed",
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, uint64(1), height)

					return header, true
				},
			},
			executor: &mockExecutor{
				BeginTxnFn: func(hash types.Hash, head *types.Header, addr types.Address) (*state.Transition, error) {
					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, header, head)
					assert.Equal(t, types.ZeroAddress, addr)

					return transitionForECDSAValidators, nil
				},
			},
			cacheSize:     1,
			initialCaches: map[uint64]interface{}{},
			validatorType: validators.ValidatorType("fake"),
			height:        1,
			expectedRes:   nil,
			expectedErr:   errors.New("unsupported validator type: fake"),
			finalCaches:   map[uint64]interface{}{},
		},
		{
			name: "should return fetched ECDSA validators",
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, uint64(1), height)

					return header, true
				},
			},
			executor: &mockExecutor{
				BeginTxnFn: func(hash types.Hash, head *types.Header, addr types.Address) (*state.Transition, error) {
					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, header, head)
					assert.Equal(t, types.ZeroAddress, addr)

					return transitionForECDSAValidators, nil
				},
			},
			cacheSize:     1,
			initialCaches: map[uint64]interface{}{},
			validatorType: validators.ECDSAValidatorType,
			height:        1,
			expectedRes:   ecdsaValidators,
			expectedErr:   nil,
			finalCaches: map[uint64]interface{}{
				1: ecdsaValidators,
			},
		},
		{
			name: "should return fetched BLS validators",
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, uint64(1), height)

					return header, true
				},
			},
			executor: &mockExecutor{
				BeginTxnFn: func(hash types.Hash, head *types.Header, addr types.Address) (*state.Transition, error) {
					assert.Equal(t, stateRoot, hash)
					assert.Equal(t, header, head)
					assert.Equal(t, types.ZeroAddress, addr)

					return transitionForBLSValidators, nil
				},
			},
			cacheSize:     1,
			initialCaches: map[uint64]interface{}{},
			validatorType: validators.BLSValidatorType,
			height:        1,
			expectedRes:   blsValidators,
			expectedErr:   nil,
			finalCaches: map[uint64]interface{}{
				1: blsValidators,
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestContractValidatorStore(
				t,
				test.blockchain,
				test.executor,
				test.cacheSize,
			)

			for height, data := range test.initialCaches {
				store.validatorSetCache.Add(height, data)
			}

			res, err := store.GetValidatorsByHeight(test.validatorType, test.height)

			assert.Equal(t, test.expectedRes, res)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)

			// check cache
			assert.Equal(t, len(test.finalCaches), store.validatorSetCache.Len())

			for height, expected := range test.finalCaches {
				cache, ok := store.validatorSetCache.Get(height)

				assert.True(t, ok)
				assert.Equal(t, expected, cache)
			}
		})
	}
}

func TestContractValidatorStore_CacheChange(t *testing.T) {
	var (
		cacheSize = 2

		store = newTestContractValidatorStore(
			t,
			nil,
			nil,
			cacheSize,
		)

		ecdsaValidators1 = validators.NewECDSAValidatorSet(
			validators.NewECDSAValidator(addr1),
		)

		ecdsaValidators2 = validators.NewECDSAValidatorSet(
			validators.NewECDSAValidator(addr1),
			validators.NewECDSAValidator(addr2),
		)

		blsValidators = validators.NewBLSValidatorSet(
			validators.NewBLSValidator(addr1, testBLSPubKey1),
			validators.NewBLSValidator(addr2, testBLSPubKey2),
		)
	)

	type testCase struct {
		height   uint64
		expected validators.Validators
	}

	testCache := func(t *testing.T, testCases ...testCase) {
		t.Helper()

		assert.Equal(t, len(testCases), store.validatorSetCache.Len())

		for _, testCase := range testCases {
			cache, ok := store.validatorSetCache.Get(testCase.height)

			assert.Truef(t, ok, "validators at %d must exist, but not found", testCase.height)
			assert.Equal(t, testCase.expected, cache)
		}
	}

	// initial cache is empty
	testCache(t)

	// overflow doesn't occur
	assert.False(
		t,
		store.saveToValidatorSetCache(0, ecdsaValidators1),
	)

	testCache(
		t,
		testCase{height: 0, expected: ecdsaValidators1},
	)

	assert.False(
		t,
		store.saveToValidatorSetCache(1, ecdsaValidators2),
	)

	testCache(
		t,
		testCase{height: 0, expected: ecdsaValidators1},
		testCase{height: 1, expected: ecdsaValidators2},
	)

	// make sure ecdsaValidators2 is loaded at the end for LRU cache
	store.validatorSetCache.Get(1)

	// overflow occurs and one validator set is removed
	assert.True(
		t,
		store.saveToValidatorSetCache(2, blsValidators),
	)

	testCache(
		t,
		testCase{height: 1, expected: ecdsaValidators2},
		testCase{height: 2, expected: blsValidators},
	)
}

func TestContractValidatorStore_NoCache(t *testing.T) {
	t.Parallel()

	var (
		store = newTestContractValidatorStore(
			t,
			nil,
			nil,
			0,
		)

		ecdsaValidators1 = validators.NewECDSAValidatorSet(
			validators.NewECDSAValidator(addr1),
		)
	)

	// nothing happens because cache is nil
	assert.False(
		t,
		store.saveToValidatorSetCache(0, ecdsaValidators1),
	)

	assert.Nil(t, store.validatorSetCache)
}
