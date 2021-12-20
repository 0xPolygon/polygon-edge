package restore

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/types"
)

var (
	// Size of blocks to pass to WriteBlocks
	chunkSize = 10
)

func ImportChain(bc *blockchain.Blockchain, filePath string) error {
	fp, err := os.Open(filePath)
	if err != nil {
		return err
	}

	blockStream := newBlockStream(fp)

	genesis, err := blockStream.nextBlock()
	if err != nil {
		return err
	}
	if genesis.Hash() != bc.Genesis() {
		return fmt.Errorf("the hash of genesis block (%s) in %s does not match blockchain genesis (%s)", genesis.Hash(), filePath, bc.Genesis())
	}

	blocks := make([]*types.Block, chunkSize)
	for {
		blocks = blocks[:0]
		for i := 0; i < chunkSize; i++ {
			block, err := blockStream.nextBlock()
			if err != nil {
				return err
			}
			if block == nil {
				break
			}
			blocks = append(blocks, block)
		}

		if len(blocks) == 0 {
			break
		}
		if err := bc.WriteBlocks(blocks); err != nil {
			return err
		}
	}

	return nil
}

// blockStream parse RLP-encoded block from stream and consumed the used bytes
type blockStream struct {
	input  io.Reader
	buffer []byte
}

func newBlockStream(input io.Reader) *blockStream {
	return &blockStream{
		input:  input,
		buffer: make([]byte, 0, 1024), // impossible to estimate block size but minimum block size is about 900 bytes
	}
}

// nextBlock takes some bytes from input, returns parsed block, and consumes used bytes
func (b *blockStream) nextBlock() (*types.Block, error) {
	prefix, err := b.loadRLPPrefix()
	if errors.Is(io.EOF, err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	payloadSize, payloadSizeSize, err := b.loadPrefixSize(1, prefix)
	if err != nil {
		return nil, err
	}
	if err = b.loadPayload(1+payloadSizeSize, payloadSize); err != nil {
		return nil, err
	}

	return b.parseBlock(1 + payloadSizeSize + payloadSize)
}

// loadRLPPrefix loads first byte of RLP encoded data from input
func (b *blockStream) loadRLPPrefix() (byte, error) {
	buf := b.buffer[:1]
	if _, err := b.input.Read(buf); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// loadPrefixSize loads array's size from input
// basically block should be array in RLP encoded value because block has 3 fields on the top: Header, Transactions, Uncles
func (b *blockStream) loadPrefixSize(offset int64, prefix byte) (int64, int64, error) {
	switch {
	case prefix >= 0xc0 && prefix <= 0xf7:
		// an array whose size is less than 56
		return int64(prefix - 0xc0), 0, nil
	case prefix >= 0xf8:
		// an array whose size is greater than or equal to 56
		// size of the data representing the size of payload
		payloadSizeSize := int64(prefix - 0xf7)

		b.reserveCap(offset + payloadSizeSize)
		payloadSizeBytes := b.buffer[offset : offset+payloadSizeSize]
		_, err := b.input.Read(payloadSizeBytes)
		if err != nil {
			return 0, 0, err
		}
		payloadSize := new(big.Int).SetBytes(payloadSizeBytes).Int64()
		return payloadSize, payloadSizeSize, nil
	}
	return 0, 0, errors.New("expected arrray but got bytes")
}

// loadPayload loads payload data from stream and store to buffer
func (b *blockStream) loadPayload(offset int64, size int64) error {
	b.reserveCap(offset + size)
	buf := b.buffer[offset : offset+size]
	if _, err := b.input.Read(buf); err != nil {
		return err
	}
	return nil
}

// parseBlock parses RLP encoded block in buffer
func (b *blockStream) parseBlock(size int64) (*types.Block, error) {
	data := b.buffer[:size]
	block := &types.Block{}
	if err := block.UnmarshalRLP(data); err != nil {
		return nil, err
	}
	return block, nil
}

// reserveCap makes sure the internal buffer has given size
func (b *blockStream) reserveCap(size int64) {
	if size > int64(cap(b.buffer)) {
		b.buffer = append(b.buffer[:cap(b.buffer)], make([]byte, size)...)
	}
}
