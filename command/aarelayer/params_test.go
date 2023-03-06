package aarelayer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateFlags_ErrorValidateIPPort(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{}

	assert.ErrorContains(t, p.validateFlags(), "invalid address:")
	p.addr = "%^%:78"
	assert.ErrorContains(t, p.validateFlags(), "invalid address:")
	p.addr = "127.0.0.1:H"
	assert.ErrorContains(t, p.validateFlags(), "invalid address:")
}

func Test_validateFlags_SecretsError(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{
		addr: "127.0.0.1:8289",
	}

	assert.ErrorContains(t, p.validateFlags(), "no config file or data directory passed in")
}

func Test_validateFlags_HappyPath(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{
		addr:       "127.0.0.1:8289",
		configPath: "something",
	}

	assert.NoError(t, p.validateFlags())
}
