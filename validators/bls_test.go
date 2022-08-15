package validators

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestNewBLSValidator(t *testing.T) {
	assert.Equal(
		t,
		&BLSValidator{addr1, testBLSPubKey1},
		NewBLSValidator(addr1, testBLSPubKey1),
	)
}

func TestBLSValidatorType(t *testing.T) {
	assert.Equal(
		t,
		BLSValidatorType,
		NewBLSValidator(addr1, testBLSPubKey1).Type(),
	)
}

func TestBLSValidatorString(t *testing.T) {
	assert.Equal(
		t,
		fmt.Sprintf(
			"%s:%s",
			addr1.String(),
			"0x"+hex.EncodeToString(testBLSPubKey1),
		),
		NewBLSValidator(addr1, testBLSPubKey1).String(),
	)
}

func TestBLSValidatorAddr(t *testing.T) {
	assert.Equal(
		t,
		addr1,
		NewBLSValidator(addr1, testBLSPubKey1).Addr(),
	)
}

func TestBLSValidatorCopy(t *testing.T) {
	v1 := NewBLSValidator(addr1, testBLSPubKey1)
	v2 := v1.Copy()

	assert.Equal(t, v1, v2)

	// check the addresses are different
	typedV2, ok := v2.(*BLSValidator)

	assert.True(t, ok)
	assert.NotSame(t, v1.Address, typedV2.Address)
	assert.NotSame(t, v1.BLSPublicKey, typedV2.BLSPublicKey)
}

