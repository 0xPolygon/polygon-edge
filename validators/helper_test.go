package validators

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	addr1          = types.StringToAddress("1")
	addr2          = types.StringToAddress("2")
	testBLSPubKey1 = BLSValidatorPublicKey([]byte("bls_pubkey1"))
	testBLSPubKey2 = BLSValidatorPublicKey([]byte("bls_pubkey2"))

	ecdsaValidator1 = NewECDSAValidator(addr1)
	ecdsaValidator2 = NewECDSAValidator(addr2)
	blsValidator1   = NewBLSValidator(addr1, testBLSPubKey1)
	blsValidator2   = NewBLSValidator(addr2, testBLSPubKey2)

	fakeValidatorType = ValidatorType("fake")
)

func createTestBLSValidatorString(
	addr types.Address,
	blsPubKey []byte,
) string {
	return fmt.Sprintf(
		"%s:%s",
		addr.String(),
		"0x"+hex.EncodeToString(blsPubKey),
	)
}

func TestNewValidatorFromType(t *testing.T) {
	tests := []struct {
		name          string
		validatorType ValidatorType
		expected      Validator
	}{
		{
			name:          "ECDSAValidator",
			validatorType: ECDSAValidatorType,
			expected:      new(ECDSAValidator),
		},
		{
			name:          "BLSValidator",
			validatorType: BLSValidatorType,
			expected:      new(BLSValidator),
		},
		{
			name:          "undefined type",
			validatorType: fakeValidatorType,
			expected:      nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				NewValidatorFromType(test.validatorType),
			)
		})
	}
}

func TestNewValidatorsFromType(t *testing.T) {
	tests := []struct {
		name          string
		validatorType ValidatorType
		expected      Validators
	}{
		{
			name:          "ECDSAValidators",
			validatorType: ECDSAValidatorType,
			expected:      new(ECDSAValidators),
		},
		{
			name:          "BLSValidators",
			validatorType: BLSValidatorType,
			expected:      new(BLSValidators),
		},
		{
			name:          "undefined type",
			validatorType: fakeValidatorType,
			expected:      nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				NewValidatorsFromType(test.validatorType),
			)
		})
	}
}

func TestParseValidator(t *testing.T) {
	tests := []struct {
		name string
		// inputs
		validatorType ValidatorType
		validatorStr  string
		// outputs
		expectedValidator Validator
		expectedErr       error
	}{
		{
			name:              "ECDSAValidator",
			validatorType:     ECDSAValidatorType,
			validatorStr:      addr1.String(),
			expectedValidator: ecdsaValidator1,
			expectedErr:       nil,
		},
		{
			name:              "BLSValidator",
			validatorType:     BLSValidatorType,
			validatorStr:      createTestBLSValidatorString(addr1, testBLSPubKey1),
			expectedValidator: blsValidator1,
			expectedErr:       nil,
		},
		{
			name:              "undefined type",
			validatorType:     fakeValidatorType,
			validatorStr:      addr1.String(),
			expectedValidator: nil,
			expectedErr:       fmt.Errorf("invalid validator type: %s", fakeValidatorType),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			val, err := ParseValidator(
				test.validatorType,
				test.validatorStr,
			)

			assert.Equal(t, test.expectedValidator, val)

			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestParseValidators(t *testing.T) {
	tests := []struct {
		name string
		// inputs
		validatorType ValidatorType
		validatorStrs []string
		// outputs
		expectedValidators Validators
		expectedErr        error
	}{
		{
			name:          "ECDSAValidator",
			validatorType: ECDSAValidatorType,
			validatorStrs: []string{
				addr1.String(),
				addr2.String(),
			},
			expectedValidators: &ECDSAValidators{
				ecdsaValidator1,
				ecdsaValidator2,
			},
			expectedErr: nil,
		},
		{
			name:          "BLSValidator",
			validatorType: BLSValidatorType,
			validatorStrs: []string{
				createTestBLSValidatorString(addr1, testBLSPubKey1),
				createTestBLSValidatorString(addr2, testBLSPubKey2),
			},
			expectedValidators: &BLSValidators{
				blsValidator1,
				blsValidator2,
			},
			expectedErr: nil,
		},
		{
			name:          "undefined type",
			validatorType: fakeValidatorType,
			validatorStrs: []string{
				createTestBLSValidatorString(addr1, testBLSPubKey1),
				createTestBLSValidatorString(addr2, testBLSPubKey2),
			},
			expectedValidators: nil,
			expectedErr:        fmt.Errorf("invalid validator type: %s", fakeValidatorType),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			vals, err := ParseValidators(
				test.validatorType,
				test.validatorStrs,
			)

			assert.Equal(t, test.expectedValidators, vals)

			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestParseECDSAValidator(t *testing.T) {
	assert.Equal(
		t,
		ecdsaValidator1,
		ParseECDSAValidator(addr1.String()),
	)
}

func TestParseBLSValidator(t *testing.T) {
	tests := []struct {
		name string
		// inputs
		validatorStr string
		// outputs
		expectedValidator *BLSValidator
		expectedErr       error
	}{
		{
			name:              "should parse correctly",
			validatorStr:      createTestBLSValidatorString(addr1, testBLSPubKey1),
			expectedValidator: blsValidator1,
			expectedErr:       nil,
		},
		{
			name:              "incorrect format",
			validatorStr:      addr1.String(),
			expectedValidator: nil,
			expectedErr:       ErrInvalidBLSValidatorFormat,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			val, err := ParseBLSValidator(
				test.validatorStr,
			)

			assert.Equal(t, test.expectedValidator, val)

			assert.Equal(t, test.expectedErr, err)
		})
	}
}
