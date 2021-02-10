package sealer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"
	"github.com/hashicorp/go-hclog"
)

// Config is the sealer config
type Config struct {
	DevMode  bool
	Coinbase types.Address
	Extra    []byte
}

// DefaultConfig is the default sealer config
func DefaultConfig() *Config {
	return &Config{
		DevMode:  false,
		Coinbase: types.Address{},
		Extra:    []byte{},
	}
}

type Blockchain interface {
	Header() *types.Header
	WriteBlocks(blocks []*types.Block) error
	SubscribeEvents() blockchain.Subscription
}

// Sealer seals blocks
type Sealer struct {
	config *Config
	logger hclog.Logger

	blockchain Blockchain
	engine     consensus.Consensus // TODO; remove once the executor has more content
	txPool     *TxPool

	signer crypto.TxSigner // TODO; this should move away?

	// sealing process
	stopFn  context.CancelFunc
	lock    sync.Mutex
	enabled bool

	executor *state.Executor

	wakeCh chan struct{}
}

// NewSealer creates a new sealer for a specific engine
func NewSealer(config *Config, logger hclog.Logger, blockchain *blockchain.Blockchain, engine consensus.Consensus, executor *state.Executor) *Sealer {
	s := &Sealer{
		blockchain: blockchain,
		engine:     engine,
		config:     config,
		logger:     logger.Named("sealer"),
		txPool:     NewTxPool(blockchain),
		signer:     crypto.NewEIP155Signer(13931),
		executor:   executor,
		wakeCh:     make(chan struct{}),
	}
	return s
}

// SetEnabled enables or disables the sealer
func (s *Sealer) SetEnabled(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	wasRunning := s.enabled
	s.enabled = enabled

	if !enabled && wasRunning {
		// stop the sealer
		s.stopFn()
	} else if enabled && !wasRunning {
		s.logger.Info("Start sealing")

		// start the sealer
		ctx, cancel := context.WithCancel(context.Background())
		s.stopFn = cancel
		go s.run(ctx)
	}
}

func (s *Sealer) run(ctx context.Context) {
	//listener := s.blockchain.SubscribeEvents()
	//eventCh := listener.GetEventCh()

	for {
		if s.config.DevMode {
			// In dev-mode we wait for new transactions to seal blocks
			select {
			case <-s.wakeCh:
			case <-ctx.Done():
				return
			}
		}

		// start sealing
		subCtx, cancel := context.WithCancel(ctx)
		done := s.sealAsync(subCtx)

		// wait for the sealing to be done
		select {
		case <-done:
			// the sealing process has finished
		case <-ctx.Done():
			// the sealing routine has been canceled
			//case <-eventCh:
			// there is a new head
		}

		// cancel the sealing process context
		cancel()

		if ctx.Err() != nil {
			return
		}
	}
}

var emptyFrom = types.StringToAddress("0")

// AddTx adds a new transaction to the transaction pool
func (s *Sealer) AddTx(tx *types.Transaction) error {
	if tx.From != emptyFrom {
		if !s.config.DevMode {
			return fmt.Errorf("non signed transactions are only valid in dev mode")
		}
	}

	if err := s.txPool.Add(tx); err != nil {
		return err
	}

	// in dev mode we notify after each transaction
	if s.config.DevMode {
		select {
		case s.wakeCh <- struct{}{}:
		default:
		}
	}
	return nil
}

func generateNewBlock(header *types.Header, txs []*types.Transaction, receipts []*types.Receipt) *types.Block {
	if len(txs) == 0 {
		header.TxRoot = types.EmptyRootHash
	} else {
		header.TxRoot = buildroot.CalculateTransactionsRoot(txs)
	}

	if len(receipts) == 0 {
		header.ReceiptsRoot = types.EmptyRootHash
	} else {
		header.ReceiptsRoot = buildroot.CalculateReceiptsRoot(receipts)
	}

	// TODO: Compute uncles
	header.Sha3Uncles = types.EmptyUncleHash

	return &types.Block{
		Header:       header,
		Transactions: txs,
	}
}

func (s *Sealer) sealAsync(ctx context.Context) chan struct{} {
	ch := make(chan struct{})
	go func() {
		if err := s.seal(ctx); err != nil {
			s.logger.Trace("failed to seal", "err", err)
		}
		select {
		case ch <- struct{}{}:
		default:
		}
	}()
	return ch
}

func (s *Sealer) seal(ctx context.Context) error {
	parent := s.blockchain.Header()

	num := parent.Number
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     num + 1,
		GasLimit:   100000000, // placeholder for now
		Timestamp:  uint64(time.Now().Unix()),
		Miner:      s.config.Coinbase,
		ExtraData:  s.config.Extra,
	}

	if err := s.engine.Prepare(header); err != nil {
		return err
	}

	transition, err := s.executor.BeginTxn(parent.StateRoot, header)
	if err != nil {
		return err
	}

	/// GET THE TRANSACTIONS

	pricedTxs, err := s.txPool.sortTxns(transition.Txn(), parent)
	if err != nil {
		return err
	}

	/// PROCESS THE TRANSACTIONS

	txns := []*types.Transaction{}
	for {
		val := pricedTxs.Pop()
		if val == nil {
			break
		}

		msg := val.tx
		txns = append(txns, msg)

		if err := transition.Write(msg); err != nil {
			break
		}
		if ctx.Err() != nil {
			return nil
		}
	}

	_, root := transition.Commit()

	header.StateRoot = root
	header.GasUsed = transition.TotalGas()
	block := generateNewBlock(header, txns, transition.Receipts())

	// Start the consensus sealing
	s.logger.Debug("seal block", "num", block.Number())
	block, err = s.engine.Seal(block, ctx)
	if err != nil {
		return err
	}
	if block == nil {
		return nil
	}

	// Check if the context was cancelled while in the sealing routine
	if ctx.Err() != nil {
		return nil
	}

	// Write the new blocks
	if err := s.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
		return fmt.Errorf("failed to write sealed block: %v", err)
	}

	s.logger.Info("Block sealed", "number", num+1, "hash", block.Header.Hash)
	return nil
}

// TxDifference returns a new set which is the difference between a and b.
func txDifference(a, b []*types.Transaction) []*types.Transaction {
	keep := make([]*types.Transaction, 0, len(a))

	remove := make(map[types.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash]; !ok {
			keep = append(keep, tx)
		}
	}
	return keep
}
