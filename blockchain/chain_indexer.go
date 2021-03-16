package blockchain

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/jsonrpc"
	"github.com/0xPolygon/minimal/types"
)

// ChainIndexerBackend handles background processing of chain segments
type ChainIndexerBackend interface {
	// Reset initiates the processing of a new chain segment, potentially terminating
	// any partially completed operations (in case of a reorg).
	Reset(ctx context.Context, section uint64, lastHead types.Hash) error

	// Process crunches through the given header
	Process(ctx context.Context, header *types.Header) error

	// Commit finalizes the section data and stores it into the database
	Commit() error
}

// ChainIndexer processes fixed sized sections of the main chain
// It is connected to the blockchain through the event system, by starting the event loop
type ChainIndexer struct {
	storage storage.Storage     // Chain database to index the data from / write to
	backend ChainIndexerBackend // Background generator the index content

	active    uint32          // Flag indicating if the event loop was started
	quit      chan chan error // Quit channel to tear down running goroutines
	ctx       context.Context // The context of the operation
	ctxCancel func()          // The context cancel function

	sectionSize uint64 // Number of blocks in a single chain segment

	storedSections uint64 // Number of sections successfully indexed in the DB

	checkpointBlocks   uint64     // Number that keeps track of seen blocks, for tracking checkpoints
	checkpointHeadHash types.Hash // Section head hash belonging to the last checkpoint

	throttling time.Duration // Disk throttling delay to prevent a heavy upgrade from occupying resources

	logger hclog.Logger // Simple logger
	mux    sync.Mutex
}

// NewChainIndexer creates a new chain indexer to do background processing on
// chain segments of a given size after a certain number of confirmations passes
//
// The throttling parameter might be used to prevent database thrashing
func NewChainIndexer(
	storage storage.Storage,
	backend ChainIndexerBackend,
	section uint64,
	throttling time.Duration,
	logger hclog.Logger) *ChainIndexer {

	chainIndexer := &ChainIndexer{
		storage:     storage,
		backend:     backend,
		quit:        make(chan chan error),
		sectionSize: section,
		throttling:  throttling,
		logger:      logger,
	}

	// Initiate the checkpoint sections
	chainIndexer.checkpointBlocks = 0

	// Grab the valid sections from the DB
	chainIndexer.loadNumValidSections()
	chainIndexer.ctx, chainIndexer.ctxCancel = context.WithCancel(context.Background())

	return chainIndexer
}

// Start creates a goroutine to pass head events into the indexer for background processing
func (c *ChainIndexer) Start(blockchain jsonrpc.BlockchainInterface) {
	subscription := blockchain.SubscribeEvents()

	go c.eventLoop(blockchain.Header(), subscription)
}

// Close tears down all goroutines belonging to the indexer
// Returns any error that might have occurred internally
func (c *ChainIndexer) Close() error {
	var errs []error

	c.ctxCancel()

	// Tear down the primary update loop
	errorChannel := make(chan error)
	c.quit <- errorChannel

	if err := <-errorChannel; err != nil {
		errs = append(errs, err)
	}

	// If needed, tear down the secondary event loop
	if atomic.LoadUint32(&c.active) != 0 {
		c.quit <- errorChannel

		if err := <-errorChannel; err != nil {
			errs = append(errs, err)
		}
	}

	// Return any failures
	switch {
	case len(errs) == 0:
		return nil

	case len(errs) == 1:
		return errs[0]

	default:
		return fmt.Errorf("chain indexer errors: %v", errs)
	}
}

// eventLoop is the main event loop of the chain indexer
func (c *ChainIndexer) eventLoop(currentHeader *types.Header, sub Subscription) {

	// Mark the chain indexer as active
	atomic.StoreUint32(&c.active, 1)

	defer sub.Close()

	// Fire the initial new head event to start any outstanding processing
	c.newHead(currentHeader.Number, false)

	// Start the loop
	for {
		select {
		case errorChannel := <-c.quit:
			// Chain indexer terminating, report no failure and abort
			errorChannel <- nil
			return

		default:
			event := sub.GetEvent()

			if event == nil {
				return
			}

			// Grab the latest header
			header := event.NewChain[len(event.NewChain)-1]

			// Check if a reorg occurred
			if len(event.OldChain) > 1 {
				// Find the common ancestor in the DB, and set the head to it
				ancestorNumber := event.OldChain[0].Number

				_, ok := c.storage.ReadCanonicalHash(ancestorNumber)

				if !ok {
					c.logger.Error("Unable to find the common ancestor in the chain indexer DB")
					return
				}

				// This invalidates DB sections of the chain
				c.newHead(ancestorNumber, true)
			}

			c.newHead(header.Number, false)
		}
	}
}

