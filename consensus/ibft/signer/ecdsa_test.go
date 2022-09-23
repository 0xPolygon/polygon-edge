package signer

import (
	"crypto/ecdsa"
	"errors"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

func newTestECDSAKeyManager(t *testing.T) (KeyManager, *ecdsa.PrivateKey) {
	t.Helper()

	testKey, _ := newTestECDSAKey(t)

	return NewECDSAKeyManagerFromKey(testKey), testKey
}

func TestNewECDSAKeyManager(t *testing.T) {
	t.Parallel()

	testKey, testKeyEncoded := newTestECDSAKey(t)

	testSecretName := func(name string) {
		t.Helper()

		// make sure that the correct key is given
		assert.Equal(t, secrets.ValidatorKey, name)
	}

	//lint:ignore dupl
	tests := []struct {
		name              string
		mockSecretManager *MockSecretManager
		expectedResult    KeyManager
		expectedErr       error
	}{
		{
			name: "should initialize ECDSAKeyManager from the loaded ECDSA key",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					return testKeyEncoded, nil
				},
			},
			expectedResult: &ECDSAKeyManager{
				key:     testKey,
				address: crypto.PubKeyToAddress(&testKey.PublicKey),
			},
			expectedErr: nil,
		},
		{
			name: "should return error if getOrCreateECDSAKey returns error",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					return nil, errTest
				},
			},
			expectedResult: nil,
			expectedErr:    errTest,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := NewECDSAKeyManager(test.mockSecretManager)

			assert.Equal(t, test.expectedResult, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}
func TestNewECDSAKeyManagerFromKey(t *testing.T) {
	t.Parallel()

	testKey, _ := newTestECDSAKey(t)

	assert.Equal(
		t,
		&ECDSAKeyManager{
			key:     testKey,
			address: crypto.PubKeyToAddress(&testKey.PublicKey),
		},
		NewECDSAKeyManagerFromKey(testKey),
	)
}

func TestECDSAKeyManagerType(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)

	assert.Equal(
		t,
		validators.ECDSAValidatorType,
		ecdsaKeyManager.Type(),
	)
}

func TestECDSAKeyManagerAddress(t *testing.T) {
	t.Parallel()

	ecdsaKey, _ := newTestECDSAKey(t)
	ecdsaKeyManager := NewECDSAKeyManagerFromKey(ecdsaKey)

	assert.Equal(
		t,
		crypto.PubKeyToAddress(&ecdsaKey.PublicKey),
		ecdsaKeyManager.Address(),
	)
}

func TestECDSAKeyManagerNewEmptyValidators(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)

	assert.Equal(
		t,
		validators.NewECDSAValidatorSet(),
		ecdsaKeyManager.NewEmptyValidators(),
	)
}

func TestECDSAKeyManagerNewEmptyCommittedSeals(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)

	assert.Equal(
		t,
		&SerializedSeal{},
		ecdsaKeyManager.NewEmptyCommittedSeals(),
	)
}

func TestECDSAKeyManagerSignProposerSeal(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)
	msg := crypto.Keccak256(
		hex.MustDecodeHex(testHeaderHashHex),
	)

	proposerSeal, err := ecdsaKeyManager.SignProposerSeal(msg)
	assert.NoError(t, err)

	recoveredAddress, err := ecrecover(proposerSeal, msg)
	assert.NoError(t, err)

	assert.Equal(
		t,
		ecdsaKeyManager.Address(),
		recoveredAddress,
	)
}

func TestECDSAKeyManagerSignCommittedSeal(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)
	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	proposerSeal, err := ecdsaKeyManager.SignCommittedSeal(msg)
	assert.NoError(t, err)

	recoveredAddress, err := ecrecover(proposerSeal, msg)
	assert.NoError(t, err)

	assert.Equal(
		t,
		ecdsaKeyManager.Address(),
		recoveredAddress,
	)
}

