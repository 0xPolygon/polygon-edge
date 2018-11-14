package blockchain

import (
	"fmt"
	"math/big"

	"github.com/umbracle/minimal/consensus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/storage"
)

var (
	NOTFOUND = "leveldb: not found"
)

// Blockchain is a blockchain reference
type Blockchain struct {
	db        *storage.Storage
	consensus consensus.Consensus
	genesis   *types.Header
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(db *storage.Storage, consensus consensus.Consensus) *Blockchain {
	return &Blockchain{db, consensus, nil}
}

// GetParent return the parent
func (b *Blockchain) GetParent(header *types.Header) (*types.Header, error) {
	return b.db.ReadHeader(header.ParentHash)
}

// Genesis returns the genesis block
func (b *Blockchain) Genesis() *types.Header {
	return b.genesis
}

// WriteGenesis writes the genesis block if not present
func (b *Blockchain) WriteGenesis(header *types.Header) error {
	b.genesis = header

	hash, err := b.db.ReadHeadHash()
	if err != nil && err.Error() != NOTFOUND {
		return err
	}
	if hash != nil {
		return nil
	}

	// add genesis block
	if err := b.addHeader(header); err != nil {
		return err
	}
	return b.advanceHead(header)
}

func (b *Blockchain) advanceHead(h *types.Header) error {
	if err := b.db.WriteHeadHash(h.Hash()); err != nil {
		return err
	}

	if err := b.db.WriteHeadNumber(h.Number); err != nil {
		return err
	}
	return nil
}

// Header returns the header of the blockchain
func (b *Blockchain) Header() (*types.Header, error) {
	hash, err := b.db.ReadHeadHash()
	if err != nil {
		return nil, err
	}
	header, err := b.db.ReadHeader(*hash)
	if err != nil {
		return nil, err
	}
	return header, nil
}

// GetHeaderByHash returns the header by his hash
func (b *Blockchain) GetHeaderByHash(hash common.Hash) *types.Header {
	h, _ := b.db.ReadHeader(hash)
	return h
}

// GetHeaderByNumber returns the header by his number
func (b *Blockchain) GetHeaderByNumber(n *big.Int) *types.Header {
	hash, err := b.db.ReadCanonicalHash(n)
	if err != nil {
		return nil
	}
	h, _ := b.db.ReadHeader(hash)
	return h
}

// WriteHeaders writes a batch of headers
func (b *Blockchain) WriteHeaders(headers []*types.Header) error {

	// validate chain
	for i := 1; i < len(headers); i++ {
		if headers[i].Number.Uint64()-1 != headers[i-1].Number.Uint64() {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, headers[i].Number.Uint64(), headers[i-1].Number.Uint64())
		}
		if headers[i].ParentHash != headers[i-1].Hash() {
			return fmt.Errorf("parent hash not correct")
		}
		// TODO: check the first header
		if err := b.consensus.VerifyHeader(headers[i-1], headers[i], true); err != nil {
			return fmt.Errorf("failed to verify the header: %v", err)
		}
	}

	// NOTE: Add headers in batches, check if the parent of the first header
	// exists, write all the blocks and set the last block as the head.

	for _, h := range headers {
		if err := b.WriteHeader(h); err != nil {
			// rollback? we have to remove all the blocks written, cache
			return err
		}
	}

	return nil
}

func (b *Blockchain) addHeader(header *types.Header) error {
	if err := b.db.WriteHeader(header); err != nil {
		return err
	}
	if err := b.db.WriteCanonicalHash(header.Number, header.Hash()); err != nil {
		return err
	}
	return nil
}

// WriteHeader writes a block and the data, assumes the genesis is already set
func (b *Blockchain) WriteHeader(header *types.Header) error {
	head, err := b.Header()
	if err != nil {
		return err
	}

	parent, err := b.db.ReadHeader(header.ParentHash)
	if err != nil {
		return err
	}

	// local difficulty of the block
	localDiff := big.NewInt(1).Add(parent.Difficulty, header.Difficulty)

	// Write the data
	if err := b.addHeader(header); err != nil {
		return err
	}

	if header.ParentHash == head.Hash() {
		// advance the chain
		if err := b.advanceHead(header); err != nil {
			return err
		}
	} else if head.Difficulty.Cmp(localDiff) < 0 {
		// reorg
		if err := b.handleReorg(head, header); err != nil {
			return err
		}
	} else {
		// fork
		if err := b.handleFork(parent, header); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) handleFork(parent, header *types.Header) error {
	forks, err := b.db.ReadForks()
	if err != nil {
		if err.Error() == NOTFOUND {
			forks = []common.Hash{}
		} else {
			return err
		}
	}

	newForks := []common.Hash{}
	for _, fork := range forks {
		if fork != parent.Hash() {
			newForks = append(newForks, fork)
		}
	}
	newForks = append(newForks, header.Hash())

	b.db.WriteForks(newForks)
	return nil
}

func (b *Blockchain) handleReorg(oldHeader *types.Header, newHeader *types.Header) error {
	newChainHead := newHeader

	var err error
	for oldHeader.Number.Cmp(newHeader.Number) > 0 {
		oldHeader, err = b.db.ReadHeader(oldHeader.ParentHash)
		if err != nil {
			return err
		}
	}

	for newHeader.Number.Cmp(oldHeader.Number) > 0 {
		newHeader, err = b.db.ReadHeader(newHeader.ParentHash)
		if err != nil {
			return err
		}
	}

	for oldHeader.Hash() != newHeader.Hash() {
		oldHeader, err = b.db.ReadHeader(oldHeader.ParentHash)
		if err != nil {
			return err
		}

		newHeader, err = b.db.ReadHeader(newHeader.ParentHash)
		if err != nil {
			return err
		}
	}

	// TODO: Handle the fork

	// NOTE. this loops are used to know the oldblocks not belonging anymore
	// to the canonical chain and updating the tx and state

	return b.advanceHead(newChainHead)
}

// GetForks returns the forks
func (b *Blockchain) GetForks() []common.Hash {
	forks, _ := b.db.ReadForks()
	return forks
}