// newHead notifies the indexer about new chain heads and/or reorgs
func (c *ChainIndexer) newHead(headNumber uint64, reorg bool) {
	c.mux.Lock()

	// If a reorg happened, invalidate all sections until that point
	if reorg {
		// Revert the known section number to the reorg point
		startSection := (headNumber + 1) / c.sectionSize

		// Update the valid sections to reflect the reorg
		if startSection <= c.storedSections {
			c.setNumValidSections(startSection - 1)
		}

		c.checkpointBlocks = headNumber - 1

		lastCheckpointHeaderBlock := headNumber - (headNumber % c.sectionSize) - 1

		// Roll back the checkpoint head
		var ok bool
		c.checkpointHeadHash, ok = c.storage.ReadCanonicalHash(lastCheckpointHeaderBlock)

		if !ok {
			_ = fmt.Errorf("bloom indexer: unable to read checkpoint hash of block %v", lastCheckpointHeaderBlock)
		}
	}

	c.checkpointBlocks++

	// Check if a section needs to be processed
	if c.checkpointBlocks%c.sectionSize == 0 {
		c.verifyLastHead()

		// A checkpoint has been reached, process the section
		section := c.storedSections

		// Cache the old head hash
		var oldHead types.Hash
		if section > 0 {
			oldHead = c.SectionHead(section - 1)
		}

		// Process the newly defined section in the background
		c.mux.Unlock()

		newHead, err := c.processSection(section, oldHead)

		if err != nil {
			select {
			case <-c.ctx.Done():
				<-c.quit <- nil
				return
			default:
			}
			_ = fmt.Errorf("bloom indexer: Section processing failed - %s", err)
		}

		c.mux.Lock()

		// If processing succeeded and no reorgs occurred, mark the section completed
		if err == nil && (section == 0 || oldHead == c.SectionHead(section-1)) {

			c.setSectionHead(section, newHead)
			c.setNumValidSections(section + 1)
			c.checkpointHeadHash = newHead

		} else {
			// Processing failed, roll back valid headers
			c.verifyLastHead()
		}
	}

	c.mux.Unlock()
}

// processSection processes an entire section by calling backend functions while
// ensuring the continuity of the passed headers. Since the chain mutex is not
// held while processing, the continuity can be broken by a long reorg, in which
// case the function returns with an error
func (c *ChainIndexer) processSection(section uint64, lastHead types.Hash) (types.Hash, error) {
	fmt.Printf("Bloom indexer: Processing new chain section (%d)", section)

	// Reset and partial processing
	if err := c.backend.Reset(c.ctx, section, lastHead); err != nil {
		c.setNumValidSections(0)
		return types.Hash{}, err
	}

	// Process all blocks in the range [section * section size, (section + 1) * section size)
	for blockNum := section * c.sectionSize; blockNum < (section+1)*c.sectionSize; blockNum++ {

		hash, ok := c.storage.ReadCanonicalHash(blockNum)
		if !ok {
			return types.Hash{}, fmt.Errorf("unable to read canonical hash")
		}

		if hash == (types.Hash{}) {
			return types.Hash{}, fmt.Errorf("Bloom indexer: canonical block #%d unknown", blockNum)
		}

		header, err := c.storage.ReadHeader(hash)

		if err != nil {
			return types.Hash{}, fmt.Errorf("unable to read header for hash %s", hash)
		}

		if header == nil {

			return types.Hash{}, fmt.Errorf("bloom indexer: block #%d [%x…] not found", blockNum, hash[:4])
		} else if header.ParentHash != lastHead {

			return types.Hash{}, fmt.Errorf("bloom indexer: chain reorged during section processing")
		}

		if err := c.backend.Process(c.ctx, header); err != nil {
			return types.Hash{}, err
		}

		lastHead = header.Hash
	}

	if err := c.backend.Commit(); err != nil {
		return types.Hash{}, err
	}

	return lastHead, nil
}

// verifyLastHead compares last stored section head with the corresponding block hash in the
// actual canonical chain and rolls back reorged sections if necessary to ensure that stored
// sections are all valid
func (c *ChainIndexer) verifyLastHead() {

	for c.storedSections > 0 && c.storedSections > (c.checkpointBlocks/c.sectionSize) {
		mainChainHash, ok := c.storage.ReadCanonicalHash(c.storedSections*c.sectionSize - 1)
		if !ok {
			panic("")
		}

		if c.SectionHead(c.storedSections-1) == mainChainHash {
			return
		}

		c.setNumValidSections(c.storedSections - 1)
	}
}

// Sections returns the number of processed sections maintained by the indexer
// and also the information about the last header indexed for potential canonical
// verifications
func (c *ChainIndexer) Sections() (uint64, uint64, types.Hash) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.verifyLastHead()

	return c.storedSections, c.storedSections*c.sectionSize - 1, c.SectionHead(c.storedSections - 1)
}

// loadValidSections fetches the number of valid sections in the DB, and stores it in the local state
func (c *ChainIndexer) loadNumValidSections() {
	c.storedSections, _ = c.storage.ReadValidSectionsNum()
}

// setValidSections sets the number of valid sections to the DB
func (c *ChainIndexer) setNumValidSections(sections uint64) {
	// Set the current number of valid sections in the database
	_ = c.storage.WriteValidSectionsNum(sections)

	// Remove any reorged sections
	for c.storedSections > sections {
		c.storedSections--
		c.removeSectionHead(c.storedSections)
	}

	c.storedSections = sections
}

// SectionHead retrieves the last block hash of a processed section from the index DB
func (c *ChainIndexer) SectionHead(section uint64) types.Hash {
	hash := c.storage.ReadIndexSectionHead(section)

	return hash
}

// setSectionHead writes the last block hash of a section to the index DB
func (c *ChainIndexer) setSectionHead(section uint64, hash types.Hash) {
	_ = c.storage.WriteIndexSectionHead(section, hash)
}

// removeSectionHead removes the reference to a processed section from the index DB
func (c *ChainIndexer) removeSectionHead(section uint64) {
	_ = c.storage.RemoveSectionHead(section)
}
