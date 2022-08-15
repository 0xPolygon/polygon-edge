package validators

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestNewECDSAValidator(t *testing.T) {
	assert.Equal(
		t,
		&ECDSAValidator{addr1},
		NewECDSAValidator(addr1),
	)
}

func TestECDSAValidatorType(t *testing.T) {
	assert.Equal(
		t,
		ECDSAValidatorType,
		NewECDSAValidator(addr1).Type(),
	)
}

func TestECDSAValidatorString(t *testing.T) {
	assert.Equal(
		t,
		addr1.String(),
		NewECDSAValidator(addr1).String(),
	)
}

func TestECDSAValidatorAddr(t *testing.T) {
	assert.Equal(
		t,
		addr1,
		NewECDSAValidator(addr1).Addr(),
	)
}

func TestECDSAValidatorCopy(t *testing.T) {
	v := NewECDSAValidator(addr1)

	assert.Equal(
		t,
		v,
		v.Copy(),
	)
}

func TestECDSAValidatorEqual(t *testing.T) {
	tests := []struct {
		name     string
		val1     *ECDSAValidator
		val2     *ECDSAValidator
		expected bool
	}{
		{
			name:     "equal",
			val1:     NewECDSAValidator(addr1),
			val2:     NewECDSAValidator(addr1),
			expected: true,
		},
		{
			name:     "not equal",
			val1:     NewECDSAValidator(addr1),
			val2:     NewECDSAValidator(addr2),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				test.val1.Equal(test.val2),
			)
		})
	}
}

func TestECDSAValidatorMarshalAndUnmarshal(t *testing.T) {
	val1 := NewECDSAValidator(addr1)

	marshalRes := types.MarshalRLPTo(val1.MarshalRLPWith, nil)

	val2 := new(ECDSAValidator)

	assert.NoError(
		t,
		types.UnmarshalRlp(val2.UnmarshalRLPFrom, marshalRes),
	)

	assert.Equal(t, val1, val2)
}

func TestECDSAValidatorBytes(t *testing.T) {
	val := NewECDSAValidator(addr1)

	// result of Bytes() equals the data encoded in RLP
	assert.Equal(
		t,
		types.MarshalRLPTo(val.MarshalRLPWith, nil),
		val.Bytes(),
	)
}

func TestECDSAValidatorSetFromBytes(t *testing.T) {
	val1 := NewECDSAValidator(addr1)
	marshalledData := types.MarshalRLPTo(val1.MarshalRLPWith, nil)

	val2 := new(ECDSAValidator)

	// SetFromBytes reads RLP encoded data
	assert.NoError(t, val2.SetFromBytes(marshalledData))

	assert.Equal(
		t,
		val1,
		val2,
	)
}

func TestECDSAValidatorsType(t *testing.T) {
	assert.Equal(
		t,
		ECDSAValidatorType,
		new(ECDSAValidators).Type(),
	)
}

func TestECDSAValidatorsLen(t *testing.T) {
	vals := &ECDSAValidators{
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	}

	assert.Equal(t, 2, vals.Len())
}

func TestECDSAValidatorsEqual(t *testing.T) {
	tests := []struct {
		name     string
		vals1    *ECDSAValidators
		vals2    *ECDSAValidators
		expected bool
	}{
		{
			name: "equal",
			vals1: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
			vals2: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
			expected: true,
		},
		{
			name: "not equal",
			vals1: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
			vals2: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr1),
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				test.vals1.Equal(test.vals2),
			)
		})
	}
}

func TestECDSAValidatorsCopy(t *testing.T) {
	vals1 := &ECDSAValidators{
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	}

	assert.Equal(
		t,
		vals1,
		vals1.Copy(),
	)
}

func TestECDSAValidatorsAt(t *testing.T) {
	valArr := []*ECDSAValidator{
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	}

	vals := ECDSAValidators(valArr)

	for idx := range valArr {
		assert.Equal(
			t,
			valArr[idx],
			vals.At(uint64(idx)),
		)
	}
}

func TestECDSAValidatorsIndex(t *testing.T) {
	tests := []struct {
		name     string
		addr     types.Address
		expected int64
	}{
		{
			name:     "addr1",
			addr:     addr1,
			expected: 0,
		},
		{
			name:     "addr2",
			addr:     addr1,
			expected: 0,
		},
		{
			name:     "not found",
			addr:     types.StringToAddress("fake"),
			expected: -1,
		},
	}

	vals := &ECDSAValidators{
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				vals.Index(test.addr),
			)
		})
	}
}

