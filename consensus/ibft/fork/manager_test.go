package fork

import (
	"errors"
	"path"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/0xPolygon/polygon-edge/validators/store/snapshot"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

type mockValidatorStore struct {
	store.ValidatorStore

	CloseFunc         func() error
	GetValidatorsFunc func(uint64, uint64, uint64) (validators.Validators, error)
}

func (m *mockValidatorStore) Close() error {
	return m.CloseFunc()
}

func (m *mockValidatorStore) GetValidators(height, epoch, from uint64) (validators.Validators, error) {
	return m.GetValidatorsFunc(height, epoch, from)
}

type mockHooksRegister struct {
	RegisterHooksFunc func(hooks *hook.Hooks, height uint64)
}

func (m *mockHooksRegister) RegisterHooks(hooks *hook.Hooks, height uint64) {
	m.RegisterHooksFunc(hooks, height)
}

type mockSecretManager struct {
	secrets.SecretsManager

	HasSecretFunc func(name string) bool
	GetSecretFunc func(name string) ([]byte, error)
}

func (m *mockSecretManager) HasSecret(name string) bool {
	return m.HasSecretFunc(name)
}

func (m *mockSecretManager) GetSecret(name string) ([]byte, error) {
	return m.GetSecretFunc(name)
}

func TestNewForkManager(t *testing.T) {
	t.Parallel()

	_, ecdsaKeyBytes, err := crypto.GenerateAndEncodeECDSAPrivateKey()
	assert.NoError(t, err)

	_, blsKeyBytes, err := crypto.GenerateAndEncodeBLSSecretKey()
	assert.NoError(t, err)

	logger := hclog.NewNullLogger()

	t.Run("should return error if ibftConfig is empty", func(t *testing.T) {
		t.Parallel()

		_, err := NewForkManager(
			logger,
			nil,
			nil,
			nil,
			"",
			0,
			map[string]interface{}{},
		)

		assert.ErrorIs(t, ErrUndefinedIBFTConfig, err)
	})

	t.Run("should return error if key manager initialization fails", func(t *testing.T) {
		t.Parallel()

		var (
			epochSize uint64 = 10

			secretManager = &mockSecretManager{
				HasSecretFunc: func(name string) bool {
					return true
				},
				GetSecretFunc: func(name string) ([]byte, error) {
					return nil, errTest
				},
			}
		)

		_, err := NewForkManager(
			logger,
			nil,
			nil,
			secretManager,
			"",
			epochSize,
			map[string]interface{}{
				"type":           "PoS",
				"validator_type": "bls",
			},
		)

		assert.ErrorIs(t, errTest, err)
	})

	t.Run("should return error if validator store initialization fails", func(t *testing.T) {
		t.Parallel()

		var (
			latestNumber uint64 = 50
			epochSize    uint64 = 10

			blockchain = &store.MockBlockchain{
				HeaderFn: func() *types.Header {
					return &types.Header{Number: latestNumber}
				},
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return nil, false
				},
			}

			secretManager = &mockSecretManager{
				HasSecretFunc: func(name string) bool {
					assert.Equal(t, secrets.ValidatorKey, name)

					return true
				},
				GetSecretFunc: func(name string) ([]byte, error) {
					assert.Equal(t, secrets.ValidatorKey, name)

					return ecdsaKeyBytes, nil
				},
			}
		)

		dirPath := createTestTempDirectory(t)

		fm, err := NewForkManager(
			logger,
			blockchain,
			nil,
			secretManager,
			dirPath,
			epochSize,
			map[string]interface{}{
				"type":           "PoA",
				"validator_type": "ecdsa",
			},
		)

		assert.NoError(t, err)
		testHelper.AssertErrorMessageContains(
			t,
			errors.New("header at 50 not found"),
			fm.Initialize(),
		)
	})

	t.Run("PoA and ECDSA", func(t *testing.T) {
		t.Parallel()

		var (
			latestNumber uint64 = 50
			epochSize    uint64 = 10

			metadata = &snapshot.SnapshotMetadata{
				LastBlock: latestNumber,
			}

			snapshots = []*snapshot.Snapshot{
				{
					Number: latestNumber,
					Hash:   types.BytesToHash(crypto.Keccak256([]byte{0x10})).String(),
					Set: validators.NewECDSAValidatorSet(
						validators.NewECDSAValidator(types.StringToAddress("1")),
					),
					Votes: []*store.Vote{
						{
							Candidate: validators.NewECDSAValidator(types.StringToAddress("2")),
							Validator: types.StringToAddress("1"),
							Authorize: true,
						},
					},
				},
			}

			blockchain = &store.MockBlockchain{
				HeaderFn: func() *types.Header {
					return &types.Header{Number: latestNumber}
				},
			}

			secretManager = &mockSecretManager{
				HasSecretFunc: func(name string) bool {
					assert.Equal(t, secrets.ValidatorKey, name)

					return true
				},
				GetSecretFunc: func(name string) ([]byte, error) {
					assert.Equal(t, secrets.ValidatorKey, name)

					return ecdsaKeyBytes, nil
				},
			}
		)

		dirPath := createTestTempDirectory(t)

		assert.NoError(
			t,
			writeDataStore(path.Join(dirPath, snapshotMetadataFilename), metadata),
		)

		assert.NoError(
			t,
			writeDataStore(path.Join(dirPath, snapshotSnapshotsFilename), snapshots),
		)

		fm, err := NewForkManager(
			logger,
			blockchain,
			nil,
			secretManager,
			dirPath,
			epochSize,
			map[string]interface{}{
				"type":           "PoA",
				"validator_type": "ecdsa",
			},
		)

		assert.NoError(t, err)
		assert.NoError(t, fm.Initialize())

		assert.NotNil(t, fm.keyManagers[validators.ECDSAValidatorType])
		assert.NotNil(t, fm.validatorStores[store.Snapshot])
		assert.NotNil(t, fm.hooksRegisters[PoA])
	})

	t.Run("PoS and BLS", func(t *testing.T) {
		t.Parallel()

		var (
			epochSize uint64 = 10

			secretManager = &mockSecretManager{
				HasSecretFunc: func(name string) bool {
					assert.True(t, name == secrets.ValidatorKey || name == secrets.ValidatorBLSKey)

					return true
				},
				GetSecretFunc: func(name string) ([]byte, error) {
					assert.True(t, name == secrets.ValidatorKey || name == secrets.ValidatorBLSKey)

					if name == secrets.ValidatorKey {
						return ecdsaKeyBytes, nil
					} else {
						return blsKeyBytes, nil
					}
				},
			}
		)

		fm, err := NewForkManager(
			logger,
			nil,
			nil,
			secretManager,
			"",
			epochSize,
			map[string]interface{}{
				"type":           "PoS",
				"validator_type": "bls",
			},
		)

		assert.NoError(t, err)
		assert.NoError(t, fm.Initialize())

		assert.NotNil(t, fm.keyManagers[validators.BLSValidatorType])
		assert.NotNil(t, fm.validatorStores[store.Contract])
		assert.NotNil(t, fm.hooksRegisters[PoS])
	})
}