func TestECDSAKeyManagerVerifyCommittedSeal(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager1, _ := newTestECDSAKeyManager(t)
	ecdsaKeyManager2, _ := newTestECDSAKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctSignature, err := ecdsaKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	wrongSignature, err := ecdsaKeyManager2.SignCommittedSeal(msg)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		validators  validators.Validators
		address     types.Address
		signature   []byte
		message     []byte
		expectedErr error
	}{
		{
			name:        "should return ErrInvalidValidators if validators is wrong type",
			validators:  validators.NewBLSValidatorSet(),
			address:     ecdsaKeyManager1.Address(),
			signature:   []byte{},
			message:     []byte{},
			expectedErr: ErrInvalidValidators,
		},
		{
			name:        "should return ErrInvalidSignature if ecrecover failed",
			validators:  validators.NewECDSAValidatorSet(),
			address:     ecdsaKeyManager1.Address(),
			signature:   []byte{},
			message:     []byte{},
			expectedErr: ErrInvalidSignature,
		},
		{
			name:        "should return ErrSignerMismatch if the signature is signed by different signer",
			validators:  validators.NewECDSAValidatorSet(),
			address:     ecdsaKeyManager1.Address(),
			signature:   wrongSignature,
			message:     msg,
			expectedErr: ErrSignerMismatch,
		},
		{
			name:        "should return ErrNonValidatorCommittedSeal if the signer is not in the validators",
			validators:  validators.NewECDSAValidatorSet(),
			address:     ecdsaKeyManager1.Address(),
			signature:   correctSignature,
			message:     msg,
			expectedErr: ErrNonValidatorCommittedSeal,
		},
		{
			name: "should return nil if it's verified",
			validators: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(
					ecdsaKeyManager1.Address(),
				),
			),
			address:     ecdsaKeyManager1.Address(),
			signature:   correctSignature,
			message:     msg,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(
				t,
				test.expectedErr,
				ecdsaKeyManager1.VerifyCommittedSeal(
					test.validators,
					test.address,
					test.signature,
					test.message,
				),
			)
		})
	}
}

