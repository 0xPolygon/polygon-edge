package signer

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
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

func testBLSKeyManagerToBLSValidator(t *testing.T, keyManager KeyManager) *validators.BLSValidator {
	t.Helper()

	blsKeyManager, ok := keyManager.(*BLSKeyManager)
	assert.True(t, ok)

	pubkeyBytes, err := crypto.BLSSecretKeyToPubkeyBytes(blsKeyManager.blsKey)
	assert.NoError(t, err)

	return validators.NewBLSValidator(
		blsKeyManager.Address(),
		pubkeyBytes,
	)
}

func testCreateAggregatedSignature(t *testing.T, msg []byte, keyManagers ...KeyManager) []byte {
	t.Helper()

	signatures := make([][]byte, len(keyManagers))

	for idx, km := range keyManagers {
		sig, err := km.SignCommittedSeal(msg)
		assert.NoError(t, err)

		signatures[idx] = sig
	}

	return testAggregateBLSSignatureBytes(t, signatures...)
}

// assert equality of marshalled aggregated BLS Public Keys
// because the field values in MultiPublicKey may be different for the same keys
func assertEqualAggregatedBLSPublicKeys(t *testing.T, apk1, apk2 *bls_sig.MultiPublicKey) {
	t.Helper()

	apkBytes1, err := apk1.MarshalBinary()
	assert.NoError(t, err)

	apkBytes2, err := apk2.MarshalBinary()
	assert.NoError(t, err)

	assert.Equal(t, apkBytes1, apkBytes2)
}

func TestNewBLSKeyManager(t *testing.T) {
	t.Parallel()

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
			name: "should return error if getOrCreateECDSAKey returns error",
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
						return nil, errTest
					case secrets.ValidatorBLSKey:
						return testBLSKeyEncoded, nil
					}

					return nil, nil
				},
			},
			expectedResult: nil,
			expectedErr:    errTest,
		},
		{
			name: "should return error if getOrCreateBLSKey returns error",
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
						return nil, errTest
					}

					return nil, nil
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

			res, err := NewBLSKeyManager(test.mockSecretManager)

			assert.Equal(t, test.expectedResult, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

func TestNewECDSAKeyManagerFromKeys(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	assert.Equal(
		t,
		validators.BLSValidatorType,
		blsKeyManager.Type(),
	)
}

func TestBLSKeyManagerAddress(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	assert.Equal(
		t,
		validators.NewBLSValidatorSet(),
		blsKeyManager.NewEmptyValidators(),
	)
}

func TestBLSKeyManagerNewEmptyCommittedSeals(t *testing.T) {
	t.Parallel()

	blsKeyManager, _, _ := newTestBLSKeyManager(t)

	assert.Equal(
		t,
		&AggregatedSeal{},
		blsKeyManager.NewEmptyCommittedSeals(),
	)
}

func TestBLSKeyManagerSignProposerSeal(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
			validators:  validators.NewECDSAValidatorSet(),
			address:     blsKeyManager1.Address(),
			signature:   []byte{},
			message:     []byte{},
			expectedErr: ErrInvalidValidators,
		},
		{
			name: "should return ErrInvalidSignature if the address is not in the validators",
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, blsKeyManager2),
			),
			address:     blsKeyManager1.Address(),
			signature:   []byte{},
			message:     []byte{},
			expectedErr: ErrValidatorNotFound,
		},
		{
			name: "should return crypto.ErrInvalidBLSSignature if it's wrong signature",
			validators: validators.NewBLSValidatorSet(
				validators.NewBLSValidator(
					blsKeyManager1.Address(),
					blsPublicKeyBytes,
				),
			),
			address:     blsKeyManager1.Address(),
			signature:   wrongSignature,
			message:     msg,
			expectedErr: crypto.ErrInvalidBLSSignature,
		},
		{
			name: "should return nil if it's correct signature",
			validators: validators.NewBLSValidatorSet(
				validators.NewBLSValidator(
					blsKeyManager1.Address(),
					blsPublicKeyBytes,
				),
			),
			address:     blsKeyManager1.Address(),
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
	t.Parallel()

	blsKeyManager1, _, _ := newTestBLSKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctCommittedSeal, err := blsKeyManager1.SignCommittedSeal(msg)
	assert.NoError(t, err)

	aggregatedBLSSigBytes := testCreateAggregatedSignature(
		t,
		msg,
		blsKeyManager1,
	)

	tests := []struct {
		name        string
		sealMap     map[types.Address][]byte
		validators  validators.Validators
		expectedRes Seals
		expectedErr error
	}{
		{
			name:        "should return ErrInvalidValidators if rawValidators is not *BLSValidators",
			sealMap:     nil,
			validators:  validators.NewECDSAValidatorSet(),
			expectedRes: nil,
			expectedErr: ErrInvalidValidators,
		},
		{
			name: "should return error if getBLSSignatures returns error",
			sealMap: map[types.Address][]byte{
				blsKeyManager1.Address(): correctCommittedSeal,
			},
			validators:  validators.NewBLSValidatorSet(),
			expectedRes: nil,
			expectedErr: ErrNonValidatorCommittedSeal,
		},
		{
			name:        "should return error if sealMap is empty",
			sealMap:     map[types.Address][]byte{},
			validators:  validators.NewBLSValidatorSet(),
			expectedRes: nil,
			expectedErr: errors.New("at least one signature is required"),
		},
		{
			name: "should return AggregatedSeal if it's successful",
			sealMap: map[types.Address][]byte{
				blsKeyManager1.Address(): correctCommittedSeal,
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(
					t,
					blsKeyManager1,
				),
			),
			expectedRes: &AggregatedSeal{
				Bitmap:    big.NewInt(0).SetBit(new(big.Int), 0, 1),
				Signature: aggregatedBLSSigBytes,
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := blsKeyManager1.GenerateCommittedSeals(
				test.sealMap,
				test.validators,
			)

			assert.Equal(t, test.expectedRes, res)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
		})
	}
}

