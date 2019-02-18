package sealer

import (
	"context"
	"fmt"
	"log"
	"math/big"
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
		signer:         types.NewEIP155Signer(big.NewInt(1)),
	}
	return s
}

func (s *Sealer) SetCoinbase(coinbase common.Address) {
	s.coinbase = coinbase
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

	parent := s.blockchain.Header()

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

	fmt.Println(header)

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

	fmt.Println("-- tx error --")
	fmt.Println(err)

	fmt.Println(newState)
	fmt.Println(root)

	header.Root = common.BytesToHash(root)

	// TODO, get uncles

	block := types.NewBlock(header, txns, nil, nil)
	s.seal(block)
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
