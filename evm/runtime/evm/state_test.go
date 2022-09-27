package evm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type codeHelper struct {
	buf []byte
}

func (c *codeHelper) Code() []byte {
	return c.buf
}

func (c *codeHelper) push1() {
	c.buf = append(c.buf, PUSH1)
	c.buf = append(c.buf, 0x1)
}

func (c *codeHelper) pop() {
	c.buf = append(c.buf, POP)
}

func getState() (*state, func()) {
	c := statePool.Get().(*state) //nolint:forcetypeassert

	return c, func() {
		c.reset()
		statePool.Put(c)
	}
}

func TestStackTop(t *testing.T) {
	s, closeFn := getState()
	defer closeFn()

	s.push(one)
	s.push(two)

	assert.Equal(t, two, s.top())
	assert.Equal(t, s.stackSize(), 2)
}

func TestStackOverflow(t *testing.T) {
	code := codeHelper{}
	for i := 0; i < stackSize; i++ {
		code.push1()
	}

	s, closeFn := getState()
	defer closeFn()

	s.code = code.buf
	s.gas = 10000

	_, err := s.Run()
	assert.NoError(t, err)

	// add one more item to the stack
	code.push1()

	s.reset()
	s.code = code.buf
	s.gas = 10000

	_, err = s.Run()
	assert.Equal(t, errStackOverflow, err)
}

func TestStackUnderflow(t *testing.T) {
	s, closeFn := getState()
	defer closeFn()

	code := codeHelper{}
	for i := 0; i < 10; i++ {
		code.push1()
	}

	for i := 0; i < 10; i++ {
		code.pop()
	}

	s.code = code.buf
	s.gas = 10000

	_, err := s.Run()
	assert.NoError(t, err)

	code.pop()

	s.reset()
	s.code = code.buf
	s.gas = 10000

	_, err = s.Run()
	assert.Equal(t, errStackUnderflow, err)
}

func TestOpcodeNotFound(t *testing.T) {
	s, closeFn := getState()
	defer closeFn()

	s.code = []byte{0xA5}
	s.gas = 1000

	_, err := s.Run()
	assert.Equal(t, errOpCodeNotFound, err)
}
