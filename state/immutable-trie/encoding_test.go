package itrie

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEncoding_HasTermSymbol(t *testing.T) {
	testTable := []struct {
		name           string
		value          []byte
		shouldHaveTerm bool
	}{
		{
			"nil value",
			nil,
			false,
		},
		{
			"empty value",
			[]byte{},
			false,
		},
		{
			"empty value with terminating symbol",
			[]byte{16},
			true,
		},
		{
			"without terminating symbol",
			[]byte{1, 2, 3, 4, 5},
			false,
		},
		{
			"with terminating symbol",
			[]byte{1, 2, 3, 4, 5, 16},
			true,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.shouldHaveTerm, hasTerm(testCase.value))
		})
	}
}

func TestEncoding_NibblesToBytes(t *testing.T) {
	testTable := []struct {
		name          string
		nibbles       []byte
		bytes         []byte
		expectedBytes []byte
		shouldFail    bool
	}{
		{
			"Valid case #1",
			[]byte{0xa, 0x1}, //	1010 0001 -> 10100001
			make([]byte, 1),
			[]byte{0xa1},
			false,
		},
		{
			"Valid case #2",
			[]byte{0xf, 0x7}, // 1111 0111 -> 11110111
			make([]byte, 1),
			[]byte{0xf7},
			false,
		},
		{
			"Valid case #3",
			[]byte{0x0, 0x0}, // 0000 0000 -> 00000000
			make([]byte, 1),
			[]byte{0x0},
			false,
		},
		{
			"Valid case #4",
			[]byte{0x0, 0x1}, // 0000 0001 -> 00000001
			make([]byte, 1),
			[]byte{0x1},
			false,
		},
		{
			"Valid case #6",
			[]byte{0xa, 0xb, 0xc, 0xd}, // 1010 1011 1100 1101 -> 10101011 11001101
			make([]byte, 2),
			[]byte{0xab, 0xcd},
			false,
		},
		{
			"Invalid case #7 - odd nibbles",
			[]byte{0xa, 0xb, 0xc}, // this shouldn't be an issue?
			make([]byte, 2),
			nil,
			true,
		},
		{
			"Invalid case #8 - invalid byte result size",
			[]byte{0xa, 0xb, 0xc, 0xd, 0xe, 0xf},
			make([]byte, 2), // should have space for 3 bytes
			nil,
			true,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			defer func() {
				// TODO these errors should be handled properly by decodeNibbles
				if r := recover(); r != nil {
					if testCase.shouldFail {
						t.Fatalf("Unimplemented error handling")
					}

					t.Fatalf("Unexpected panic occurred")
				}
			}()

			decodeNibbles(testCase.nibbles, testCase.bytes)

			assert.Equal(t, testCase.expectedBytes, testCase.bytes)
		})
	}
}

func TestEncoding_KeyBytesToHexNibbles(t *testing.T) {
	termSignal := byte(0x10) // 16

	testTable := []struct {
		name           string
		inputString    []byte
		expectedOutput []byte
	}{
		{
			"Valid case #1",
			[]byte{0xa},
			[]byte{
				0x0, 0xa,
				termSignal,
			},
		},
		{
			"Valid case #2",
			[]byte{0xa, 0xb, 0xc},
			[]byte{
				0x0, 0xa,
				0x0, 0xb,
				0x0, 0xc,
				termSignal,
			},
		},
		{
			"Valid case #3",
			[]byte("Polygon"),
			[]byte{
				0x5, 0x0, // P -> 85 	-> 0x50
				0x6, 0xf, // o -> 111 	-> 0x6f
				0x6, 0xc, // l -> 108 	-> 0x6c
				0x7, 0x9, // y -> 121 	-> 0x79
				0x6, 0x7, // g -> 103 	-> 0x67
				0x6, 0xf, // o -> 111 	-> 0x6f
				0x6, 0xe, // n -> 110 	-> 0x6e
				termSignal,
			},
		},
		{
			"Valid case #4",
			[]byte{},
			[]byte{termSignal},
		},
		{
			"Valid case #5",
			nil,
			[]byte{termSignal},
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			output := keybytesToHex(testCase.inputString)

			assert.Len(t, output, len(testCase.expectedOutput))
			assert.Equal(t, testCase.expectedOutput, output)
		})
	}
}
