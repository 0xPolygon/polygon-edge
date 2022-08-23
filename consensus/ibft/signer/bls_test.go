package signer

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/stretchr/testify/assert"
)

func newTestBLSKeyManager(t *testing.T) (KeyManager, *ecdsa.PrivateKey, *bls_sig.SecretKey) {
	t.Helper()

	testECDSAKey, _ := newTestECDSAKey(t)
	testBLSKey, _ := newTestBLSKey(t)

	return NewBLSKeyManagerFromKeys(testECDSAKey, testBLSKey), testECDSAKey, testBLSKey
}

func testBLSPrivateKeyToPublicKeyBytes(t *testing.T, key *bls_sig.SecretKey) []byte {
	t.Helper()

	pubkeyBytes, err := crypto.BLSSecretKeyToPubkeyBytes(key)
	assert.NoError(t, err)

	return pubkeyBytes
}

func testAggregateBLSSignatureBytes(t *testing.T, sigs ...[]byte) []byte {
	t.Helper()

	blsSignatures := make([]*bls_sig.Signature, len(sigs))

	for idx, sigBytes := range sigs {
		blsSig, err := crypto.UnmarshalBLSSignature(sigBytes)
		assert.NoError(t, err)

		blsSignatures[idx] = blsSig
	}

	aggregatedBLSSig, err := bls_sig.NewSigPop().AggregateSignatures(blsSignatures...)
	assert.NoError(t, err)

	aggregatedBLSSigBytes, err := aggregatedBLSSig.MarshalBinary()
	assert.NoError(t, err)

	return aggregatedBLSSigBytes
}

