package signer

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/stretchr/testify/assert"
)

var (
	testHeader = &types.Header{
		ParentHash:   types.BytesToHash(crypto.Keccak256([]byte{0x1})),
		Sha3Uncles:   types.BytesToHash(crypto.Keccak256([]byte{0x2})),
		Miner:        crypto.Keccak256([]byte{0x3}),
		StateRoot:    types.BytesToHash(crypto.Keccak256([]byte{0x4})),
		TxRoot:       types.BytesToHash(crypto.Keccak256([]byte{0x5})),
		ReceiptsRoot: types.BytesToHash(crypto.Keccak256([]byte{0x6})),
		LogsBloom:    types.Bloom{0x7},
		Difficulty:   8,
		Number:       9,
		GasLimit:     10,
		GasUsed:      11,
		Timestamp:    12,
		ExtraData:    crypto.Keccak256([]byte{0x13}),
	}

	testHeaderHashHex = "0xd6701b3d601fd78734ce2f2542dc3d9cc1c75b1ed980c61c8d69cd2cb638f89c"
)

func newTestECDSAKey(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()

	testKey, testKeyEncoded, err := crypto.GenerateAndEncodeECDSAPrivateKey()
	assert.NoError(t, err, "failed to initialize ECDSA key")

	return testKey, testKeyEncoded
}

func newTestBLSKey(t *testing.T) (*bls_sig.SecretKey, []byte) {
	t.Helper()

	testKey, testKeyEncoded, err := crypto.GenerateAndEncodeBLSSecretKey()

	assert.NoError(t, err, "failed to initialize test ECDSA key")

	return testKey, testKeyEncoded
}

// Make sure the target function always returns the same result
func Test_wrapCommitHash(t *testing.T) {
	t.Parallel()

	var (
		input             = crypto.Keccak256([]byte{0x1})
		expectedOutputHex = "0x8a319084d2e52be9c9192645aa98900413ee2a7c93c2916ef99d62218207d1da"
	)

	expectedOutput, err := hex.DecodeHex(expectedOutputHex)
	if err != nil {
		t.Fatalf("failed to parse expected output: %s, %v", expectedOutputHex, err)
	}

	output := wrapCommitHash(input)

	assert.Equal(t, expectedOutput, output)
}