func TestECDSAKeyManagerGenerateCommittedSeals(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager1, _ := newTestECDSAKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctCommittedSeal, err := ecdsaKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	wrongCommittedSeal := []byte("fake")

	tests := []struct {
		name        string
		sealMap     map[types.Address][]byte
		expectedRes Seals
		expectedErr error
	}{
		{
			name: "should return ErrInvalidCommittedSealLength if the size of committed seal doesn't equal to IstanbulExtraSeal",
			sealMap: map[types.Address][]byte{
				ecdsaKeyManager1.Address(): wrongCommittedSeal,
			},
			expectedRes: nil,
			expectedErr: ErrInvalidCommittedSealLength,
		},
		{
			name: "should return SerializedSeal",
			sealMap: map[types.Address][]byte{
				ecdsaKeyManager1.Address(): correctCommittedSeal,
			},
			expectedRes: &SerializedSeal{
				correctCommittedSeal,
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := ecdsaKeyManager1.GenerateCommittedSeals(
				test.sealMap,
				nil,
			)

			assert.Equal(t, test.expectedRes, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

func TestECDSAKeyManagerVerifyCommittedSeals(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager1, _ := newTestECDSAKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctCommittedSeal, err := ecdsaKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		committedSeals Seals
		digest         []byte
		rawSet         validators.Validators
		expectedRes    int
		expectedErr    error
	}{
		{
			name:           "should return ErrInvalidCommittedSealType if the Seals is not *SerializedSeal",
			committedSeals: &AggregatedSeal{},
			digest:         msg,
			rawSet:         nil,
			expectedRes:    0,
			expectedErr:    ErrInvalidCommittedSealType,
		},
		{
			name:           "should return ErrInvalidValidators if the rawSet is not *validators.ECDSAValidators",
			committedSeals: &SerializedSeal{},
			digest:         msg,
			rawSet:         validators.NewBLSValidatorSet(),
			expectedRes:    0,
			expectedErr:    ErrInvalidValidators,
		},
		{
			name: "should return size of CommittedSeals if verification is successful",
			committedSeals: &SerializedSeal{
				correctCommittedSeal,
			},
			digest: msg,
			rawSet: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(
					ecdsaKeyManager1.Address(),
				),
			),
			expectedRes: 1,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := ecdsaKeyManager1.VerifyCommittedSeals(
				test.committedSeals,
				test.digest,
				test.rawSet,
			)

			assert.Equal(t, test.expectedRes, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

func TestECDSAKeyManagerSignIBFTMessageAndEcrecover(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager, _ := newTestECDSAKeyManager(t)
	msg := crypto.Keccak256([]byte("message"))

	proposerSeal, err := ecdsaKeyManager.SignIBFTMessage(msg)
	assert.NoError(t, err)

	recoveredAddress, err := ecdsaKeyManager.Ecrecover(proposerSeal, msg)
	assert.NoError(t, err)

	assert.Equal(
		t,
		ecdsaKeyManager.Address(),
		recoveredAddress,
	)
}

func TestECDSAKeyManager_verifyCommittedSealsImpl(t *testing.T) {
	t.Parallel()

	ecdsaKeyManager1, _ := newTestECDSAKeyManager(t)
	ecdsaKeyManager2, _ := newTestECDSAKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctCommittedSeal, err := ecdsaKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	nonValidatorsCommittedSeal, err := ecdsaKeyManager2.SignCommittedSeal(msg)
	assert.NoError(t, err)

	wrongSignature := []byte("fake")

	tests := []struct {
		name           string
		committedSeals *SerializedSeal
		msg            []byte
		validators     validators.Validators
		expectedRes    int
		expectedErr    error
	}{
		{
			name:           "should return ErrInvalidCommittedSealType if the Seals is not *SerializedSeal",
			committedSeals: &SerializedSeal{},
			msg:            msg,
			validators:     validators.NewECDSAValidatorSet(),
			expectedRes:    0,
			expectedErr:    ErrEmptyCommittedSeals,
		},
		{
			name: "should return error if Ecrecover failed",
			committedSeals: &SerializedSeal{
				wrongSignature,
			},
			msg:         msg,
			validators:  validators.NewECDSAValidatorSet(),
			expectedRes: 0,
			expectedErr: errors.New("invalid compact signature size"),
		},
		{
			name: "should return error ErrRepeatedCommittedSeal if CommittedSeal",
			committedSeals: &SerializedSeal{
				correctCommittedSeal,
				correctCommittedSeal,
			},
			msg: msg,
			validators: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(
					ecdsaKeyManager1.Address(),
				),
			),
			expectedRes: 0,
			expectedErr: ErrRepeatedCommittedSeal,
		},
		{
			name: "should return error ErrNonValidatorCommittedSeal if CommittedSeals has the signature by non-validator",
			committedSeals: &SerializedSeal{
				correctCommittedSeal,
				nonValidatorsCommittedSeal,
			},
			msg: msg,
			validators: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(
					ecdsaKeyManager1.Address(),
				),
			),
			expectedRes: 0,
			expectedErr: ErrNonValidatorCommittedSeal,
		},
		{
			name: "should return the size of CommittedSeals if verification is successful",
			committedSeals: &SerializedSeal{
				correctCommittedSeal,
			},
			msg: msg,
			validators: validators.NewECDSAValidatorSet(
				validators.NewECDSAValidator(
					ecdsaKeyManager1.Address(),
				),
			),
			expectedRes: 1,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := ecdsaKeyManager1.(*ECDSAKeyManager).verifyCommittedSealsImpl(
				test.committedSeals,
				test.msg,
				test.validators,
			)

			assert.Equal(t, test.expectedRes, res)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
		})
	}
}
