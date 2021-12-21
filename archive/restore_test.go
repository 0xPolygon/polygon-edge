package archive

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
)

var (
	metadata = Metadata{
		Latest:     10,
		LatestHash: types.StringToHash("10"),
	}
)

func Test_parseBlock(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		block       *types.Block
		err         error
	}{
		{
			name:        "should return next block",
			blockstream: newBlockStream(bytes.NewBuffer(blocks[0].MarshalRLP())),
			block:       blocks[0],
			err:         nil,
		},
		{
			name:        "should return error",
			blockstream: newBlockStream(bytes.NewBuffer((&Metadata{}).MarshalRLP())),
			block:       nil,
			// should fail by wrong format
			err: errors.New("not enough elements to decode block, expected 3 but found 2"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := tt.blockstream.nextBlock()
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.block, block)
		})
	}
}

func Test_parseMetadata(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		metadata    *Metadata
		err         error
	}{
		{
			name:        "should return Metadata",
			blockstream: newBlockStream(bytes.NewBuffer(metadata.MarshalRLP())),
			metadata:    &metadata,
			err:         nil,
		},
		{
			name:        "should return error",
			blockstream: newBlockStream(bytes.NewBuffer(blocks[0].MarshalRLP())),
			metadata:    nil,
			err:         errors.New("not enough elements to decode Metadata, expected 2 but found 3"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := tt.blockstream.getMetadata()
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.metadata, metadata)
		})
	}
}

func Test_loadRLPPrefix(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		data        byte
		err         error
	}{
		{
			name:        "should return first byte",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{0x1})),
			data:        0x1,
			err:         nil,
		},
		{
			name:        "should return EOF",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{})),
			data:        0,
			err:         io.EOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.blockstream.loadRLPPrefix()
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.data, data)
		})
	}
}

func Test_loadPrefixSize(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		prefix      byte
		size        uint64
		consumed    uint64
		err         error
	}{
		{
			name:        "should return size when prefix is less than 0xf8",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{})),
			prefix:      0xc5,
			size:        5,
			consumed:    0,
			err:         nil,
		},
		{
			name:        "should return size when prefix is greater than 0xf7",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{0x1, 0x00})),
			prefix:      0xf9, // 2 bytes
			size:        0x100,
			consumed:    2,
			err:         nil,
		},
		{
			name:        "should return EOF if can't any data",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{})),
			prefix:      0xf9, // 2 bytes
			size:        0,
			consumed:    0,
			err:         io.EOF,
		},
		{
			name:        "should return EOF if can't load required amount of bytes",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{0x01})),
			prefix:      0xf9, // 2 bytes
			size:        0,
			consumed:    0,
			err:         io.EOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, consumed, err := tt.blockstream.loadPrefixSize(0, tt.prefix)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.size, size)
			assert.Equal(t, tt.consumed, consumed)
		})
	}
}

func Test_loadPayload(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		offset      uint64
		size        uint64
		err         error
		newBuffer   []byte
	}{
		{
			name:        "should load data",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{0x1, 0x2})),
			offset:      0,
			size:        2,
			err:         nil,
			newBuffer:   []byte{0x1, 0x2},
		},
		{
			name: "should load data with offset",
			blockstream: &blockStream{
				input:  bytes.NewBuffer([]byte{0x3, 0x4}),
				buffer: []byte{0x1, 0x2},
			},
			offset:    2,
			size:      2,
			err:       nil,
			newBuffer: []byte{0x1, 0x2, 0x3, 0x4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.blockstream.loadPayload(tt.offset, tt.size)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.newBuffer, tt.blockstream.buffer)
		})
	}
}

func Test_parrseMetadata(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		size        uint64
		metadata    *Metadata
		err         error
	}{
		{
			name: "should return Metadata",
			blockstream: &blockStream{
				input:  bytes.NewBuffer([]byte{}),
				buffer: metadata.MarshalRLP(),
			},
			size:     uint64(len(metadata.MarshalRLP())),
			metadata: &metadata,
			err:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := tt.blockstream.parseMetadata(tt.size)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.metadata, metadata)
		})
	}
}

func Test_parrseBlock(t *testing.T) {
	tests := []struct {
		name        string
		blockstream *blockStream
		size        uint64
		block       *types.Block
		err         error
	}{
		{
			name: "should return block",
			blockstream: &blockStream{
				input:  bytes.NewBuffer([]byte{}),
				buffer: blocks[0].MarshalRLP(),
			},
			size:  uint64(len(blocks[0].MarshalRLP())),
			block: blocks[0],
			err:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := tt.blockstream.parseBlock(tt.size)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.block, block)
		})
	}
}

func Test_reserveCap(t *testing.T) {
	tests := []struct {
		name         string
		blockstream  *blockStream
		expectedSize uint64
		newLen       int
		newCap       int
	}{
		{
			name: "should expand and change size",
			blockstream: &blockStream{
				buffer: make([]byte, 0, 10),
			},
			expectedSize: 20,
			newLen:       20,
			newCap:       32,
		},
		{
			name: "should change size",
			blockstream: &blockStream{
				buffer: make([]byte, 0, 30),
			},
			expectedSize: 20,
			newLen:       20,
			newCap:       30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.blockstream.reserveCap(tt.expectedSize)
			assert.Equal(t, tt.newLen, len(tt.blockstream.buffer))
			assert.Equal(t, tt.newCap, cap(tt.blockstream.buffer))
		})
	}
}