func TestNewBLSKeyManager(t *testing.T) {
	testECDSAKey, testECDSAKeyEncoded := newTestECDSAKey(t)
	testBLSKey, testBLSKeyEncoded := newTestBLSKey(t)

	testSecretName := func(name string) {
		t.Helper()

		// make sure that the correct key is given
		assert.Contains(
			t,
			[]string{secrets.ValidatorKey, secrets.ValidatorBLSKey},
			name,
		)
	}

	//lint:ignore dupl
	tests := []struct {
		name              string
		mockSecretManager *MockSecretManager
		expectedResult    KeyManager
		expectedErr       error
	}{
		{
			name: "should initialize BLSKeyManager from the loaded ECDSA and BLS key",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					switch name {
					case secrets.ValidatorKey:
						return testECDSAKeyEncoded, nil
					case secrets.ValidatorBLSKey:
						return testBLSKeyEncoded, nil
					}

					return nil, nil
				},
			},
			expectedResult: &BLSKeyManager{
				ecdsaKey: testECDSAKey,
				blsKey:   testBLSKey,
				address:  crypto.PubKeyToAddress(&testECDSAKey.PublicKey),
			},
			expectedErr: nil,
		},
		{
			name: "should return error when getOrCreateECDSAKey returns error",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					switch name {
					case secrets.ValidatorKey:
						// return error instead of key
						return nil, errFake
					case secrets.ValidatorBLSKey:
						return testBLSKeyEncoded, nil
					}

					return nil, nil
				},
			},
			expectedResult: nil,
			expectedErr:    errFake,
		},
		{
			name: "should return error when getOrCreateBLSKey returns error",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					switch name {
					case secrets.ValidatorKey:
						return testECDSAKeyEncoded, nil
					case secrets.ValidatorBLSKey:
						// return error instead of key
						return nil, errFake
					}

					return nil, nil
				},
			},
			expectedResult: nil,
			expectedErr:    errFake,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := NewBLSKeyManager(test.mockSecretManager)

			assert.Equal(t, test.expectedResult, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

func TestNewECDSAKeyManagerFromKeys(t *testing.T) {
	testKey, _ := newTestECDSAKey(t)
	testBLSKey, _ := newTestBLSKey(t)

	assert.Equal(
		t,
		&BLSKeyManager{
			ecdsaKey: testKey,
			blsKey:   testBLSKey,
			address:  crypto.PubKeyToAddress(&testKey.PublicKey),
		},
		NewBLSKeyManagerFromKeys(testKey, testBLSKey),
	)
}

func TestBLSKeyManagerType(t *testing.T) {
	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	assert.Equal(
		t,
		validators.BLSValidatorType,
		blsKeyManager.Type(),
	)
}

func TestBLSKeyManagerAddress(t *testing.T) {
	ecdsaKey, _ := newTestECDSAKey(t)
	blsKey, _ := newTestBLSKey(t)
	blsKeyManager := NewBLSKeyManagerFromKeys(ecdsaKey, blsKey)

	assert.Equal(
		t,
		crypto.PubKeyToAddress(&ecdsaKey.PublicKey),
		blsKeyManager.Address(),
	)
}

func TestBLSKeyManagerNewEmptyValidators(t *testing.T) {
	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	assert.Equal(
		t,
		&validators.BLSValidators{},
		blsKeyManager.NewEmptyValidators(),
	)
}

func TestBLSKeyManagerNewEmptyCommittedSeals(t *testing.T) {
	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	assert.Equal(
		t,
		&BLSSeal{},
		blsKeyManager.NewEmptyCommittedSeals(),
	)
}

func TestBLSKeyManagerSignProposerSeal(t *testing.T) {
	blsKeyManager, _, _ := newTestBLSKeyManager(t)
	msg := crypto.Keccak256(
		hex.MustDecodeHex(testHeaderHashHex),
	)

	proposerSeal, err := blsKeyManager.SignProposerSeal(msg)
	assert.NoError(t, err)

	recoveredAddress, err := ecrecover(proposerSeal, msg)
	assert.NoError(t, err)

	assert.Equal(
		t,
		blsKeyManager.Address(),
		recoveredAddress,
	)
}

func TestBLSKeyManagerSignCommittedSeal(t *testing.T) {
	ecdsaKeyManager, _, blsKey := newTestBLSKeyManager(t)
	blsPubKey, err := blsKey.GetPublicKey()
	assert.NoError(t, err)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	proposerSealBytes, err := ecdsaKeyManager.SignCommittedSeal(msg)
	assert.NoError(t, err)

	proposerSeal, err := crypto.UnmarshalBLSSignature(proposerSealBytes)
	assert.NoError(t, err)

	assert.NoError(
		t,
		crypto.VerifyBLSSignature(
			blsPubKey,
			proposerSeal,
			msg,
		),
	)
}

func TestBLSKeyManagerVerifyCommittedSeal(t *testing.T) {
	blsKeyManager1, _, blsSecretKey1 := newTestBLSKeyManager(t)
	blsKeyManager2, _, _ := newTestBLSKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctSignature, err := blsKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	wrongSignature, err := blsKeyManager2.SignCommittedSeal(msg)
	assert.NoError(t, err)

	blsPublicKey1, err := blsSecretKey1.GetPublicKey()
	assert.NoError(t, err)

	blsPublicKeyBytes, err := blsPublicKey1.MarshalBinary()
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
			validators:  &validators.ECDSAValidators{},
			address:     blsKeyManager1.Address(),
			signature:   []byte{},
			message:     []byte{},
			expectedErr: ErrInvalidValidators,
		},
		{
			name: "should return ErrInvalidSignature if the address is not in the validators",
			validators: &validators.BLSValidators{
				&validators.BLSValidator{
					Address: blsKeyManager2.Address(),
				},
			},
			address:     blsKeyManager1.Address(),
			signature:   []byte{},
			message:     []byte{},
			expectedErr: ErrValidatorNotFound,
		},
		{
			name: "should return crypto.ErrInvalidBLSSignature if it's wrong signature",
			validators: &validators.BLSValidators{
				&validators.BLSValidator{
					Address:      blsKeyManager1.Address(),
					BLSPublicKey: blsPublicKeyBytes,
				},
			},
			address:     blsKeyManager1.Address(),
			signature:   wrongSignature,
			message:     msg,
			expectedErr: crypto.ErrInvalidBLSSignature,
		},
		{
			name: "should return nil if it's correct signature",
			validators: &validators.BLSValidators{
				&validators.BLSValidator{
					Address:      blsKeyManager1.Address(),
					BLSPublicKey: blsPublicKeyBytes,
				},
			},
			address:     blsKeyManager1.Address(),
			signature:   correctSignature,
			message:     msg,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.ErrorIs(
				t,
				test.expectedErr,
				blsKeyManager1.VerifyCommittedSeal(
					test.validators,
					test.address,
					test.signature,
					test.message,
				),
			)
		})
	}
}

