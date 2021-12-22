package archive

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/helper/progress"
	"github.com/0xPolygon/polygon-sdk/types"
)

var (
	// Size of blocks to pass to WriteBlocks
	chunkSize = 50
)

type blockchainInterface interface {
	SubscribeEvents() blockchain.Subscription
	Genesis() types.Hash
	GetBlockByNumber(uint64, bool) (*types.Block, bool)
	GetHashByNumber(uint64) types.Hash
	WriteBlocks([]*types.Block) error
}

// RestoreChain reads blocks from the archive and write to the chain
func RestoreChain(chain blockchainInterface, filePath string, progression *progress.ProgressionWrapper) error {
	fp, err := os.Open(filePath)
	if err != nil {
		return err
	}
	blockStream := newBlockStream(fp)

	if _, _, err = importBlocks(chain, blockStream, progression); err != nil {
		return err
	}

	return nil
}

// import blocks scans all blocks from stream and write them to chain
func importBlocks(chain blockchainInterface, blockStream *blockStream, progression *progress.ProgressionWrapper) (uint64, uint64, error) {
	shutdownCh := common.GetTerminationSignalCh()

	metadata, err := blockStream.getMetadata()
	if err != nil {
		return 0, 0, err
	}
	if metadata == nil {
		return 0, 0, errors.New("expected metadata in archive but doesn't exist")
	}

	// check whether the local chain has the latest block already
	latestBlock, ok := chain.GetBlockByNumber(metadata.Latest, false)
	if ok && latestBlock.Hash() == metadata.LatestHash {
		return 0, 0, nil
	}

	// skip existing blocks
	firstBlock, err := consumeCommonBlocks(chain, blockStream, shutdownCh)
	if err != nil {
		return 0, 0, err
	}
	if firstBlock == nil {
		return 0, 0, nil
	}

	// Create a blockchain subscription for the sync progression and start tracking
	progression.StartProgression(firstBlock.Number(), chain.SubscribeEvents())
	// Set the goal
	progression.UpdateHighestProgression(metadata.Latest)
	// Stop monitoring the sync progression upon exit
	defer progression.StopProgression()

	blocks := make([]*types.Block, 0, chunkSize)
	blocks = append(blocks, firstBlock)
	var lastBlockNumber uint64
processLoop:
	for {
		for len(blocks) < chunkSize {
			block, err := blockStream.nextBlock()
			if err != nil {
				return 0, 0, err
			}
			if block == nil {
				break
			}
			blocks = append(blocks, block)
		}

		// no blocks to be written any more
		if len(blocks) == 0 {
			break
		}
		if err := chain.WriteBlocks(blocks); err != nil {
			return firstBlock.Number(), lastBlockNumber, err
		}
		lastBlockNumber = blocks[len(blocks)-1].Number()
		// clear slice but keep capacity
		blocks = blocks[:0]

		select {
		case <-shutdownCh:
			break processLoop
		default:
		}
	}

	return firstBlock.Number(), lastBlockNumber, nil
}

// consumeCommonBlocks consumes blocks in blockstream to latest block in chain or different hash
// returns the first block to be written into chain
func consumeCommonBlocks(chain blockchainInterface, blockStream *blockStream, shutdownCh <-chan os.Signal) (*types.Block, error) {
	for {
		block, err := blockStream.nextBlock()
		if err != nil {
			return nil, err
		}
		if block == nil {
			return nil, nil
		}
		if block.Number() == 0 {
			if block.Hash() != chain.Genesis() {
				return nil, fmt.Errorf("the hash of genesis block (%s) does not match blockchain genesis (%s)", block.Hash(), chain.Genesis())
			}
			continue
		}
		if hash := chain.GetHashByNumber(block.Number()); hash != block.Hash() {
			return block, nil
		}

		select {
		case <-shutdownCh:
			return nil, nil
		default:
		}
	}
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

// getMetadata consumes some bytes from input and returns parsed Metadata
func (b *blockStream) getMetadata() (*Metadata, error) {
	size, err := b.loadRLPArray()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	return b.parseMetadata(size)
}

// nextBlock consumes some bytes from input and returns parsed block
func (b *blockStream) nextBlock() (*types.Block, error) {
	size, err := b.loadRLPArray()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	return b.parseBlock(size)
}

// loadRLPArray loads RLP encoded array from input to buffer
func (b *blockStream) loadRLPArray() (uint64, error) {
	prefix, err := b.loadRLPPrefix()
	if errors.Is(io.EOF, err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	payloadSize, payloadSizeSize, err := b.loadPrefixSize(1, prefix)
	if err != nil {
		return 0, err
	}
	if err = b.loadPayload(1+payloadSizeSize, payloadSize); err != nil {
		return 0, err
	}
	return 1 + payloadSizeSize + payloadSize, nil
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
func (b *blockStream) loadPrefixSize(offset uint64, prefix byte) (uint64, uint64, error) {
	switch {
	case prefix >= 0xc0 && prefix <= 0xf7:
		// an array whose size is less than 56
		return uint64(prefix - 0xc0), 0, nil
	case prefix >= 0xf8:
		// an array whose size is greater than or equal to 56
		// size of the data representing the size of payload
		payloadSizeSize := uint64(prefix - 0xf7)

		b.reserveCap(offset + payloadSizeSize)
		payloadSizeBytes := b.buffer[offset : offset+payloadSizeSize]
		n, err := b.input.Read(payloadSizeBytes)
		if err != nil {
			return 0, 0, err
		}
		if uint64(n) < payloadSizeSize {
			// couldn't load required amount of bytes
			return 0, 0, io.EOF
		}
		payloadSize := new(big.Int).SetBytes(payloadSizeBytes).Int64()
		return uint64(payloadSize), uint64(payloadSizeSize), nil
	}
	return 0, 0, errors.New("expected arrray but got bytes")
}

// loadPayload loads payload data from stream and store to buffer
func (b *blockStream) loadPayload(offset uint64, size uint64) error {
	b.reserveCap(offset + size)
	buf := b.buffer[offset : offset+size]
	if _, err := b.input.Read(buf); err != nil {
		return err
	}
	return nil
}

// parseMetadata parses RLP encoded Metadata in buffer
func (b *blockStream) parseMetadata(size uint64) (*Metadata, error) {
	data := b.buffer[:size]
	metadata := &Metadata{}
	if err := metadata.UnmarshalRLP(data); err != nil {
		return nil, err
	}
	return metadata, nil
}

// parseBlock parses RLP encoded Block in buffer
func (b *blockStream) parseBlock(size uint64) (*types.Block, error) {
	data := b.buffer[:size]
	block := &types.Block{}
	if err := block.UnmarshalRLP(data); err != nil {
		return nil, err
	}
	return block, nil
}

// reserveCap makes sure the internal buffer has given size
func (b *blockStream) reserveCap(size uint64) {
	if diff := int64(size) - int64(cap(b.buffer)); diff > 0 {
		b.buffer = append(b.buffer[:cap(b.buffer)], make([]byte, diff)...)
	} else {
		b.buffer = b.buffer[:size]
	}
}
