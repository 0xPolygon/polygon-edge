package evm

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPushOpcodes(t *testing.T) {
	code := make([]byte, 33)
	for i := 0; i < 33; i++ {
		code[i] = byte(i + 1)
	}

	c := 1

	for i := PUSH1; i <= PUSH32; i++ {
		s := &state{
			code: code,
		}

		inst := dispatchTable[i]
		inst.inst(s)

		assert.False(t, s.stop)

		res := s.pop().Bytes()
		assert.Len(t, res, c)

		assert.True(t, bytes.HasPrefix(code[1:], res))
		c++
	}
}