func TestBLSKeyManagerGenerateCommittedSeals(t *testing.T) {
	blsKeyManager1, _, blsSecretKey1 := newTestBLSKeyManager(t)
	blsPublicKey1 := testBLSPrivateKeyToPublicKeyBytes(t, blsSecretKey1)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctCommittedSeal, err := blsKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	aggregatedBLSSigBytes := testAggregateBLSSignatureBytes(t, correctCommittedSeal)

	tests := []struct {
		name          string
		sealMap       map[types.Address][]byte
		rawValidators validators.Validators
		expectedRes   Sealer
		expectedErr   error
	}{
		{
			name:          "should return ErrInvalidValidators when rawValidators is not *BLSValidators",
			sealMap:       nil,
			rawValidators: &validators.ECDSAValidators{},
			expectedRes:   nil,
			expectedErr:   ErrInvalidValidators,
		},
		{
			name: "should return error when getBLSSignatures returns error",
			sealMap: map[types.Address][]byte{
				blsKeyManager1.Address(): correctCommittedSeal,
			},
			rawValidators: &validators.BLSValidators{},
			expectedRes:   nil,
			expectedErr:   ErrNonValidatorCommittedSeal,
		},
		{
			name: "should return BLSSeal when it's successful",
			sealMap: map[types.Address][]byte{
				blsKeyManager1.Address(): correctCommittedSeal,
			},
			rawValidators: &validators.BLSValidators{
				validators.NewBLSValidator(
					blsKeyManager1.Address(),
					blsPublicKey1,
				),
			},
			expectedRes: &BLSSeal{
				Bitmap:    big.NewInt(0).SetBit(new(big.Int), 0, 1),
				Signature: aggregatedBLSSigBytes,
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := blsKeyManager1.GenerateCommittedSeals(
				test.sealMap,
				test.rawValidators,
			)

			assert.Equal(t, test.expectedRes, res)

			if test.expectedErr != nil {
				assert.ErrorContains(t, err, test.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBLSKeyManagerVerifyCommittedSeals(t *testing.T) {
	blsKeyManager1, _, blsSecretKey1 := newTestBLSKeyManager(t)
	blsPublicKey1 := testBLSPrivateKeyToPublicKeyBytes(t, blsSecretKey1)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctCommittedSeal, err := blsKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	aggregatedBLSSigBytes := testAggregateBLSSignatureBytes(t, correctCommittedSeal)

	tests := []struct {
		name              string
		rawCommittedSeals Sealer
		hash              []byte
		rawValidators     validators.Validators
		expectedRes       int
		expectedErr       error
	}{
		{
			name:              "should return ErrInvalidCommittedSealType when rawCommittedSeal is not *BLSSeal",
			rawCommittedSeals: &SerializedSeal{},
			hash:              nil,
			rawValidators:     nil,
			expectedRes:       0,
			expectedErr:       ErrInvalidCommittedSealType,
		},
		{
			name: "should return ErrInvalidValidators when rawValidators is not *BLSValidators",
			rawCommittedSeals: &BLSSeal{
				Bitmap:    big.NewInt(0).SetBit(new(big.Int), 0, 1),
				Signature: aggregatedBLSSigBytes,
			},
			rawValidators: &validators.ECDSAValidators{},
			expectedRes:   0,
			expectedErr:   ErrInvalidValidators,
		},
		{
			name: "should return size of BLSSeal when it's successful",
			rawCommittedSeals: &BLSSeal{
				Bitmap:    big.NewInt(0).SetBit(new(big.Int), 0, 1),
				Signature: aggregatedBLSSigBytes,
			},
			rawValidators: &validators.BLSValidators{
				validators.NewBLSValidator(
					blsKeyManager1.Address(),
					blsPublicKey1,
				),
			},
			expectedRes: 1,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := blsKeyManager1.VerifyCommittedSeals(
				test.rawCommittedSeals,
				msg,
				test.rawValidators,
			)

			assert.Equal(t, test.expectedRes, res)

			if test.expectedErr != nil {
				assert.ErrorContains(t, err, test.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