//nolint
func Test_getOrCreateECDSAKey(t *testing.T) {
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
		expectedResult    *ecdsa.PrivateKey
		expectedErr       error
	}{
		{
			name: "should load ECDSA key from secret manager if the key exists",
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
			expectedResult: testKey,
			expectedErr:    nil,
		},
		{
			name: "should create new ECDSA key if the key doesn't exist",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return false
				},
				SetSecretFn: func(name string, key []byte) error {
					testSecretName(name)

					assert.NotEqual(t, testKeyEncoded, key)

					return nil
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					return testKeyEncoded, nil
				},
			},
			expectedResult: testKey,
			expectedErr:    nil,
		},
		{
			name: "should return error if secret manager returns error",
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
		{
			name: "should return error if the key manager fails to generate new ECDSA key",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return false
				},
				SetSecretFn: func(name string, key []byte) error {
					testSecretName(name)

					return errTest
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

			res, err := getOrCreateECDSAKey(test.mockSecretManager)

			assert.Equal(t, test.expectedResult, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

//nolint
func Test_getOrCreateBLSKey(t *testing.T) {
	t.Parallel()

	testKey, testKeyEncoded := newTestBLSKey(t)

	testSecretName := func(name string) {
		t.Helper()

		// make sure that the correct key is given
		assert.Equal(t, secrets.ValidatorBLSKey, name)
	}

	tests := []struct {
		name              string
		mockSecretManager *MockSecretManager
		expectedResult    *bls_sig.SecretKey
		expectedErr       error
	}{
		{
			name: "should load BLS key from secret manager if the key exists",
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
			expectedResult: testKey,
			expectedErr:    nil,
		},
		{
			name: "should create new BLS key if the key doesn't exist",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return false
				},
				SetSecretFn: func(name string, key []byte) error {
					testSecretName(name)

					assert.NotEqual(t, testKeyEncoded, key)

					return nil
				},
				GetSecretFn: func(name string) ([]byte, error) {
					testSecretName(name)

					return testKeyEncoded, nil
				},
			},
			expectedResult: testKey,
			expectedErr:    nil,
		},
		{
			name: "should return error if secret manager returns error",
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
		{
			name: "should return error if the key manager fails to generate new BLS key",
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					testSecretName(name)

					return false
				},
				SetSecretFn: func(name string, key []byte) error {
					testSecretName(name)

					return errTest
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

			res, err := getOrCreateBLSKey(test.mockSecretManager)

			assert.Equal(t, test.expectedResult, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

// make sure that header hash calculation returns the same hash
func Test_calculateHeaderHash(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		types.StringToHash(testHeaderHashHex),
		calculateHeaderHash(testHeader),
	)
}

func Test_ecrecover(t *testing.T) {
	t.Parallel()

	testKey, _ := newTestECDSAKey(t)
	signerAddress := crypto.PubKeyToAddress(&testKey.PublicKey)

	rawMessage := crypto.Keccak256([]byte{0x1})

	signature, err := crypto.Sign(
		testKey,
		rawMessage,
	)
	assert.NoError(t, err)

	recoveredAddress, err := ecrecover(signature, rawMessage)
	assert.NoError(t, err)

	assert.Equal(
		t,
		signerAddress,
		recoveredAddress,
	)
}

func TestNewKeyManagerFromType(t *testing.T) {
	t.Parallel()

	testECDSAKey, testECDSAKeyEncoded := newTestECDSAKey(t)
	testBLSKey, testBLSKeyEncoded := newTestBLSKey(t)

	tests := []struct {
		name              string
		validatorType     validators.ValidatorType
		mockSecretManager *MockSecretManager
		expectedRes       KeyManager
		expectedErr       error
	}{
		{
			name:          "ECDSAValidatorType",
			validatorType: validators.ECDSAValidatorType,
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					return testECDSAKeyEncoded, nil
				},
			},
			expectedRes: NewECDSAKeyManagerFromKey(testECDSAKey),
			expectedErr: nil,
		},
		{
			name:          "BLSValidatorType",
			validatorType: validators.BLSValidatorType,
			mockSecretManager: &MockSecretManager{
				HasSecretFn: func(name string) bool {
					return true
				},
				GetSecretFn: func(name string) ([]byte, error) {
					switch name {
					case secrets.ValidatorKey:
						return testECDSAKeyEncoded, nil
					case secrets.ValidatorBLSKey:
						return testBLSKeyEncoded, nil
					}

					return nil, fmt.Errorf("unexpected key name: %s", name)
				},
			},
			expectedRes: NewBLSKeyManagerFromKeys(testECDSAKey, testBLSKey),
		},
		{
			name:          "unsupported type",
			validatorType: validators.ValidatorType("fake"),
			expectedRes:   nil,
			expectedErr:   errors.New("unsupported validator type: fake"),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := NewKeyManagerFromType(test.mockSecretManager, test.validatorType)

			assert.Equal(t, test.expectedRes, res)

			if test.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorContains(t, err, test.expectedErr.Error())
			}
		})
	}
}

func Test_verifyIBFTExtraSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		extraData []byte
		isError   bool
	}{
		{
			name:      "should return error if ExtraData size is 0",
			extraData: make([]byte, 0),
			isError:   true,
		},
		{
			name:      "should return error if ExtraData size is less than IstanbulExtraVanity",
			extraData: make([]byte, IstanbulExtraVanity-1),
			isError:   true,
		},
		{
			name:      "should return nil if ExtraData size matches with IstanbulExtraVanity",
			extraData: make([]byte, IstanbulExtraVanity),
			isError:   false,
		},
		{
			name:      "should return nil if ExtraData size is greater than IstanbulExtraVanity",
			extraData: make([]byte, IstanbulExtraVanity+1),
			isError:   false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := &types.Header{
				ExtraData: test.extraData,
			}

			err := verifyIBFTExtraSize(header)

			if test.isError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
