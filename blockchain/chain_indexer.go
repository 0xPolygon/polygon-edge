package blockchain

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/types"
)

type BlockNumber int64

// ChainIndexerBackend handles background processing of chain segments
type ChainIndexerBackend interface {
	// Reset initiates the processing of a new chain segment, potentially terminating
	// any partially completed operations (in case of a reorg).
	Reset(ctx context.Context, section uint64, lastHead types.Hash) error

	// Process crunches through the next header in the chain segment. The caller
	// will ensure a sequential order of headers.
	Process(ctx context.Context, header *types.Header) error

	// Prune deletes the chain index older than the given threshold.
	Commit(blockNr uint64) error
}

// ChainIndexer processes fixed sized sections of the main chain
// It is connected to the blockchain through the event system, by starting the event loop
// The ChainIndexer can have child ChainIndexers, which get their data from the parent indexer (new head notifications)
type ChainIndexer struct {
	storage  storage.Storage     // Chain database to index the data from / write to
	backend  ChainIndexerBackend // Background generator the index data content
	children []*ChainIndexer     // Child indexers to cascade chain updates to

	active    uint32          // Flag indicating if the event loop was started
	update    chan struct{}   // Notification channel receives headers
	quit      chan chan error // Quit channel to tear down running goroutines
	ctx       context.Context // The context of the operation
	ctxCancel func()          // The context cancel function

	sectionSize uint64 // Number of blocks in a single chain segment
	confirmsReq uint64 // Number of confirmations before processing a completed segment

	storedSections uint64 // Number of sections successfully indexed in the DB
	knownSections  uint64 // Number of sections known to be complete (block wise)
	cascadedHead   uint64 // Block number of the last completed section cascaded to child indexers

	checkpointSections uint64     // Number of sections covered by the checkpoint
	checkpointHeadHash types.Hash // Section head hash belonging to the checkpoint

	throttling time.Duration // Disk throttling delay to prevent a heavy upgrade from occupying resources

	logger hclog.Logger // Simple logger
	mux    sync.Mutex
}

// ChainIndexerChain is used for connecting the indexer to a blockchain
type ChainIndexerChain interface {
	// CurrentHeader retrieves the latest locally known header
	CurrentHeader() *types.Header

	// SubscribeChainHeadEvent subscribes to new chain header notifications
	SubscribeChainHeadEvent(ch chan *Event) Subscription
}

// NewChainIndexer creates a new chain indexer to do background processing on
// chain segments of a given size after a certain number of confirmations passes
//
// The throttling parameter might be used to prevent database thrashing
func NewChainIndexer(
	storage storage.Storage,
	backend ChainIndexerBackend,
	section,
	confirm uint64,
	throttling time.Duration,
	logger hclog.Logger) *ChainIndexer {

	c := &ChainIndexer{
		storage:     storage,
		backend:     backend,
		update:      make(chan struct{}, 1),
		quit:        make(chan chan error),
		sectionSize: section,
		confirmsReq: confirm,
		throttling:  throttling,
		logger:      logger,
	}

	// Initialize database dependent fields and start the updater
	c.loadNumValidSections()
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())

	go c.updateLoop()

	return c
}

// AddCheckpoint adds a checkpoint. Sections are never processed and the chain
// is not expected to be available before this point. The indexer assumes that
// the backend has sufficient information available to process subsequent sections
//
// Note: knownSections == 0 and storedSections == checkpointSections until
// syncing reaches the checkpoint
func (c *ChainIndexer) AddCheckpoint(section uint64, sectionHeadHash types.Hash) {
	c.mux.Lock()
	defer c.mux.Unlock()

	// Break if the given checkpoint is lower than the local's checkpoint
	if c.checkpointSections >= section+1 || section < c.storedSections {
		return
	}

	// Update the checkpoint
	c.checkpointSections = section + 1
	c.checkpointHeadHash = sectionHeadHash

	// Update the current values
	c.setSectionHead(section, sectionHeadHash)
	c.setNumValidSections(section + 1)
}

