package aarelayer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateFlags_ValidateIPPort(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{}

	assert.ErrorContains(t, p.validateFlags(), "invalid address:")

	p.addr = "%^%:78"
	assert.ErrorContains(t, p.validateFlags(), "invalid address:")

	p.addr = "127.0.0.1:H"
	assert.ErrorContains(t, p.validateFlags(), "invalid address:")

	p.addr = "127.0.0.1:8289"
	assert.NoError(t, p.validateFlags())
}