func TestBLSKeyManagerVerifyCommittedSeals(t *testing.T) {
	t.Parallel()

	blsKeyManager1, _, _ := newTestBLSKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	aggregatedBLSSigBytes := testCreateAggregatedSignature(
		t,
		msg,
		blsKeyManager1,
	)

	tests := []struct {
		name              string
		rawCommittedSeals Seals
		hash              []byte
		validators        validators.Validators
		expectedRes       int
		expectedErr       error
	}{
		{
			name:              "should return ErrInvalidCommittedSealType if rawCommittedSeal is not *AggregatedSeal",
			rawCommittedSeals: &SerializedSeal{},
			hash:              nil,
			validators:        nil,
			expectedRes:       0,
			expectedErr:       ErrInvalidCommittedSealType,
		},
		{
			name: "should return ErrInvalidValidators if rawValidators is not *BLSValidators",
			rawCommittedSeals: &AggregatedSeal{
				Bitmap:    big.NewInt(0).SetBit(new(big.Int), 0, 1),
				Signature: aggregatedBLSSigBytes,
			},
			validators:  validators.NewECDSAValidatorSet(),
			expectedRes: 0,
			expectedErr: ErrInvalidValidators,
		},
		{
			name: "should return size of AggregatedSeal if it's successful",
			rawCommittedSeals: &AggregatedSeal{
				Bitmap:    big.NewInt(0).SetBit(new(big.Int), 0, 1),
				Signature: aggregatedBLSSigBytes,
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(
					t,
					blsKeyManager1,
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

			res, err := blsKeyManager1.VerifyCommittedSeals(
				test.rawCommittedSeals,
				msg,
				test.validators,
			)

			assert.Equal(t, test.expectedRes, res)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
		})
	}
}

func TestBLSKeyManagerSignIBFTMessageAndEcrecover(t *testing.T) {
	t.Parallel()

	blsKeyManager, _, _ := newTestBLSKeyManager(t)
	msg := crypto.Keccak256([]byte("message"))

	proposerSeal, err := blsKeyManager.SignIBFTMessage(msg)
	assert.NoError(t, err)

	recoveredAddress, err := blsKeyManager.Ecrecover(proposerSeal, msg)
	assert.NoError(t, err)

	assert.Equal(
		t,
		blsKeyManager.Address(),
		recoveredAddress,
	)
}

func Test_getBLSSignatures(t *testing.T) {
	t.Parallel()

	validatorKeyManager, _, _ := newTestBLSKeyManager(t)
	nonValidatorKeyManager, _, _ := newTestBLSKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	validatorCommittedSeal, err := validatorKeyManager.SignCommittedSeal(msg)
	assert.NoError(t, err)

	nonValidatorCommittedSeal, err := nonValidatorKeyManager.SignCommittedSeal(msg)
	assert.NoError(t, err)

	wrongCommittedSeal := []byte("fake committed seal")

	validatorSignature, err := crypto.UnmarshalBLSSignature(validatorCommittedSeal)
	assert.NoError(t, err)

	tests := []struct {
		name               string
		sealMap            map[types.Address][]byte
		validators         validators.Validators
		expectedSignatures []*bls_sig.Signature
		expectedBitMap     *big.Int
		expectedErr        error
	}{
		{
			name: "should return ErrNonValidatorCommittedSeal if sealMap has committed seal signed by non validator",
			sealMap: map[types.Address][]byte{
				nonValidatorKeyManager.Address(): nonValidatorCommittedSeal,
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(
					t,
					validatorKeyManager,
				),
			),
			expectedSignatures: nil,
			expectedBitMap:     nil,
			expectedErr:        ErrNonValidatorCommittedSeal,
		},
		{
			name: "should return error if unmarshalling committed seal is failed",
			sealMap: map[types.Address][]byte{
				validatorKeyManager.Address(): wrongCommittedSeal,
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(
					t,
					validatorKeyManager,
				),
			),
			expectedSignatures: nil,
			expectedBitMap:     nil,
			expectedErr:        errors.New("signature must be 96 bytes"),
		},
		{
			name: "should return signatures and bitmap if all committed seals are right and signed by validators",
			sealMap: map[types.Address][]byte{
				validatorKeyManager.Address(): validatorCommittedSeal,
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(
					t,
					validatorKeyManager,
				),
			),
			expectedSignatures: []*bls_sig.Signature{
				validatorSignature,
			},
			expectedBitMap: new(big.Int).SetBit(new(big.Int), 0, 1),
			expectedErr:    nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sigs, bitmap, err := getBLSSignatures(
				test.sealMap,
				test.validators,
			)

			assert.ElementsMatch(
				t,
				test.expectedSignatures,
				sigs,
			)
			assert.Equal(t, test.expectedBitMap, bitmap)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
		})
	}

	t.Run("multiple committed seals by validators", func(t *testing.T) {
		t.Parallel()

		// which validator signed committed seals
		signerFlags := []bool{
			false,
			true,
			false,
			true,
			true,
		}

		msg := crypto.Keccak256(
			wrapCommitHash(
				hex.MustDecodeHex(testHeaderHashHex),
			),
		)

		sealMap := make(map[types.Address][]byte)
		validators := validators.NewBLSValidatorSet()

		expectedSignatures := make([]*bls_sig.Signature, 0, len(signerFlags))
		expectedBitMap := new(big.Int)

		for idx, signed := range signerFlags {
			blsKeyManager, _, _ := newTestBLSKeyManager(t)

			// add to validators
			assert.NoError(
				t,
				validators.Add(
					testBLSKeyManagerToBLSValidator(
						t,
						blsKeyManager,
					),
				),
			)

			if !signed {
				continue
			}

			committedSeal, err := blsKeyManager.SignCommittedSeal(msg)
			assert.NoError(t, err)

			// set committed seals to sealMap
			sealMap[blsKeyManager.Address()] = committedSeal

			// build expected signatures
			signature, err := crypto.UnmarshalBLSSignature(committedSeal)
			assert.NoError(t, err)

			expectedSignatures = append(expectedSignatures, signature)

			// build expected bit map
			expectedBitMap = expectedBitMap.SetBit(expectedBitMap, idx, 1)
		}

		signatures, bitmap, err := getBLSSignatures(
			sealMap,
			validators,
		)

		// the order might be different due to scanning sealMap
		assert.ElementsMatch(
			t,
			expectedSignatures,
			signatures,
		)
		assert.Equal(t, expectedBitMap, bitmap)
		assert.NoError(t, err)
	})
}

