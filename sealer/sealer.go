package sealer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/consensus"
)

// Sealer seals blocks
type Sealer struct {
	config     *Config
	logger     *log.Logger
	lastHeader *types.Header

	blockchain *blockchain.Blockchain
	engine     consensus.Consensus
	txPool     *TxPool

	// sealer
	stopFn   context.CancelFunc
	newBlock chan struct{}

	// commit block every n seconds if no transactions are sent
	commitInterval *time.Timer
}

// NewSealer creates a new sealer for a specific engine
func NewSealer(config *Config, logger *log.Logger, blockchain *blockchain.Blockchain, engine consensus.Consensus) *Sealer {
	if config.CommitInterval < minCommitInterval {
		config.CommitInterval = minCommitInterval
	}

	s := &Sealer{
		blockchain:     blockchain,
		engine:         engine,
		config:         config,
		logger:         logger,
		txPool:         NewTxPool(blockchain),
		newBlock:       make(chan struct{}, 1),
		commitInterval: time.NewTimer(config.CommitInterval),
	}
	return s
}

func (s *Sealer) Start() error {
	go s.loop()

	return nil
}

func (s *Sealer) AddTx(tx *types.Transaction) {
	s.txPool.Add(tx)
}

func (s *Sealer) loop() {

	for {
		select {
		case <-s.newBlock:
			s.commit(false)

		case <-s.commitInterval.C:
			s.commit(true)
		}
	}
}

func (s *Sealer) commit(interval bool) {
	s.commitInterval.Reset(s.config.CommitInterval)

	// we have been notified of another block
	// or the time just passed

	h := s.blockchain.Header()

	promoted, err := s.txPool.reset(s.lastHeader, h)
	if err != nil {
		panic(err)
	}

	fmt.Println(promoted)

	// TODO, take an instance of the state and apply the new promoted transactions
	// Send the new state to the blockchain once sealed
}

func (s *Sealer) NotifyBlock(b *types.Block) {
	select {
	case s.newBlock <- struct{}{}:
	default:
	}
}

func (s *Sealer) seal(b *types.Block) {
	if s.stopFn != nil {
		s.stopFn()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.stopFn = cancel

	if err := s.engine.Seal(ctx, b); err != nil {
		panic(err)
	}
}
