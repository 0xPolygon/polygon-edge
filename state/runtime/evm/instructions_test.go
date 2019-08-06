package evm

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	two = big.NewInt(2)
)

type cases2To1 []struct {
	a *big.Int
	b *big.Int
	c *big.Int
}

func test2to1(t *testing.T, f instruction, tests cases2To1) {
	s, close := getState()
	defer close()

	for _, i := range tests {
		s.push(i.a)
		s.push(i.b)

		f(s)

		assert.Equal(t, i.c, s.pop())
	}
}

type cases2ToBool []struct {
	a *big.Int
	b *big.Int
	c bool
}

func test2toBool(t *testing.T, f instruction, tests cases2ToBool) {
	s, close := getState()
	defer close()

	for _, i := range tests {
		s.push(i.a)
		s.push(i.b)

		f(s)

		if i.c {
			assert.Equal(t, uint64(1), s.pop().Uint64())
		} else {
			assert.Equal(t, uint64(0), s.pop().Uint64())
		}
	}
}

func TestAdd(t *testing.T) {
	test2to1(t, opAdd, cases2To1{
		{one, one, two},
		{zero, one, one},
	})
}

func TestGt(t *testing.T) {
	test2toBool(t, opGt, cases2ToBool{
		{one, one, false},
		{two, one, false},
		{one, two, true},
	})
}

func TestIsZero(t *testing.T) {
	test2toBool(t, opIsZero, cases2ToBool{
		{one, one, false},
		{zero, zero, true},
		{two, two, false},
	})
}

func TestMStore(t *testing.T) {
	s, close := getState()
	defer close()

	s.push(big.NewInt(10))   // value
	s.push(big.NewInt(1024)) // offset

	s.gas = 1000
	opMStore(s)

	assert.Len(t, s.memory, 1024+32)
}
