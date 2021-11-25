package dev

import (
	"context"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/consensus"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/txpool"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
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
	params *consensus.ConsensusParams,
) (consensus.Consensus, error) {
	logger := params.Logger.Named("dev")

	d := &Dev{
		logger:     logger,
		notifyCh:   make(chan struct{}),
		closeCh:    make(chan struct{}),
		blockchain: params.Blockchain,
		executor:   params.Executor,
		txpool:     params.Txpool,
	}

	rawInterval, ok := params.Config.Config["interval"]
	if ok {
		interval, ok := rawInterval.(uint64)
		if !ok {
			return nil, fmt.Errorf("interval expected int")
		}
		d.interval = interval
	}

	// enable dev mode so that we can accept non-signed txns
	params.Txpool.EnableDev()
	params.Txpool.NotifyCh = d.notifyCh

	return d, nil
}

// Start starts the consensus mechanism
func (d *Dev) Start() error {
	go d.run()

	return nil
}

func (d *Dev) nextNotify() chan struct{} {
	if d.interval != 0 {
		ch := make(chan struct{})
		go func() {
			<-time.After(time.Duration(d.interval) * time.Second)
			ch <- struct{}{}
		}()

		return ch
	}

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

	txns := []*types.Transaction{}
	for {
		// Add transactions to the list until there are none left
		txn, retFn := d.txpool.Pop()

		if txn == nil {
			break
		}

		if txn.ExceedsBlockGasLimit(gasLimit) {
			d.logger.Error(fmt.Sprintf("failed to write transaction: %v", state.ErrBlockLimitExceeded))
			d.txpool.DecreaseAccountNonce(txn)
		} else {
			// Execute the state transition
			if err := transition.Write(txn); err != nil {
				if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable {
					retFn()
				} else {
					d.txpool.DecreaseAccountNonce(txn)
				}

				break
			}

			txns = append(txns, txn)
		}
	}

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

	// Write the block to the blockchain
	if err := d.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
		return err
	}

	return nil
}

// REQUIRED BASE INTERFACE METHODS //

func (d *Dev) VerifyHeader(parent *types.Header, header *types.Header) error {
	// All blocks are valid
	return nil
}

func (d *Dev) GetBlockCreator(header *types.Header) (types.Address, error) {
	return header.Miner, nil
}

func (d *Dev) Prepare(header *types.Header) error {
	// TODO: Remove
	return nil
}

func (d *Dev) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	// TODO: Remove
	return nil, nil
}

func (d *Dev) Close() error {
	close(d.closeCh)
	return nil
}