func TestForkManagerClose(t *testing.T) {
	t.Parallel()

	t.Run("should call all Close methods of ValidatorStore", func(t *testing.T) {
		t.Parallel()

		numCalls := 0

		fm := &ForkManager{
			validatorStores: map[store.SourceType]ValidatorStore{
				store.Contract: &mockValidatorStore{
					CloseFunc: func() error {
						numCalls++

						return nil
					},
				},
				store.Snapshot: &mockValidatorStore{
					CloseFunc: func() error {
						numCalls++

						return nil
					},
				},
			},
		}

		assert.NoError(t, fm.Close())
		assert.Equal(t, 2, numCalls)
	})

	t.Run("should return error if one of Close method returns error", func(t *testing.T) {
		t.Parallel()

		numCalls := 0

		fm := &ForkManager{
			validatorStores: map[store.SourceType]ValidatorStore{
				// should call the either
				store.Contract: &mockValidatorStore{
					CloseFunc: func() error {
						numCalls++

						return errTest
					},
				},
				store.Snapshot: &mockValidatorStore{
					CloseFunc: func() error {
						numCalls++

						return errTest
					},
				},
			},
		}

		assert.Equal(t, errTest, fm.Close())
		assert.Equal(t, 1, numCalls)
	})
}

type MockKeyManager struct {
	signer.KeyManager
	ValType validators.ValidatorType
}

