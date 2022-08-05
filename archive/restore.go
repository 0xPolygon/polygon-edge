package archive

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	restore = "restore"
)

type blockchainInterface interface {
	SubscribeEvents() blockchain.Subscription
	Genesis() types.Hash
	GetBlockByNumber(uint64, bool) (*types.Block, bool)
	GetHashByNumber(uint64) types.Hash
	WriteBlock(*types.Block, string) error
	VerifyFinalizedBlock(*types.Block) error
}

// RestoreChain reads blocks from the archive and write to the chain
func RestoreChain(chain blockchainInterface, filePath string, progression *progress.ProgressionWrapper) error {
	fp, err := os.Open(filePath)
	if err != nil {
		return err
	}

	blockStream := newBlockStream(fp)

	return importBlocks(chain, blockStream, progression)
}

// import blocks scans all blocks from stream and write them to chain
func importBlocks(chain blockchainInterface, blockStream *blockStream, progression *progress.ProgressionWrapper) error {
	shutdownCh := common.GetTerminationSignalCh()

	metadata, err := blockStream.getMetadata()
	if err != nil {
		return err
	}

	if metadata == nil {
		return errors.New("expected metadata in archive but doesn't exist")
	}

	// check whether the local chain has the latest block already
	latestBlock, ok := chain.GetBlockByNumber(metadata.Latest, false)
	if ok && latestBlock.Hash() == metadata.LatestHash {
		return nil
	}

	// skip existing blocks
	firstBlock, err := consumeCommonBlocks(chain, blockStream, shutdownCh)
	if err != nil {
		return err
	}

	if firstBlock == nil {
		return nil
	}

	// Create a blockchain subscription for the sync progression and start tracking
	progression.StartProgression(firstBlock.Number(), chain.SubscribeEvents())
	// Stop monitoring the sync progression upon exit
	defer progression.StopProgression()

	// Set the goal
	progression.UpdateHighestProgression(metadata.Latest)

	nextBlock := firstBlock

	for {
		if err := chain.VerifyFinalizedBlock(nextBlock); err != nil {
			return err
		}

		if err := chain.WriteBlock(nextBlock, restore); err != nil {
			return err
		}

		progression.UpdateCurrentProgression(nextBlock.Number())

		nextBlock, err = blockStream.nextBlock()
		if err != nil {
			return err
		}

		if nextBlock == nil {
			break
		}

		select {
		case <-shutdownCh:
			return nil
		default:
		}
	}

	return nil
}

// consumeCommonBlocks consumes blocks in blockstream to latest block in chain or different hash
// returns the first block to be written into chain
func consumeCommonBlocks(
	chain blockchainInterface,
	blockStream *blockStream,
	shutdownCh <-chan os.Signal,
) (*types.Block, error) {
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
				return nil, fmt.Errorf(
					"the hash of genesis block (%s) does not match blockchain genesis (%s)",
					block.Hash(),
					chain.Genesis(),
				)
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

	block, err := b.parseBlock(size)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// loadRLPArray loads RLP encoded array from input to buffer
func (b *blockStream) loadRLPArray() (uint64, error) {
	prefix, err := b.loadRLPPrefix()
	if errors.Is(io.EOF, err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	// read information from RLP array header
	headerSize, payloadSize, err := b.loadPrefixSize(1, prefix)
	if err != nil {
		return 0, err
	}

	if err = b.loadPayload(headerSize, payloadSize); err != nil {
		return 0, err
	}

	return headerSize + payloadSize, nil
}

// loadRLPPrefix loads first byte of RLP encoded data from input
func (b *blockStream) loadRLPPrefix() (byte, error) {
	buf := b.buffer[:1]
	if _, err := b.input.Read(buf); err != nil {
		return 0, err
	}

	return buf[0], nil
}

// loadPrefixSize loads array's size from input and return RLP header size and payload size
// basically block should be array in RLP encoded value
// because block has 3 fields on the top: Header, Transactions, Uncles
func (b *blockStream) loadPrefixSize(offset uint64, prefix byte) (uint64, uint64, error) {
	switch {
	case prefix >= 0xc0 && prefix <= 0xf7:
		// an array whose size is less than 56
		return 1, uint64(prefix - 0xc0), nil
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

		return payloadSizeSize + 1, uint64(payloadSize), nil
	}

	return 0, 0, errors.New("expected array but got bytes")
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
