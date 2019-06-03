package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRightPadBytes(t *testing.T) {
	cases := []struct {
		input  []byte
		size   int
		output []byte
	}{
		{
			input:  []byte{0x1, 0x2, 0x3},
			size:   5,
			output: []byte{0x1, 0x2, 0x3, 0x0, 0x0},
		},
		{
			input: []byte{0x1, 0x2},
			size:  2,
		},
		{
			input: []byte{0x1, 0x2},
			size:  1,
		},
	}

	for _, c := range cases {
		output := RightPadBytes(c.input, c.size)

		expected := c.input
		if c.output != nil {
			expected = c.output
		}
		assert.Equal(t, expected, output)
	}
}

func TestLeftPadBytes(t *testing.T) {
	cases := []struct {
		input  []byte
		size   int
		output []byte
	}{
		{
			input:  []byte{0x1, 0x2, 0x3},
			size:   5,
			output: []byte{0x0, 0x0, 0x1, 0x2, 0x3},
		},
		{
			input: []byte{0x1, 0x2},
			size:  2,
		},
		{
			input: []byte{0x1, 0x2},
			size:  1,
		},
	}

	for _, c := range cases {
		output := LeftPadBytes(c.input, c.size)

		expected := c.input
		if c.output != nil {
			expected = c.output
		}
		assert.Equal(t, expected, output)
	}
}
