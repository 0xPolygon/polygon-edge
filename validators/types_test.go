package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseValidatorType(t *testing.T) {
	t.Parallel()

	t.Run("ECDSA", func(t *testing.T) {
		t.Parallel()

		res, err := ParseValidatorType("ecdsa")

		assert.Equal(
			t,
			ECDSAValidatorType,
			res,
		)

		assert.NoError(
			t,
			err,
		)
	})

	t.Run("BLS", func(t *testing.T) {
		t.Parallel()

		res, err := ParseValidatorType("bls")

		assert.Equal(
			t,
			BLSValidatorType,
			res,
		)

		assert.NoError(
			t,
			err,
		)
	})

	t.Run("other type", func(t *testing.T) {
		t.Parallel()

		_, err := ParseValidatorType("fake")

		assert.Equal(
			t,
			ErrInvalidValidatorType,
			err,
		)
	})
}