func TestForkManagerGetSigner(t *testing.T) {
	t.Parallel()

	var (
		ecdsaKeyManager = &MockKeyManager{
			ValType: validators.ECDSAValidatorType,
		}
		blsKeyManager = &MockKeyManager{
			ValType: validators.BLSValidatorType,
		}
	)

	tests := []struct {
		name           string
		forks          IBFTForks
		keyManagers    map[validators.ValidatorType]signer.KeyManager
		height         uint64
		expectedSigner signer.Signer
		expectedErr    error
	}{
		{
			name: "should return ErrForkNotFound if fork not found",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 10},
				},
				{
					ValidatorType: validators.BLSValidatorType,
					From:          common.JSONNumber{Value: 11},
					To:            &common.JSONNumber{Value: 20},
				},
			},
			keyManagers:    map[validators.ValidatorType]signer.KeyManager{},
			height:         22,
			expectedSigner: nil,
			expectedErr:    ErrForkNotFound,
		},
		{
			name: "should return ErrKeyManagerNotFound if fork managers is nil",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
				},
			},
			keyManagers:    map[validators.ValidatorType]signer.KeyManager{},
			height:         10,
			expectedSigner: nil,
			expectedErr:    ErrKeyManagerNotFound,
		},
		{
			name: "should return err if fork not found for parent key manager",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 2},
				},
			},
			keyManagers: map[validators.ValidatorType]signer.KeyManager{
				validators.ECDSAValidatorType: MockKeyManager{},
			},
			height:         2,
			expectedSigner: nil,
			expectedErr:    ErrForkNotFound,
		},
		{
			name: "should return the signer with single key manager if the height is 1",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 10},
				},
				{
					ValidatorType: validators.BLSValidatorType,
					From:          common.JSONNumber{Value: 11},
				},
			},
			keyManagers: map[validators.ValidatorType]signer.KeyManager{
				validators.ECDSAValidatorType: ecdsaKeyManager,
				validators.BLSValidatorType:   blsKeyManager,
			},
			height:         1,
			expectedSigner: signer.NewSigner(ecdsaKeyManager, nil),
		},
		{
			name: "should return the signer with different key manager and parent key manager",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 10},
				},
				{
					ValidatorType: validators.BLSValidatorType,
					From:          common.JSONNumber{Value: 11},
				},
			},
			keyManagers: map[validators.ValidatorType]signer.KeyManager{
				validators.ECDSAValidatorType: ecdsaKeyManager,
				validators.BLSValidatorType:   blsKeyManager,
			},
			height:         11,
			expectedSigner: signer.NewSigner(blsKeyManager, ecdsaKeyManager),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fm := &ForkManager{
				forks:       test.forks,
				keyManagers: test.keyManagers,
			}

			signer, err := fm.GetSigner(test.height)

			assert.Equal(
				t,
				test.expectedSigner,
				signer,
			)

			assert.Equal(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func TestForkManagerGetValidatorStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		forks           IBFTForks
		validatorStores map[store.SourceType]ValidatorStore
		height          uint64
		expectedStore   ValidatorStore
		expectedErr     error
	}{
		{
			name: "should return ErrForkNotFound if fork not found",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 10},
				},
				{
					ValidatorType: validators.BLSValidatorType,
					From:          common.JSONNumber{Value: 11},
					To:            &common.JSONNumber{Value: 20},
				},
			},
			validatorStores: map[store.SourceType]ValidatorStore{},
			height:          25,
			expectedStore:   nil,
			expectedErr:     ErrForkNotFound,
		},
		{
			name: "should return ErrValidatorStoreNotFound if validator store not found",
			forks: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
				},
			},
			validatorStores: map[store.SourceType]ValidatorStore{},
			height:          25,
			expectedStore:   nil,
			expectedErr:     ErrValidatorStoreNotFound,
		},
		{
			name: "should return Snapshot store for PoA",
			forks: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
				},
			},
			validatorStores: map[store.SourceType]ValidatorStore{
				store.Snapshot: &mockValidatorStore{},
			},
			height:        25,
			expectedStore: &mockValidatorStore{},
			expectedErr:   nil,
		},
		{
			name: "should return Contract store for PoS",
			forks: IBFTForks{
				{
					Type:          PoS,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
				},
			},
			validatorStores: map[store.SourceType]ValidatorStore{
				store.Contract: &mockValidatorStore{},
			},
			height:        25,
			expectedStore: &mockValidatorStore{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fm := &ForkManager{
				forks:           test.forks,
				validatorStores: test.validatorStores,
			}

			store, err := fm.GetValidatorStore(test.height)

			assert.Equal(
				t,
				test.expectedStore,
				store,
			)

			assert.Equal(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func TestForkManagerGetValidators(t *testing.T) {
	t.Parallel()

	var epochSize uint64 = 10

	tests := []struct {
		name               string
		forks              IBFTForks
		validatorStores    map[store.SourceType]ValidatorStore
		height             uint64
		expectedValidators validators.Validators
		expectedErr        error
	}{
		{
			name: "should return ErrForkNotFound if fork not found",
			forks: IBFTForks{
				{
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 10},
				},
				{
					ValidatorType: validators.BLSValidatorType,
					From:          common.JSONNumber{Value: 11},
					To:            &common.JSONNumber{Value: 20},
				},
			},
			validatorStores:    map[store.SourceType]ValidatorStore{},
			height:             25,
			expectedValidators: nil,
			expectedErr:        ErrForkNotFound,
		},
		{
			name: "should return ErrValidatorStoreNotFound if validator store not found",
			forks: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
				},
			},
			validatorStores:    map[store.SourceType]ValidatorStore{},
			height:             25,
			expectedValidators: nil,
			expectedErr:        ErrValidatorStoreNotFound,
		},
		{
			name: "should return Validators",
			forks: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 10},
				},
			},
			validatorStores: map[store.SourceType]ValidatorStore{
				store.Snapshot: &mockValidatorStore{
					GetValidatorsFunc: func(u1, u2, u3 uint64) (validators.Validators, error) {
						assert.Equal(t, uint64(25), u1) // height
						assert.Equal(t, epochSize, u2)  // epochSize
						assert.Equal(t, uint64(10), u3) // from

						return validators.NewECDSAValidatorSet(
							validators.NewECDSAValidator(types.StringToAddress("1")),
							validators.NewECDSAValidator(types.StringToAddress("2")),
						), nil
					},
				},
			},
			height: 25,
			expectedValidators: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(types.StringToAddress("1")),
				validators.NewECDSAValidator(types.StringToAddress("2")),
			),
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fm := &ForkManager{
				forks:           test.forks,
				validatorStores: test.validatorStores,
				epochSize:       epochSize,
			}

			validators, err := fm.GetValidators(test.height)

			assert.Equal(
				t,
				test.expectedValidators,
				validators,
			)

			assert.Equal(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func TestForkManagerGetHooks(t *testing.T) {
	t.Parallel()

	var (
		height uint64 = 25

		err1 = errors.New("error 1")
		err2 = errors.New("error 2")
	)

	fm := &ForkManager{
		hooksRegisters: map[IBFTType]HooksRegister{
			PoA: &mockHooksRegister{
				RegisterHooksFunc: func(hooks *hook.Hooks, h uint64) {
					assert.Equal(t, height, h)

					hooks.ModifyHeaderFunc = func(h *types.Header, a types.Address) error {
						return err1
					}
				},
			},
			PoS: &mockHooksRegister{
				RegisterHooksFunc: func(hooks *hook.Hooks, h uint64) {
					assert.Equal(t, height, h)

					hooks.VerifyBlockFunc = func(b *types.Block) error {
						return err2
					}
				},
			},
		},
	}

	hooks := fm.GetHooks(height)

	assert.Equal(t, err1, hooks.ModifyHeader(&types.Header{}, types.StringToAddress("1")))
	assert.Equal(t, err2, hooks.VerifyBlock(&types.Block{}), nil)
}

func TestForkManager_initializeKeyManagers(t *testing.T) {
	t.Parallel()

	key, keyBytes, err := crypto.GenerateAndEncodeECDSAPrivateKey()
	assert.NoError(t, err)

	tests := []struct {
		name                string
		forks               IBFTForks
		secretManager       *mockSecretManager
		expectedErr         error
		expectedKeyManagers map[validators.ValidatorType]signer.KeyManager
	}{
		{
			name: "should return error if NewKeyManagerFromType fails",
			forks: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ValidatorType("fake"),
					From:          common.JSONNumber{Value: 0},
				},
			},
			secretManager:       nil,
			expectedErr:         errors.New("unsupported validator type: fake"),
			expectedKeyManagers: map[validators.ValidatorType]signer.KeyManager{},
		},
		{
			name: "should initializes key manager",
			forks: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 49},
				},
				{
					Type:          PoS,
					ValidatorType: validators.ECDSAValidatorType,
					From:          common.JSONNumber{Value: 50},
				},
			},
			secretManager: &mockSecretManager{
				HasSecretFunc: func(name string) bool {
					assert.Equal(t, secrets.ValidatorKey, name)

					return true
				},
				GetSecretFunc: func(name string) ([]byte, error) {
					assert.Equal(t, secrets.ValidatorKey, name)

					return keyBytes, nil
				},
			},
			expectedErr: nil,
			expectedKeyManagers: map[validators.ValidatorType]signer.KeyManager{
				validators.ECDSAValidatorType: signer.NewECDSAKeyManagerFromKey(key),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fm := &ForkManager{
				forks:          test.forks,
				secretsManager: test.secretManager,
				keyManagers:    map[validators.ValidatorType]signer.KeyManager{},
			}

			testHelper.AssertErrorMessageContains(
				t,
				fm.initializeKeyManagers(),
				test.expectedErr,
			)

			assert.Equal(
				t,
				test.expectedKeyManagers,
				fm.keyManagers,
			)
		})
	}
}