func TestECDSAValidatorsIncludes(t *testing.T) {
	tests := []struct {
		name     string
		addr     types.Address
		expected bool
	}{
		{
			name:     "addr1",
			addr:     addr1,
			expected: true,
		},
		{
			name:     "addr2",
			addr:     addr1,
			expected: true,
		},
		{
			name:     "not found",
			addr:     types.StringToAddress("fake"),
			expected: false,
		},
	}

	vals := &ECDSAValidators{
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				vals.Includes(test.addr),
			)
		})
	}
}

func TestECDSAValidatorsAdd(t *testing.T) {
	tests := []struct {
		name         string
		newValidator Validator
		expectedErr  error
		expectedRes  *ECDSAValidators
	}{
		{
			name:         "should add new validator",
			newValidator: NewECDSAValidator(addr2),
			expectedErr:  nil,
			expectedRes: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
		},
		{
			name:         "should throw error for wrong typed validator",
			newValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:  ErrMismatchValidatorType,
			expectedRes: &ECDSAValidators{
				NewECDSAValidator(addr1),
			},
		},
		{
			name:         "should throw error for existing validator",
			newValidator: NewECDSAValidator(addr1),
			expectedErr:  ErrValidatorAlreadyExists,
			expectedRes: &ECDSAValidators{
				NewECDSAValidator(addr1),
			},
		},
	}

	for _, test := range tests {
		vals := &ECDSAValidators{
			NewECDSAValidator(addr1),
		}

		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expectedErr,
				vals.Add(test.newValidator),
			)

			assert.Equal(
				t,
				test.expectedRes,
				vals,
			)
		})
	}
}

func TestECDSAValidatorsDel(t *testing.T) {
	tests := []struct {
		name            string
		removeValidator Validator
		expectedErr     error
		expectedRes     *ECDSAValidators
	}{
		{
			name:            "should remove validator",
			removeValidator: NewECDSAValidator(addr1),
			expectedErr:     nil,
			expectedRes: &ECDSAValidators{
				NewECDSAValidator(addr2),
			},
		},
		{
			name:            "should throw error for wrong typed validator",
			removeValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:     ErrMismatchValidatorType,
			expectedRes: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
		},
		{
			name:            "should throw error for non-existing validator",
			removeValidator: NewECDSAValidator(types.StringToAddress("fake")),
			expectedErr:     ErrValidatorNotFound,
			expectedRes: &ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
		},
	}

	for _, test := range tests {
		vals := &ECDSAValidators{
			NewECDSAValidator(addr1),
			NewECDSAValidator(addr2),
		}

		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expectedErr,
				vals.Del(test.removeValidator),
			)

			assert.Equal(
				t,
				test.expectedRes,
				vals,
			)
		})
	}
}

func TestECDSAValidatorsMerge(t *testing.T) {
	t.Run("mismatch types", func(t *testing.T) {
		vals1 := &ECDSAValidators{
			NewECDSAValidator(addr1),
		}
		vals2 := &BLSValidators{
			NewBLSValidator(addr1, testBLSPubKey1),
		}

		assert.ErrorIs(
			t,
			ErrMismatchValidatorSetType,
			vals1.Merge(vals2),
		)
	})

	t.Run("should merge", func(t *testing.T) {
		vals1 := &ECDSAValidators{
			NewECDSAValidator(addr1),
		}
		vals2 := &ECDSAValidators{
			NewECDSAValidator(addr1),
			NewECDSAValidator(addr2),
		}

		assert.NoError(
			t,
			vals1.Merge(vals2),
		)

		assert.Equal(
			t,
			&ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
			vals1,
		)

		assert.Equal(
			t,
			&ECDSAValidators{
				NewECDSAValidator(addr1),
				NewECDSAValidator(addr2),
			},
			vals2,
		)
	})
}

func TestECDSAValidatorsMarshalAndUnmarshal(t *testing.T) {
	vals1 := &ECDSAValidators{
		NewECDSAValidator(addr1),
		NewECDSAValidator(addr2),
	}

	encoded := types.MarshalRLPTo(vals1.MarshalRLPWith, nil)

	vals2 := new(ECDSAValidators)

	assert.NoError(
		t,
		types.UnmarshalRlp(vals2.UnmarshalRLPFrom, encoded),
	)

	assert.Equal(
		t,
		vals1,
		vals2,
	)
}
