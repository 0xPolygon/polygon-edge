package dev

import (
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	devConsensus = "dev-consensus"
)

// Dev consensus protocol seals any new transaction immediately
type Dev struct {
	logger hclog.Logger

	notifyCh chan struct{}
	closeCh  chan struct{}

	interval uint64
	txpool   *txpool.TxPool

	blockchain *blockchain.Blockchain
	executor   *state.Executor
}

// Factory implements the base factory method
func Factory(
	params *consensus.Params,
) (consensus.Consensus, error) {
	logger := params.Logger.Named("dev")

	d := &Dev{
		logger:     logger,
		notifyCh:   make(chan struct{}),
		closeCh:    make(chan struct{}),
		blockchain: params.Blockchain,
		executor:   params.Executor,
		txpool:     params.TxPool,
	}

	rawInterval, ok := params.Config.Config["interval"]
	if ok {
		interval, ok := rawInterval.(uint64)
		if !ok {
			return nil, fmt.Errorf("interval expected int")
		}

		d.interval = interval
	}

	return d, nil
}

// Initialize initializes the consensus
func (d *Dev) Initialize() error {
	d.txpool.SetSealing(true)

	return nil
}

// Start starts the consensus mechanism
func (d *Dev) Start() error {
	go d.run()

	return nil
}

func (d *Dev) nextNotify() chan struct{} {
	if d.interval == 0 {
		d.interval = 1
	}

	go func() {
		<-time.After(time.Duration(d.interval) * time.Second)
		d.notifyCh <- struct{}{}
	}()

	return d.notifyCh
}

func (d *Dev) run() {
	d.logger.Info("consensus started")

	for {
		// wait until there is a new txn
		select {
		case <-d.nextNotify():
		case <-d.closeCh:
			return
		}

		// There are new transactions in the pool, try to seal them
		header := d.blockchain.Header()
		if err := d.writeNewBlock(header); err != nil {
			d.logger.Error("failed to mine block", "err", err)
		}
	}
}

type transitionInterface interface {
	Write(txn *types.Transaction) error
}

func (d *Dev) writeTransactions(gasLimit uint64, transition transitionInterface) []*types.Transaction {
	var successful []*types.Transaction

	d.txpool.Prepare()

	for {
		tx := d.txpool.Peek()
		if tx == nil {
			break
		}

		if tx.ExceedsBlockGasLimit(gasLimit) {
			d.txpool.Drop(tx)

			continue
		}

		if err := transition.Write(tx); err != nil {
			if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
				break
			} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
				d.txpool.Demote(tx)
			} else {
				d.txpool.Drop(tx)
			}

			continue
		}

		// no errors, pop the tx from the pool
		d.txpool.Pop(tx)

		successful = append(successful, tx)
	}

	d.logger.Info("picked out txns from pool", "num", len(successful), "remaining", d.txpool.Length())

	return successful
}

// writeNewBLock generates a new block based on transactions from the pool,
// and writes them to the blockchain
func (d *Dev) writeNewBlock(parent *types.Header) error {
	// Generate the base block
	num := parent.Number
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     num + 1,
		GasLimit:   parent.GasLimit, // Inherit from parent for now, will need to adjust dynamically later.
		Timestamp:  uint64(time.Now().Unix()),
	}

	// calculate gas limit based on parent header
	gasLimit, err := d.blockchain.CalculateGasLimit(header.Number)
	if err != nil {
		return err
	}

	header.GasLimit = gasLimit

	miner, err := d.GetBlockCreator(header)
	if err != nil {
		return err
	}

	transition, err := d.executor.BeginTxn(parent.StateRoot, header, miner)

	if err != nil {
		return err
	}

	txns := d.writeTransactions(gasLimit, transition)

	// Commit the changes
	_, root := transition.Commit()

	// Update the header
	header.StateRoot = root
	header.GasUsed = transition.TotalGas()

	// Build the actual block
	// The header hash is computed inside buildBlock
	block := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   header,
		Txns:     txns,
		Receipts: transition.Receipts(),
	})

	if err := d.blockchain.VerifyFinalizedBlock(block); err != nil {
		return err
	}

	// Write the block to the blockchain
	if err := d.blockchain.WriteBlock(block, devConsensus); err != nil {
		return err
	}

	// after the block has been written we reset the txpool so that
	// the old transactions are removed
	d.txpool.ResetWithHeaders(block.Header)

	return nil
}

// REQUIRED BASE INTERFACE METHODS //

func (d *Dev) VerifyHeader(header *types.Header) error {
	// All blocks are valid
	return nil
}

func (d *Dev) ProcessHeaders(headers []*types.Header) error {
	return nil
}

func (d *Dev) GetBlockCreator(header *types.Header) (types.Address, error) {
	return types.BytesToAddress(header.Miner), nil
}

// PreCommitState a hook to be called before finalizing state transition on inserting block
func (d *Dev) PreCommitState(_header *types.Header, _txn *state.Transition) error {
	return nil
}

func (d *Dev) GetSyncProgression() *progress.Progression {
	return nil
}

func (d *Dev) Close() error {
	close(d.closeCh)

	return nil
}