func TestForkManager_initializeValidatorStores(t *testing.T) {
	t.Parallel()

	var (
		logger     = hclog.NewNullLogger()
		blockchain = &store.MockBlockchain{}
		executor   = &MockExecutor{}

		forks = IBFTForks{
			{
				Type:          PoS,
				ValidatorType: validators.ECDSAValidatorType,
				From:          common.JSONNumber{Value: 0},
				To:            &common.JSONNumber{Value: 49},
			},
			{
				Type:          PoS,
				ValidatorType: validators.ECDSAValidatorType,
				From:          common.JSONNumber{Value: 50},
			},
		}
	)

	fm := &ForkManager{
		forks:           forks,
		validatorStores: map[store.SourceType]ValidatorStore{},
		logger:          logger,
		blockchain:      blockchain,
		executor:        executor,
	}

	assert.NoError(t, fm.initializeValidatorStores())

	assert.NotNil(
		t,
		fm.validatorStores[store.Contract],
	)

	assert.Nil(
		t,
		fm.validatorStores[store.Snapshot],
	)
}

func TestForkManager_initializeHooksRegisters(t *testing.T) {
	t.Parallel()

	var (
		forks = IBFTForks{
			{
				Type:          PoA,
				ValidatorType: validators.ECDSAValidatorType,
				From:          common.JSONNumber{Value: 0},
				To:            &common.JSONNumber{Value: 49},
			},
			{
				Type:          PoS,
				ValidatorType: validators.ECDSAValidatorType,
				From:          common.JSONNumber{Value: 50},
				To:            &common.JSONNumber{Value: 100},
			},
			{
				Type:          PoS,
				ValidatorType: validators.ECDSAValidatorType,
				From:          common.JSONNumber{Value: 101},
			},
		}
	)

	fm := &ForkManager{
		forks:          forks,
		hooksRegisters: map[IBFTType]HooksRegister{},
	}

	fm.initializeHooksRegisters()

	assert.NotNil(
		t,
		fm.hooksRegisters[PoA],
	)

	assert.NotNil(
		t,
		fm.hooksRegisters[PoS],
	)
}
