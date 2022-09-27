package validators

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestNewECDSAValidator(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		&ECDSAValidator{addr1},
		NewECDSAValidator(addr1),
	)
}

func TestECDSAValidatorType(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		ECDSAValidatorType,
		NewECDSAValidator(addr1).Type(),
	)
}

func TestECDSAValidatorString(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		addr1.String(),
		NewECDSAValidator(addr1).String(),
	)
}

func TestECDSAValidatorAddr(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		addr1,
		NewECDSAValidator(addr1).Addr(),
	)
}

func TestECDSAValidatorCopy(t *testing.T) {
	t.Parallel()

	v1 := NewECDSAValidator(addr1)

	v2 := v1.Copy()

	assert.Equal(t, v1, v2)

	// check the addresses are different
	typedV2, ok := v2.(*ECDSAValidator)

	assert.True(t, ok)
	assert.NotSame(t, v1.Address, typedV2.Address)
}

func TestECDSAValidatorEqual(t *testing.T) {
	t.Parallel()

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
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.val1.Equal(test.val2),
			)
		})
	}
}

func TestECDSAValidatorMarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	val := NewECDSAValidator(addr1)

	// result of Bytes() equals the data encoded in RLP
	assert.Equal(
		t,
		val.Address.Bytes(),
		val.Bytes(),
	)
}

func TestECDSAValidatorFromBytes(t *testing.T) {
	t.Parallel()

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