// Start creates a goroutine to pass head events into the indexer for background processing
// Children do not need to be started, they are notified about new events by their parents
func (c *ChainIndexer) Start(chain ChainIndexerChain) {
	events := make(chan *Event, 10)

	subscription := chain.SubscribeChainHeadEvent(events)

	go c.eventLoop(chain.CurrentHeader(), events, subscription)
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

	// Close all children
	for _, child := range c.children {
		if err := child.Close(); err != nil {
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
		return fmt.Errorf("Bloom indexer errors: %v", errs)
	}
}

// eventLoop is a secondary event loop of the indexer which is only
// started for the outermost indexer to push chain head events into a processing queue
func (c *ChainIndexer) eventLoop(currentHeader *types.Header, events chan *Event, sub Subscription) {

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

		case event, ok := <-events:

			// Received a new event, ensure it's not nil (closing) and update
			if !ok {
				errorChannel := <-c.quit
				errorChannel <- nil
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
func (c *ChainIndexer) newHead(headNumber uint64, isReorg bool) {
	c.mux.Lock()

	defer c.mux.Unlock()

	// If a reorg happened, invalidate all sections until that point
	if isReorg {
		// Revert the known section number to the reorg point
		known := (headNumber + 1) / c.sectionSize
		stored := known

		if known < c.checkpointSections {
			known = 0
		}

		if stored < c.checkpointSections {
			stored = c.checkpointSections
		}

		if known < c.knownSections {
			c.knownSections = known
		}

		// Revert the stored sections from the database to the reorg point
		if stored < c.storedSections {
			c.setNumValidSections(stored)
		}

		// Update the new head number to the finalized section end and notify children
		headNumber = known * c.sectionSize

		if headNumber < c.cascadedHead {
			// The children have invalid head data
			c.cascadedHead = headNumber
			for _, child := range c.children {
				child.newHead(c.cascadedHead, true)
			}
		}
		return
	}

	// Calculate the number of newly known sections and update if high enough
	var sections uint64
	if headNumber >= c.confirmsReq {

		sections = (headNumber + 1 - c.confirmsReq) / c.sectionSize
		if sections < c.checkpointSections {
			sections = 0
		}

		if sections > c.knownSections {
			if c.knownSections < c.checkpointSections {
				// syncing reached the checkpoint, verify section head
				syncedHead, err := c.storage.ReadCanonicalHash(c.checkpointSections*c.sectionSize - 1)
				if err {
					panic(err)
				}

				if syncedHead != c.checkpointHeadHash {
					_ = fmt.Errorf("Bloom Indexer - Synced chain does not match checkpoint number: %d, expected %s synced %s",
						c.checkpointSections*c.sectionSize-1,
						c.checkpointHeadHash,
						syncedHead)
					return
				}
			}
			c.knownSections = sections

			select {
			case c.update <- struct{}{}:
			default:
			}
		}
	}
}

// updateLoop is the main event loop of the indexer which pushes chain segments into the processing backend
func (c *ChainIndexer) updateLoop() {

	var (
		updating bool
		updated  time.Time
	)

	for {
		select {
		case errChannel := <-c.quit:
			// Chain indexer terminating, report no failure and abort
			errChannel <- nil
			return

		case <-c.update:
			// Section headers completed (or rolled back), update the index
			c.mux.Lock()

			if c.knownSections > c.storedSections {

				// Periodically print an update log message to the user
				if time.Since(updated) > 10*time.Second {
					if c.knownSections > c.storedSections+1 {
						updating = true
						fmt.Printf("Bloom Indexer: Updating chain index: %d%", c.storedSections*100/c.knownSections)
					}

					updated = time.Now()
				}

				// Cache the current section count
				c.verifyLastHead()

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
					_ = fmt.Errorf("Bloom indexer: Section processing failed - %s", err)
				}
				c.mux.Lock()

				// If processing succeeded and no reorgs occurred, mark the section completed
				if err == nil && (section == 0 || oldHead == c.SectionHead(section-1)) {

					c.setSectionHead(section, newHead)
					c.setNumValidSections(section + 1)

					if c.storedSections == c.knownSections && updating {
						updating = false
						fmt.Printf("Bloom indexer: Finished updating chain index")
					}

					// Alert the children indexers
					c.cascadedHead = c.storedSections*c.sectionSize - 1
					for _, child := range c.children {
						child.newHead(c.cascadedHead, false)
					}
				} else {
					// If processing failed, don't retry until further notification
					c.verifyLastHead()
					c.knownSections = c.storedSections
				}
			}

			// If there are still further sections to process, reschedule the processing
			if c.knownSections > c.storedSections {
				time.AfterFunc(c.throttling, func() {
					select {
					case c.update <- struct{}{}:
					default:
					}
				})
			}

			c.mux.Unlock()
		}
	}
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
			return types.Hash{}, fmt.Errorf("Unable to read canonical hash")
		}

		if hash == (types.Hash{}) {
			return types.Hash{}, fmt.Errorf("Bloom indexer: canonical block #%d unknown", blockNum)
		}

		header, ok := c.storage.ReadHeader(hash)

		if !ok {
			return types.Hash{}, fmt.Errorf("Unable to read header for hash %s", hash)
		}

		if header == nil {

			return types.Hash{}, fmt.Errorf("Bloom indexer: block #%d [%x…] not found", blockNum, hash[:4])
		} else if header.ParentHash != lastHead {

			return types.Hash{}, fmt.Errorf("Bloom indexer: chain reorged during section processing")
		}

		if err := c.backend.Process(c.ctx, header); err != nil {
			return types.Hash{}, err
		}

		lastHead = header.Hash
	}

	writeBlockNr := (section + 1) * c.sectionSize

	if err := c.backend.Commit(writeBlockNr - 1); err != nil {
		return types.Hash{}, err
	}
	return lastHead, nil
}

// verifyLastHead compares last stored section head with the corresponding block hash in the
// actual canonical chain and rolls back reorged sections if necessary to ensure that stored
// sections are all valid
func (c *ChainIndexer) verifyLastHead() {

	for c.storedSections > 0 && c.storedSections > c.checkpointSections {
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

// AddChildIndexer adds a child ChainIndexer that can use the output of this one
func (c *ChainIndexer) AddChildIndexer(indexer *ChainIndexer) {
	if indexer == c {
		panic("Bloom indexer: Can't add indexer as a child of itself")
	}

	c.mux.Lock()
	defer c.mux.Unlock()

	c.children = append(c.children, indexer)

	// Cascade any pending updates to new children too
	sections := c.storedSections

	if c.knownSections < sections {
		// if a section is "stored" but not "known" then it is a checkpoint without
		// available chain data so we should not cascade it yet
		sections = c.knownSections
	}

	if sections > 0 {
		indexer.newHead(sections*c.sectionSize-1, false)
	}
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
