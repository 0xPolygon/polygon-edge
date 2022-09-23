package fork

import (
	"errors"
	"testing"

	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/stretchr/testify/assert"
)

func TestIBFTTypeString(t *testing.T) {
	t.Parallel()

	cases := map[IBFTType]string{
		PoA: "PoA",
		PoS: "PoS",
	}

	for typ, expected := range cases {
		assert.Equal(
			t,
			expected,
			typ.String(),
		)
	}
}

func TestParseIBFTType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		res   IBFTType
		err   error
	}{
		{
			value: "PoA",
			res:   PoA,
			err:   nil,
		},
		{
			value: "PoS",
			res:   PoS,
			err:   nil,
		},
		{
			value: "hoge",
			res:   IBFTType(""),
			err:   errors.New("invalid IBFT type hoge"),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.value, func(t *testing.T) {
			t.Parallel()

			res, err := ParseIBFTType(test.value)

			assert.Equal(
				t,
				test.res,
				res,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.err,
				err,
			)
		})
	}
}
