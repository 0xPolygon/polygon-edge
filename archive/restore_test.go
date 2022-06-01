package archive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/protocol"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	metadata = Metadata{
		Latest:     10,
		LatestHash: types.StringToHash("10"),
	}
)

type mockChain struct {
	genesis *types.Block
	blocks  []*types.Block
}

func (m *mockChain) Genesis() types.Hash {
	return m.genesis.Hash()
}

func (m *mockChain) GetBlockByNumber(num uint64, full bool) (*types.Block, bool) {
	for _, b := range m.blocks {
		if b.Number() == num {
			return b, true
		}
	}

	return nil, false
}

func (m *mockChain) GetHashByNumber(num uint64) types.Hash {
	b, ok := m.GetBlockByNumber(num, false)
	if !ok {
		return types.Hash{}
	}

	return b.Hash()
}

func (m *mockChain) WriteBlock(block *types.Block) error {
	m.blocks = append(m.blocks, block)

	return nil
}

func (m *mockChain) VerifyFinalizedBlock(block *types.Block) error {
	return nil
}

func (m *mockChain) SubscribeEvents() blockchain.Subscription {
	return protocol.NewMockSubscription()
}

func getLatestBlockFromMockChain(m *mockChain) *types.Block {
	if l := len(m.blocks); l != 0 {
		return m.blocks[l-1]
	}

	return nil
}

func Test_importBlocks(t *testing.T) {
	newTestBlockStream := func(metadata *Metadata, blocks ...*types.Block) *blockStream {
		var buf bytes.Buffer

		buf.Write(metadata.MarshalRLP())

		for _, b := range blocks {
			buf.Write(b.MarshalRLP())
		}

		return newBlockStream(&buf)
	}

	tests := []struct {
		name          string
		metadata      *Metadata
		archiveBlocks []*types.Block
		chain         *mockChain
		// result
		err         error
		latestBlock *types.Block
	}{
		{
			name: "should write all blocks",
			metadata: &Metadata{
				Latest:     blocks[2].Number(),
				LatestHash: blocks[2].Hash(),
			},
			archiveBlocks: []*types.Block{
				genesis, blocks[0], blocks[1], blocks[2],
			},
			chain: &mockChain{
				genesis: genesis,
				blocks:  []*types.Block{},
			},
			err:         nil,
			latestBlock: blocks[2],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progression := progress.NewProgressionWrapper(progress.ChainSyncRestore)
			blockStream := newTestBlockStream(tt.metadata, tt.archiveBlocks...)
			err := importBlocks(tt.chain, blockStream, progression)

			assert.Equal(t, tt.err, err)
			latestBlock := getLatestBlockFromMockChain(tt.chain)
			assert.Equal(t, tt.latestBlock, latestBlock)
		})
	}
}

func Test_consumeCommonBlocks(t *testing.T) {
	newTestArchiveStream := func(blocks ...*types.Block) *blockStream {
		var buf bytes.Buffer
		for _, b := range blocks {
			buf.Write(b.MarshalRLP())
		}

		return newBlockStream(&buf)
	}

	tests := []struct {
		name        string
		blockStream *blockStream
		chain       blockchainInterface
		// result
		block *types.Block
		err   error
	}{
		{
			name:        "should consume common blocks",
			blockStream: newTestArchiveStream(genesis, blocks[0], blocks[1], blocks[2]),
			chain: &mockChain{
				genesis: genesis,
				blocks:  []*types.Block{blocks[0], blocks[1]},
			},
			block: blocks[2],
			err:   nil,
		},
		{
			name:        "should consume all blocks",
			blockStream: newTestArchiveStream(genesis, blocks[0], blocks[1]),
			chain: &mockChain{
				genesis: genesis,
				blocks:  []*types.Block{blocks[0], blocks[1]},
			},
			block: nil,
			err:   nil,
		},
		{
			name:        "should return error in case of genesis mismatch",
			blockStream: newTestArchiveStream(genesis, blocks[0], blocks[1]),
			chain: &mockChain{
				genesis: &types.Block{
					Header: &types.Header{
						Hash:   types.StringToHash("wrong genesis"),
						Number: 0,
					},
				},
				blocks: []*types.Block{blocks[0], blocks[1]},
			},
			block: nil,
			err: fmt.Errorf(
				"the hash of genesis block (%s) does not match blockchain genesis (%s)",
				genesis.Hash(),
				types.StringToHash("wrong genesis"),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			osSignal := make(<-chan os.Signal)
			resultBlock, err := consumeCommonBlocks(tt.chain, tt.blockStream, osSignal)

			assert.Equal(t, tt.block, resultBlock)
			assert.Equal(t, tt.err, err)
		})
	}
}

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
			err: errors.New("incorrect number of elements to decode block, expected 3 but found 2"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := tt.blockstream.nextBlock()

			assert.Equal(t, tt.block, block)
			assert.Equal(t, tt.err, err)
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
			consumed:    1,
			err:         nil,
		},
		{
			name:        "should return size when prefix is greater than 0xf7",
			blockstream: newBlockStream(bytes.NewBuffer([]byte{0x1, 0x00})),
			prefix:      0xf9, // 2 bytes
			size:        0x100,
			consumed:    3,
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
			headerSize, payloadSize, err := tt.blockstream.loadPrefixSize(0, tt.prefix)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.consumed, headerSize)
			assert.Equal(t, tt.size, payloadSize)
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
	}{
		{
			name: "should expand and change size",
			blockstream: &blockStream{
				buffer: make([]byte, 0, 10),
			},
			expectedSize: 20,
			newLen:       20,
		},
		{
			name: "should change size",
			blockstream: &blockStream{
				buffer: make([]byte, 0, 30),
			},
			expectedSize: 20,
			newLen:       20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.blockstream.reserveCap(tt.expectedSize)
			assert.Equal(t, tt.newLen, len(tt.blockstream.buffer))
			assert.Greater(t, cap(tt.blockstream.buffer), tt.newLen)
		})
	}
}