func TestBLSValidatorEqual(t *testing.T) {
	tests := []struct {
		name     string
		val1     *BLSValidator
		val2     *BLSValidator
		expected bool
	}{
		{
			name:     "equal",
			val1:     NewBLSValidator(addr1, testBLSPubKey1),
			val2:     NewBLSValidator(addr1, testBLSPubKey1),
			expected: true,
		},
		{
			name:     "addr does not equal",
			val1:     NewBLSValidator(addr1, testBLSPubKey1),
			val2:     NewBLSValidator(addr2, testBLSPubKey1),
			expected: false,
		},
		{
			name:     "public key does not equal",
			val1:     NewBLSValidator(addr1, testBLSPubKey1),
			val2:     NewBLSValidator(addr1, testBLSPubKey2),
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

func TestBLSValidatorMarshalAndUnmarshal(t *testing.T) {
	val1 := NewBLSValidator(addr1, testBLSPubKey1)

	marshalRes := types.MarshalRLPTo(val1.MarshalRLPWith, nil)

	val2 := new(BLSValidator)

	assert.NoError(
		t,
		types.UnmarshalRlp(val2.UnmarshalRLPFrom, marshalRes),
	)

	assert.Equal(t, val1, val2)
}

func TestBLSValidatorBytes(t *testing.T) {
	val := NewBLSValidator(addr1, testBLSPubKey1)

	// result of Bytes() equals the data encoded in RLP
	assert.Equal(
		t,
		types.MarshalRLPTo(val.MarshalRLPWith, nil),
		val.Bytes(),
	)
}

func TestBLSValidatorSetFromBytes(t *testing.T) {
	val1 := NewBLSValidator(addr1, testBLSPubKey1)
	marshalledData := types.MarshalRLPTo(val1.MarshalRLPWith, nil)

	val2 := new(BLSValidator)

	// SetFromBytes reads RLP encoded data
	assert.NoError(t, val2.SetFromBytes(marshalledData))

	assert.Equal(
		t,
		val1,
		val2,
	)
}

func TestBLSValidatorsType(t *testing.T) {
	assert.Equal(
		t,
		BLSValidatorType,
		new(BLSValidator).Type(),
	)
}

func TestBLSValidatorsLen(t *testing.T) {
	vals := &BLSValidators{
		NewBLSValidator(addr1, testBLSPubKey1),
		NewBLSValidator(addr2, testBLSPubKey2),
	}

	assert.Equal(t, 2, vals.Len())
}

func TestBLSValidatorsEqual(t *testing.T) {
	tests := []struct {
		name     string
		vals1    *BLSValidators
		vals2    *BLSValidators
		expected bool
	}{
		{
			name: "equal",
			vals1: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
			vals2: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
			expected: true,
		},
		{
			name: "not equal",
			vals1: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
			vals2: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey1),
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

func TestBLSValidatorsCopy(t *testing.T) {
	vals1 := &BLSValidators{
		NewBLSValidator(addr1, testBLSPubKey1),
		NewBLSValidator(addr2, testBLSPubKey2),
	}

	vals2 := vals1.Copy()

	assert.Equal(t, vals1, vals2)

	// check the addresses are different
	for i := 0; i < vals1.Len(); i++ {
		assert.NotSame(
			t,
			vals1.At(uint64(i)),
			vals2.At(uint64(i)),
		)
	}
}

func TestBLSValidatorsAt(t *testing.T) {
	valArr := []*BLSValidator{
		NewBLSValidator(addr1, testBLSPubKey1),
		NewBLSValidator(addr2, testBLSPubKey2),
	}

	vals := BLSValidators(valArr)

	for idx := range valArr {
		assert.Equal(
			t,
			valArr[idx],
			vals.At(uint64(idx)),
		)
	}
}

func TestBLSValidatorsIndex(t *testing.T) {
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

	vals := &BLSValidators{
		NewBLSValidator(addr1, testBLSPubKey1),
		NewBLSValidator(addr2, testBLSPubKey1),
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

func TestBLSValidatorsIncludes(t *testing.T) {
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

	vals := &BLSValidators{
		NewBLSValidator(addr1, testBLSPubKey1),
		NewBLSValidator(addr2, testBLSPubKey1),
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

func TestBLSValidatorsAdd(t *testing.T) {
	tests := []struct {
		name         string
		newValidator Validator
		expectedErr  error
		expectedRes  *BLSValidators
	}{
		{
			name:         "should add new validator",
			newValidator: NewBLSValidator(addr2, testBLSPubKey2),
			expectedErr:  nil,
			expectedRes: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
		},
		{
			name:         "should throw error for wrong typed validator",
			newValidator: NewECDSAValidator(addr2),
			expectedErr:  ErrMismatchValidatorType,
			expectedRes: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
			},
		},
		{
			name:         "should throw error for existing validator",
			newValidator: NewBLSValidator(addr1, testBLSPubKey1),
			expectedErr:  ErrValidatorAlreadyExists,
			expectedRes: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
			},
		},
	}

	for _, test := range tests {
		vals := &BLSValidators{
			NewBLSValidator(addr1, testBLSPubKey1),
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

func TestBLSValidatorsRemove(t *testing.T) {
	tests := []struct {
		name            string
		removeValidator Validator
		expectedErr     error
		expectedRes     *BLSValidators
	}{
		{
			name:            "should remove new validator",
			removeValidator: NewBLSValidator(addr1, testBLSPubKey1),
			expectedErr:     nil,
			expectedRes: &BLSValidators{
				NewBLSValidator(addr2, testBLSPubKey2),
			},
		},
		{
			name:            "should throw error for wrong typed validator",
			removeValidator: NewECDSAValidator(addr2),
			expectedErr:     ErrMismatchValidatorType,
			expectedRes: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
		},
		{
			name: "should throw error for non-existing validator",
			removeValidator: NewBLSValidator(
				types.StringToAddress("fake"),
				[]byte("fake"),
			),
			expectedErr: ErrValidatorNotFound,
			expectedRes: &BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
		},
	}

	for _, test := range tests {
		vals := &BLSValidators{
			NewBLSValidator(addr1, testBLSPubKey1),
			NewBLSValidator(addr2, testBLSPubKey2),
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

func TestBLSValidatorsMerge(t *testing.T) {
	t.Run("mismatch types", func(t *testing.T) {
		vals1 := &BLSValidators{
			NewBLSValidator(addr1, testBLSPubKey1),
		}
		vals2 := &ECDSAValidators{
			NewECDSAValidator(addr1),
		}

		assert.ErrorIs(
			t,
			ErrMismatchValidatorSetType,
			vals1.Merge(vals2),
		)
	})

	t.Run("should merge", func(t *testing.T) {
		vals1 := &BLSValidators{
			NewBLSValidator(addr1, testBLSPubKey1),
		}
		vals2 := &BLSValidators{
			NewBLSValidator(addr1, testBLSPubKey1),
			NewBLSValidator(addr2, testBLSPubKey2),
		}

		assert.NoError(
			t,
			vals1.Merge(vals2),
		)

		assert.Equal(
			t,
			&BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
			vals1,
		)

		assert.Equal(
			t,
			&BLSValidators{
				NewBLSValidator(addr1, testBLSPubKey1),
				NewBLSValidator(addr2, testBLSPubKey2),
			},
			vals2,
		)
	})
}

func TestBLSValidatorsMarshalAndUnmarshal(t *testing.T) {
	vals1 := &BLSValidators{
		NewBLSValidator(addr1, testBLSPubKey1),
		NewBLSValidator(addr2, testBLSPubKey2),
	}

	encoded := types.MarshalRLPTo(vals1.MarshalRLPWith, nil)

	vals2 := new(BLSValidators)

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
