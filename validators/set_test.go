package validators

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSetType(t *testing.T) {
	t.Parallel()

	t.Run("ECDSAValidators", func(t *testing.T) {
		t.Parallel()

		assert.Equal(
			t,
			ECDSAValidatorType,
			NewECDSAValidatorSet().Type(),
		)
	})

	t.Run("BLSValidators", func(t *testing.T) {
		t.Parallel()

		assert.Equal(
			t,
			BLSValidatorType,
			NewBLSValidatorSet().Type(),
		)
	})
}

func TestSetLen(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		2,
		NewECDSAValidatorSet(
			NewECDSAValidator(addr1),
			NewECDSAValidator(addr2),
		).Len(),
	)
}

func TestSetEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		vals1    Validators
		vals2    Validators
		expected bool
	}{
		{
			name: "types are not equal",
			vals1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			vals2: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
			expected: false,
		},
		{
			name: "sizes are not equal",
			vals1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			vals2: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			expected: false,
		},
		{
			name: "equal (ECDSAValidators)",
			vals1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			vals2: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			expected: true,
		},
		{
			name: "not equal (ECDSAValidators)",
			vals1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			vals2: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr1),
			),
			expected: false,
		},
		{
			name: "equal (BLSValidators)",
			vals1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			vals2: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			expected: true,
		},
		{
			name: "not equal (BLSValidators)",
			vals1: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
			vals2: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr1, testBLSPubKey1),
			),
			expected: false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.vals1.Equal(test.vals2),
			)
		})
	}
}

func TestSetCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		validators Validators
	}{
		{
			name: "ECDSAValidators",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
		},
		{
			name: "BLSValidators",
			validators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr1, testBLSPubKey1),
			),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			copied := test.validators.Copy()

			assert.Equal(t, test.validators, copied)

			// check the addresses are different
			for i := 0; i < test.validators.Len(); i++ {
				assert.NotSame(
					t,
					test.validators.At(uint64(i)),
					copied.At(uint64(i)),
				)
			}
		})
	}
}

func TestSetAt(t *testing.T) {
	t.Parallel()

	validators := NewECDSAValidatorSet(
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	)

	set, ok := validators.(*Set)
	assert.True(t, ok)

	for idx, val := range set.Validators {
		assert.Equal(
			t,
			val,
			set.At(uint64(idx)),
		)

		// check the addresses are same
		assert.Same(
			t,
			val,
			set.At(uint64(idx)),
		)
	}
}

func TestSetIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		validators Validators
		addr       types.Address
		expected   int64
	}{
		{
			name: "ECDSAValidators",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			addr:     addr1,
			expected: 0,
		},
		{
			name: "BLSValidators",
			validators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
			addr:     addr2,
			expected: 1,
		},
		{
			name: "not found",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			addr:     types.StringToAddress("fake"),
			expected: -1,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.validators.Index(test.addr),
			)
		})
	}
}

func TestSetIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		validators Validators
		addr       types.Address
		expected   bool
	}{
		{
			name: "ECDSAValidators",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			addr:     addr1,
			expected: true,
		},
		{
			name: "BLSValidators",
			validators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
			addr:     addr2,
			expected: true,
		},
		{
			name: "not found",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			addr:     types.StringToAddress("fake"),
			expected: false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.validators.Includes(test.addr),
			)
		})
	}
}

func TestSetAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		validators         Validators
		newValidator       Validator
		expectedErr        error
		expectedValidators Validators
	}{
		{
			name: "should return error in case of type mismatch",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			newValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:  ErrMismatchValidatorType,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
		},
		{
			name: "should return error in case of duplicated validator",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			newValidator: NewECDSAValidator(addr1),
			expectedErr:  ErrValidatorAlreadyExists,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
		},
		{
			name: "should add ECDSA Validator",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			newValidator: NewECDSAValidator(addr2),
			expectedErr:  nil,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
		},
		{
			name: "should add BLS Validator",
			validators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
			),
			newValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:  nil,
			expectedValidators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(
				t,
				test.expectedErr,
				test.validators.Add(test.newValidator),
			)

			assert.Equal(
				t,
				test.expectedValidators,
				test.validators,
			)
		})
	}
}

func TestSetDel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		validators         Validators
		removeValidator    Validator
		expectedErr        error
		expectedValidators Validators
	}{
		{
			name: "should return error in case of type mismatch",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			removeValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:     ErrMismatchValidatorType,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
		},
		{
			name: "should return error in case of non-existing validator",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			removeValidator: NewECDSAValidator(addr2),
			expectedErr:     ErrValidatorNotFound,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
		},
		{
			name: "should remove ECDSA Validator",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			removeValidator: NewECDSAValidator(addr1),
			expectedErr:     nil,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr2),
			),
		},
		{
			name: "should remove BLS Validator",
			validators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
			removeValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:     nil,
			expectedValidators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
			),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(
				t,
				test.expectedErr,
				test.validators.Del(test.removeValidator),
			)

			assert.Equal(
				t,
				test.expectedValidators,
				test.validators,
			)
		})
	}
}

func TestSetMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		validators1        Validators
		validators2        Validators
		expectedErr        error
		expectedValidators Validators
	}{
		{
			name: "should return error in case of type mismatch",
			validators1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			validators2: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
			),
			expectedErr: ErrMismatchValidatorsType,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
		},
		{
			name: "should merge 2 ECDSAValidators",
			validators1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			validators2: NewECDSAValidatorSet(
				NewECDSAValidator(addr2),
			),
			expectedErr: nil,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
		},
		{
			name: "should merge BLS Validator",
			validators1: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
			),
			validators2: NewBLSValidatorSet(
				NewBLSValidator(addr2, testBLSPubKey2),
			),
			expectedErr: nil,
			expectedValidators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
		},
		{
			name: "should merge 2 ECDSAValidators but ignore the validators that already exists in set1",
			validators1: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
			),
			validators2: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
			expectedErr: nil,
			expectedValidators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(
				t,
				test.expectedErr,
				test.validators1.Merge(test.validators2),
			)

			assert.Equal(
				t,
				test.expectedValidators,
				test.validators1,
			)
		})
	}
}

func TestSetMarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		validators Validators
	}{
		{
			name: "ECDSAValidators",
			validators: NewECDSAValidatorSet(
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			),
		},
		{
			name: "BLSValidators",
			validators: NewBLSValidatorSet(
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded := types.MarshalRLPTo(test.validators.MarshalRLPWith, nil)

			validator2 := &Set{
				ValidatorType: test.validators.Type(),
				Validators:    []Validator{},
			}

			assert.NoError(
				t,
				types.UnmarshalRlp(validator2.UnmarshalRLPFrom, encoded),
			)

			assert.Equal(
				t,
				test.validators,
				validator2,
			)
		})
	}
}
