package tracker

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	bt "github.com/umbracle/ethgo/blocktracker"
)

const (
	defaultMaxBlockBacklog = 10
)

// BlockTracker is an interface to track new blocks on the chain
type BlockTracker struct {
	config       *Config
	blocks       []*ethgo.Block
	blocksLock   sync.Mutex
	subscriber   bt.BlockTrackerInterface
	blockChs     []chan *bt.BlockEvent
	blockChsLock sync.Mutex
	provider     bt.BlockProvider
	closeCh      chan struct{}
	logger       hclog.Logger
}

type Config struct {
	Tracker         bt.BlockTrackerInterface
	MaxBlockBacklog uint64
}

func DefaultConfig() *Config {
	return &Config{
		MaxBlockBacklog: defaultMaxBlockBacklog,
	}
}

type ConfigOption func(*Config)

func WithBlockMaxBacklog(b uint64) ConfigOption {
	return func(c *Config) {
		c.MaxBlockBacklog = b
	}
}

func WithTracker(b bt.BlockTrackerInterface) ConfigOption {
	return func(c *Config) {
		c.Tracker = b
	}
}

func NewBlockTracker(provider bt.BlockProvider, logger hclog.Logger, opts ...ConfigOption) *BlockTracker {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	tracker := config.Tracker
	if tracker == nil {
		tracker = bt.NewJSONBlockTracker(provider)
	}

	return &BlockTracker{
		blocks:       nil,
		blockChs:     nil,
		config:       config,
		subscriber:   tracker,
		provider:     provider,
		closeCh:      make(chan struct{}),
		logger:       logger,
		blocksLock:   sync.Mutex{},
		blockChsLock: sync.Mutex{},
	}
}

func (t *BlockTracker) Subscribe() chan *bt.BlockEvent {
	t.blockChsLock.Lock()
	defer t.blockChsLock.Unlock()

	ch := make(chan *bt.BlockEvent, 1)
	t.blockChs = append(t.blockChs, ch)

	return ch
}

func (t *BlockTracker) AcquireLock() bt.Lock {
	return bt.NewLock(&t.blocksLock)
}

func (t *BlockTracker) Init() error {
	return nil
}

func (t *BlockTracker) MaxBlockBacklog() uint64 {
	return t.config.MaxBlockBacklog
}

func (t *BlockTracker) LastBlocked() *ethgo.Block {
	if len(t.blocks) == 0 {
		return nil
	}

	return t.blocks[len(t.blocks)-1].Copy()
}

func (t *BlockTracker) BlocksBlocked() []*ethgo.Block {
	res := make([]*ethgo.Block, len(t.blocks))
	for i, block := range t.blocks {
		res[i] = block.Copy()
	}

	return res
}

func (t *BlockTracker) Len() int {
	return len(t.blocks)
}

func (t *BlockTracker) Close() error {
	close(t.closeCh)

	return nil
}

func (t *BlockTracker) Start() error {
	ctx, cancelFn := context.WithCancel(context.Background())
	go func() {
		<-t.closeCh
		cancelFn()
	}()

	// start the polling
	return t.subscriber.Track(ctx, func(block *ethgo.Block) error {
		return t.HandleTrackedBlock(block)
	})
}

func (t *BlockTracker) AddBlockLocked(block *ethgo.Block) error {
	if uint64(len(t.blocks)) == t.config.MaxBlockBacklog {
		// remove past blocks if there are more than maxReconcileBlocks
		t.blocks = t.blocks[1:]
	}

	if len(t.blocks) != 0 {
		if lastNum := t.blocks[len(t.blocks)-1].Number; lastNum+1 != block.Number {
			return fmt.Errorf("bad number sequence. %d and %d", lastNum, block.Number)
		}
	}

	t.blocks = append(t.blocks, block)

	return nil
}

func (t *BlockTracker) blockAtIndex(hash ethgo.Hash) int {
	for indx, b := range t.blocks {
		if b.Hash == hash {
			return indx
		}
	}

	return -1
}

func (t *BlockTracker) handleReconcileImpl(block *ethgo.Block) ([]*ethgo.Block, int, error) {
	// Append to the head of the chain
	if len(t.blocks) > 0 && t.blocks[len(t.blocks)-1].Hash == block.ParentHash {
		return []*ethgo.Block{block}, -1, nil
	}

	// The block already exists, but if not last, remove all the following
	if indx := t.blockAtIndex(block.Hash); indx != -1 {
		return nil, indx + 1, nil
	}

	// Backfill. We dont know the parent of the block.
	// Need to query the chain until we find a known block
	var (
		added        = []*ethgo.Block{block}
		count uint64 = 0
		indx         = 0 // if indx stays 0 - it means remove all from the current state
		err   error
	)

	for ; count < t.config.MaxBlockBacklog && block.Number > 1; count++ {
		parentHash := block.ParentHash

		// if there is a parent at some index, break loop and update from where should we delete
		if indx = t.blockAtIndex(parentHash); indx != -1 {
			indx++

			break
		}

		block, err = t.provider.GetBlockByHash(parentHash, false)
		if err != nil {
			return nil, -1, fmt.Errorf("parent with hash retrieving error: %w", err)
		} else if block == nil {
			// if block does not exist (for example reorg happened) GetBlockByHash will return nil, nil
			return nil, -1, fmt.Errorf("parent with hash %s not found", parentHash)
		}

		added = append(added, block)
	}

	// need the blocks in reverse order
	for i := len(added)/2 - 1; i >= 0; i-- {
		opp := len(added) - 1 - i
		added[i], added[opp] = added[opp], added[i]
	}

	if count == t.config.MaxBlockBacklog {
		t.logger.Info("reconcile did not found parent for new blocks", "hash", added[0].Hash)
	}

	return added, indx, nil
}

func (t *BlockTracker) HandleBlockEvent(block *ethgo.Block) (*bt.BlockEvent, error) {
	t.blocksLock.Lock()
	defer t.blocksLock.Unlock()

	blocks, indx, err := t.handleReconcileImpl(block)
	if err != nil {
		return nil, err
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	blockEvnt := &bt.BlockEvent{}

	// there are some blocks to remove
	if indx >= 0 && indx < len(t.blocks) {
		for i := indx; i < len(t.blocks); i++ {
			blockEvnt.Removed = append(blockEvnt.Removed, t.blocks[i])
		}

		t.blocks = t.blocks[:indx]
	}

	// include the new blocks
	for _, block := range blocks {
		blockEvnt.Added = append(blockEvnt.Added, block)

		if err := t.AddBlockLocked(block); err != nil {
			return nil, err
		}
	}

	return blockEvnt, nil
}

func (t *BlockTracker) HandleTrackedBlock(block *ethgo.Block) error {
	blockEvnt, err := t.HandleBlockEvent(block)
	if err != nil {
		return err
	}

	if blockEvnt != nil {
		t.blockChsLock.Lock()
		defer t.blockChsLock.Unlock()

		for _, ch := range t.blockChs {
			ch <- blockEvnt
		}
	}

	return nil
}