func Test_createAggregatedBLSPubKeys(t *testing.T) {
	t.Parallel()

	t.Run("multiple validators", func(t *testing.T) {
		t.Parallel()

		// which validator signed committed seals
		signerFlags := []bool{
			false,
			true,
			false,
			true,
			true,
		}

		validators := validators.NewBLSValidatorSet()
		bitMap := new(big.Int)

		expectedBLSPublicKeys := []*bls_sig.PublicKey{}
		expectedNumSigners := 0

		for idx, signed := range signerFlags {
			blsKeyManager, _, blsSecretKey := newTestBLSKeyManager(t)

			// add to validators
			assert.NoError(
				t,
				validators.Add(
					testBLSKeyManagerToBLSValidator(
						t,
						blsKeyManager,
					),
				),
			)

			if !signed {
				continue
			}

			// set bit in bitmap
			bitMap = bitMap.SetBit(bitMap, idx, 1)

			blsPubKey, err := blsSecretKey.GetPublicKey()
			assert.NoError(t, err)

			expectedBLSPublicKeys = append(expectedBLSPublicKeys, blsPubKey)
			expectedNumSigners++
		}

		expectedAggregatedBLSPublicKeys, err := bls_sig.NewSigPop().AggregatePublicKeys(
			expectedBLSPublicKeys...,
		)
		assert.NoError(t, err)

		aggregatedPubKey, num, err := createAggregatedBLSPubKeys(
			validators,
			bitMap,
		)

		assert.NoError(t, err)
		assert.Equal(t, expectedNumSigners, num)

		assertEqualAggregatedBLSPublicKeys(t, expectedAggregatedBLSPublicKeys, aggregatedPubKey)
	})

	t.Run("should return error if bitMap is empty", func(t *testing.T) {
		t.Parallel()

		aggrecatedPubKeys, num, err := createAggregatedBLSPubKeys(
			validators.NewBLSValidatorSet(),
			new(big.Int),
		)

		assert.Nil(t, aggrecatedPubKeys)
		assert.Zero(t, num)
		assert.ErrorContains(t, err, "at least one public key is required")
	})

	t.Run("should return error if public key is wrong", func(t *testing.T) {
		t.Parallel()

		aggrecatedPubKeys, num, err := createAggregatedBLSPubKeys(
			validators.NewBLSValidatorSet(
				validators.NewBLSValidator(
					types.StringToAddress("0"),
					[]byte("fake"),
				),
			),
			new(big.Int).SetBit(new(big.Int), 0, 1),
		)

		assert.Nil(t, aggrecatedPubKeys)
		assert.Zero(t, num)
		assert.ErrorContains(t, err, "public key must be 48 bytes")
	})
}

