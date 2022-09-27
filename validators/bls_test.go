package validators

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestBLSValidatorPublicKeyString(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		hex.EncodeToHex([]byte(testBLSPubKey1)),
		testBLSPubKey1.String(),
	)
}

func TestBLSValidatorPublicKeyMarshal(t *testing.T) {
	t.Parallel()

	res, err := json.Marshal(testBLSPubKey1)

	assert.NoError(t, err)
	assert.Equal(
		t,
		hex.EncodeToHex([]byte(testBLSPubKey1)),
		strings.Trim(
			// remove double quotes in json
			string(res),
			"\"",
		),
	)
}

func TestBLSValidatorPublicKeyUnmarshal(t *testing.T) {
	t.Parallel()

	key := BLSValidatorPublicKey{}

	err := json.Unmarshal(
		[]byte(
			fmt.Sprintf("\"%s\"", hex.EncodeToHex(testBLSPubKey2)),
		),
		&key,
	)

	assert.NoError(t, err)
	assert.Equal(
		t,
		testBLSPubKey2,
		key,
	)
}

func TestNewBLSValidator(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		&BLSValidator{addr1, testBLSPubKey1},
		NewBLSValidator(addr1, testBLSPubKey1),
	)
}

func TestBLSValidatorType(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		BLSValidatorType,
		NewBLSValidator(addr1, testBLSPubKey1).Type(),
	)
}

func TestBLSValidatorString(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	assert.Equal(
		t,
		addr1,
		NewBLSValidator(addr1, testBLSPubKey1).Addr(),
	)
}

func TestBLSValidatorCopy(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

func TestBLSValidatorMarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	val := NewBLSValidator(addr1, testBLSPubKey1)

	// result of Bytes() equals the data encoded in RLP
	assert.Equal(
		t,
		types.MarshalRLPTo(val.MarshalRLPWith, nil),
		val.Bytes(),
	)
}

func TestBLSValidatorFromBytes(t *testing.T) {
	t.Parallel()

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
