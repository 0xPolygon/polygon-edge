package evm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpcodesString(t *testing.T) {
	assert := func(op OpCode, str string) {
		assert.Equal(t, op.String(), str)
	}

	assert(PUSH1, "PUSH1")
	assert(PUSH32, "PUSH32")

	assert(LOG0, "LOG0")
	assert(LOG4, "LOG4")

	assert(SWAP1, "SWAP1")
	assert(SWAP16, "SWAP16")

	assert(DUP1, "DUP1")
	assert(DUP16, "DUP16")

	assert(OpCode(0xA5), "")
}
