package sealer

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
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

	signer   types.Signer
	coinbase common.Address

	// sealer
	stopSealing context.CancelFunc
	newBlock    chan struct{}

	// commit block every n seconds if no transactions are sent
	commitInterval *time.Timer

	stopFn  context.CancelFunc
	lock    sync.Mutex
	enabled bool

	sealedCh chan *SealedNotify
}

type SealedNotify struct {
	Block *types.Block
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
		signer:         types.NewEIP155Signer(big.NewInt(1)),
		sealedCh:       make(chan *SealedNotify, 1),
	}
	return s
}

func (s *Sealer) SetCoinbase(coinbase common.Address) {
	s.coinbase = coinbase
}

func (s *Sealer) SetEnabled(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	wasRunning := s.enabled
	s.enabled = enabled

	if !enabled && wasRunning {
		// stop the sealer
		s.stopFn()
	} else if enabled && !wasRunning {
		// start the sealer
		ctx, cancel := context.WithCancel(context.Background())
		s.stopFn = cancel
		s.run(ctx)
	}
}

func (s *Sealer) run(ctx context.Context) {
	for {
		select {
		case <-s.newBlock:
			s.commit()

		case <-s.commitInterval.C:
			s.commit()

		case <-ctx.Done():
			return
		}
	}
}

func (s *Sealer) AddTx(tx *types.Transaction) {
	s.txPool.Add(tx)
}

func (s *Sealer) commit() {
	s.commitInterval.Reset(s.config.CommitInterval)

	// Check if there is a sealing going on.
	// If the sealing started some n seconds before we got the new message
	// we may want to keep it sealing
	// If the block we are sealing is way older than the current block
	// we definitely want to remove it. Can that happen?
	if s.stopSealing != nil {
		s.stopSealing()
	}

	// we have been notified of another block
	// or the time just passed

	parent := s.blockchain.Header()

	fmt.Println("-- Sealing --")
	fmt.Printf("Parent number: %d\n", parent.Number)
	fmt.Printf("Parent root: %s\n", parent.Root.String())

	promoted, err := s.txPool.reset(s.lastHeader, parent)
	if err != nil {
		panic(err)
	}

	fmt.Println(promoted)

	pricedTxs := newTxPriceHeap()
	for _, tx := range promoted {
		from, err := types.Sender(s.signer, tx)
		if err != nil {
			panic("invalid sender")
		}

		if err := pricedTxs.Push(from, tx, tx.GasPrice().Uint64()); err != nil {
			panic(err)
		}
	}

	timestamp := time.Now().Unix()
	if parent.Time.Cmp(new(big.Int).SetInt64(timestamp)) >= 0 {
		timestamp = parent.Time.Int64() + 1
	}

	num := parent.Number
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   calcGasLimit(parent, 8000000, 8000000),
		Extra:      []byte{},
		Time:       big.NewInt(timestamp),
		Coinbase:   s.coinbase,
	}

	if err := s.engine.Prepare(parent, header); err != nil {
		panic(err)
	}

	state, ok := s.blockchain.GetState(parent)
	if !ok {
		panic("state not found")
	}

	txIterator := func(err error, gas uint64) (*types.Transaction, bool) {
		if gas < params.TxGas {
			return nil, false
		}
		tx := pricedTxs.Pop()
		if tx == nil {
			return nil, false
		}
		return tx.tx, true
	}

	newState, root, txns, err := s.blockchain.BlockIterator(state, header, txIterator)

	header.Root = common.BytesToHash(root)

	fmt.Println("__ new state root __")
	fmt.Println(header.Root.String())

	// TODO, get uncles?

	block := types.NewBlock(header, txns, nil, nil)
	s.seal(block)

	// Write the new blocks
	if err := s.blockchain.WriteBlock(block); err != nil {
		s.logger.Printf("ERR: %v", err)
	}

	// Write the new state
	s.blockchain.AddState(common.BytesToHash(root), newState)

	// TODO, broadcast the new block to the network
	select {
	case s.sealedCh <- &SealedNotify{Block: block}:
	default:
	}

	return
}

func (s *Sealer) NotifyBlock(b *types.Block) {
	select {
	case s.newBlock <- struct{}{}:
	default:
	}
}

func (s *Sealer) seal(b *types.Block) {
	/*
		if s.stopSealing != nil {
			s.stopSealing()
		}
	*/

	ctx, cancel := context.WithCancel(context.Background())
	s.stopSealing = cancel

	sealed, err := s.engine.Seal(ctx, b)
	if err != nil {
		panic(err)
	}

	fmt.Println("-- sealed --")
	fmt.Println(sealed)
}

func calcGasLimit(parent *types.Header, gasFloor, gasCeil uint64) uint64 {
	// contrib = (parentGasUsed * 3 / 2) / 1024
	contrib := (parent.GasUsed + parent.GasUsed/2) / params.GasLimitBoundDivisor

	// decay = parentGasLimit / 1024 -1
	decay := parent.GasLimit/params.GasLimitBoundDivisor - 1

	limit := parent.GasLimit - decay + contrib
	if limit < params.MinGasLimit {
		limit = params.MinGasLimit
	}
	// If we're outside our allowed gas range, we try to hone towards them
	if limit < gasFloor {
		limit = parent.GasLimit + decay
		if limit > gasFloor {
			limit = gasFloor
		}
	} else if limit > gasCeil {
		limit = parent.GasLimit - decay
		if limit < gasCeil {
			limit = gasCeil
		}
	}
	return limit
}
