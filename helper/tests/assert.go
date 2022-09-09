package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertErrorMessageContains is a helper function to make sure
// an error message matches with the expected error which is not exported
// if expected error is nil, it checks the target error is nil
func AssertErrorMessageContains(t *testing.T, expected, target error) {
	t.Helper()

	if expected != nil {
		assert.ErrorContains(t, target, expected.Error())
	} else {
		assert.NoError(t, target)
	}
}
