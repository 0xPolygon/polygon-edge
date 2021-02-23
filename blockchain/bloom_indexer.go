package blockchain

import (
	"context"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/types"
)

const (
	// bloomThrottling is the time to wait between processing two consecutive index sections
	// It's useful during chain upgrades to prevent disk overload.
	bloomThrottling = 100 * time.Millisecond
)

// BloomIndexer implements a ChainIndexer, building up a rotated bloom bits index for the Ethereum header bloom filters
type BloomIndexer struct {
	size           uint64          // section size to generate bloom bits for
	db             storage.Storage // database instance to write index data and metadata into
	generator      *Generator      // generator to rotate the bloom bits crating the bloom index
	currentSection uint64          // Section is the section number being processed currently
	head           types.Hash      // Head is the hash of the last header processed
}

// NewBloomIndexer returns a chain indexer that generates bloom bits data for the main chain for fast logs filtering
func NewBloomIndexer(db storage.Storage, size, confirms uint64, logger hclog.Logger) *ChainIndexer {
	backend := &BloomIndexer{
		db:   db,
		size: size,
	}

	return NewChainIndexer(db, backend, size, confirms, bloomThrottling, logger)
}

// Reset implements the ChainIndexerBackend interface
// Starts a new bloom bits index section
func (b *BloomIndexer) Reset(ctx context.Context, sectionStart uint64, lastHead types.Hash) error {
	gen, err := NewGenerator(uint(b.size))

	b.generator = gen
	b.currentSection = sectionStart
	b.head = types.Hash{} // No head yet

	return err
}

// Process implements ChainIndexerBackend interface
// Adds a new bloom into the index
func (b *BloomIndexer) Process(ctx context.Context, header *types.Header) error {

	err := b.generator.AddBloom(uint(header.Number-b.currentSection*b.size), header.LogsBloom)
	b.head = header.Hash

	return err
}

// Commit implements ChainIndexerBackend interface
// Finalizes the bloom section, and writes it to the DB
func (b *BloomIndexer) Commit(blockNr uint64) error {

	// TODO add transactions
	for i := 0; i < types.BloomBitLength; i++ {

		bits, err := b.generator.Bitset(uint(i))
		if err != nil {
			return err
		}

		if bits != nil {
			b.db.WriteBloomBits(uint(i), b.currentSection, b.head, bits)
		}
	}

	return nil
}