func Test_verifyBLSCommittedSealsImpl(t *testing.T) {
	t.Parallel()

	validatorKeyManager1, _, _ := newTestBLSKeyManager(t)
	validatorKeyManager2, _, _ := newTestBLSKeyManager(t)
	validatorKeyManager3, _, _ := newTestBLSKeyManager(t)
	validatorKeyManager4, _, _ := newTestBLSKeyManager(t)

	msg := crypto.Keccak256(
		wrapCommitHash(
			hex.MustDecodeHex(testHeaderHashHex),
		),
	)

	correctAggregatedSig := testCreateAggregatedSignature(
		t,
		msg,
		validatorKeyManager1,
		validatorKeyManager2,
	)

	wrongAggregatedSig := testCreateAggregatedSignature(
		t,
		[]byte("fake"),
		validatorKeyManager1,
		validatorKeyManager2,
	)

	tests := []struct {
		name          string
		committedSeal *AggregatedSeal
		msg           []byte
		validators    validators.Validators
		expectedRes   int
		expectedErr   error
	}{
		{
			name: "should return ErrEmptyCommittedSeals if committedSeal.Signature is empty",
			committedSeal: &AggregatedSeal{
				Signature: []byte{},
				Bitmap:    new(big.Int).SetBit(new(big.Int), 0, 1),
			},
			expectedRes: 0,
			expectedErr: ErrEmptyCommittedSeals,
		},
		{
			name: "should return ErrEmptyCommittedSeals if committedSeal.BitMap is nil",
			committedSeal: &AggregatedSeal{
				Signature: []byte("test"),
				Bitmap:    nil,
			},
			expectedRes: 0,
			expectedErr: ErrEmptyCommittedSeals,
		},
		{
			name: "should return ErrEmptyCommittedSeals if committedSeal.BitMap is zero",
			committedSeal: &AggregatedSeal{
				Signature: []byte("test"),
				Bitmap:    new(big.Int),
			},
			expectedRes: 0,
			expectedErr: ErrEmptyCommittedSeals,
		},
		{
			name: "should return error if failed to aggregate public keys",
			committedSeal: &AggregatedSeal{
				Signature: []byte("test"),
				Bitmap:    new(big.Int).SetBit(new(big.Int), 0, 1),
			},
			validators: validators.NewBLSValidatorSet(
				&validators.BLSValidator{
					BLSPublicKey: []byte("test"),
				},
			),
			expectedRes: 0,
			expectedErr: errors.New("failed to aggregate BLS Public Keys: public key must be 48 bytes"),
		},
		{
			name: "should return error if failed to unmarshal aggregated signature",
			committedSeal: &AggregatedSeal{
				Signature: []byte("test"),
				Bitmap:    new(big.Int).SetBit(new(big.Int), 0, 1),
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager1),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager2),
			),
			expectedRes: 0,
			expectedErr: errors.New("multi signature must be 96 bytes"),
		},
		{
			name: "should return error if message is nil",
			committedSeal: &AggregatedSeal{
				Signature: correctAggregatedSig,
				Bitmap:    new(big.Int).SetBit(new(big.Int), 0, 1),
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager1),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager2),
			),
			msg:         nil,
			expectedRes: 0,
			expectedErr: errors.New("signature and message and public key cannot be nil or zero"),
		},
		{
			name: "should return ErrInvalidSignature if verification failed (different message)",
			committedSeal: &AggregatedSeal{
				Signature: wrongAggregatedSig,
				Bitmap:    new(big.Int).SetBytes([]byte{0x3}), // validator1 & validator2
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager1),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager2),
			),
			msg:         msg,
			expectedRes: 0,
			expectedErr: ErrInvalidSignature,
		},
		{
			name: "should return ErrInvalidSignature if verification failed (wrong validator set)",
			committedSeal: &AggregatedSeal{
				Signature: correctAggregatedSig,
				Bitmap:    new(big.Int).SetBytes([]byte{0x3}), // validator1 & validator 2
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager3),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager4),
			),
			msg:         msg,
			expectedRes: 0,
			expectedErr: ErrInvalidSignature,
		},
		{
			name: "should return ErrInvalidSignature if verification failed (smaller validator set)",
			committedSeal: &AggregatedSeal{
				Signature: correctAggregatedSig,
				Bitmap:    new(big.Int).SetBytes([]byte{0x1}), // validator1
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager1),
			),
			msg:         msg,
			expectedRes: 0,
			expectedErr: ErrInvalidSignature,
		},
		{
			name: "should return ErrInvalidSignature if verification failed (bigger validator set)",
			committedSeal: &AggregatedSeal{
				Signature: correctAggregatedSig,
				Bitmap:    new(big.Int).SetBytes([]byte{0x7}), // validator1 & validator 2 & validator 3
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager1),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager2),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager3),
			),
			msg:         msg,
			expectedRes: 0,
			expectedErr: ErrInvalidSignature,
		},
		{
			name: "should succeed",
			committedSeal: &AggregatedSeal{
				Signature: correctAggregatedSig,
				Bitmap:    new(big.Int).SetBytes([]byte{0x3}), // validator1 & validator 2
			},
			validators: validators.NewBLSValidatorSet(
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager1),
				testBLSKeyManagerToBLSValidator(t, validatorKeyManager2),
			),
			msg:         msg,
			expectedRes: 2,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := verifyBLSCommittedSealsImpl(
				test.committedSeal,
				test.msg,
				test.validators,
			)

			assert.Equal(t, test.expectedRes, res)
			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
		})
	}
}
