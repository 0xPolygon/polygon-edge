package jsonrpc

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/tracers"
	"github.com/0xPolygon/polygon-edge/tracers/logger"
	"github.com/0xPolygon/polygon-edge/types"
)

type debugTraceStore interface {
	// add new method to handle tracer
	ApplyMessage(header *types.Header, txn *types.Transaction, tracer runtime.TraceConfig) (*runtime.ExecutionResult, error)
}

type debugStore interface {
	// inherit some methods of ethStore
	ethStore
	debugTraceStore
}

// Debug is the debug jsonrpc endpoint
type Debug struct {
	store debugStore
}

// TraceConfig holds extra parameters to trace functions.
type TraceConfig struct {
	*logger.Config
	Tracer  *string
	Timeout *string
	Reexec  *uint64
}

// TraceTransaction returns the version of the web3 client (web3_clientVersion)
func (d *Debug) TraceTransaction(hash types.Hash, config *TraceConfig) (interface{}, error) {
	findSealedTx := func() (*types.Transaction, *types.Block, uint64) {
		// Check the chain state for the transaction
		blockHash, ok := d.store.ReadTxLookup(hash)
		if !ok {
			// Block not found in storage
			return nil, nil, 0
		}

		block, ok := d.store.GetBlockByHash(blockHash, true)

		if !ok {
			// Block receipts not found in storage
			return nil, nil, 0
		}

		// Find the transaction within the block
		for txIndx, txn := range block.Transactions {
			if txn.Hash == hash {
				return txn, block, uint64(txIndx)
			}
		}

		return nil, nil, 0
	}
	// get transaction + block
	msg, block, txIndx := findSealedTx()

	if msg == nil {
		return nil, fmt.Errorf("hash not found")
	}
	// construct tracer
	txctx := &tracers.Context{
		BlockHash: block.Hash(),
		TxIndex:   int(txIndx),
		TxHash:    hash,
	}

	var tracer tracers.Tracer
	var err error
	if config == nil {
		config = &TraceConfig{}
	}
	tracer = logger.NewStructLogger(config.Config)
	if config.Tracer != nil {
		tracer, err = tracers.New(*config.Tracer, txctx)
		if err != nil {
			return nil, err
		}
	}

	txn := msg.Copy()
	txn.Gas = msg.Gas
	// is an ugly but simple way to match the mechanism of pe
	txn.Nonce = txn.Nonce + 1
	_, err = d.store.ApplyMessage(block.Header, txn, runtime.TraceConfig{Debug: true, Tracer: tracer, NoBaseFee: true})

	if err != nil {
		return nil, err
	}

	return tracer.GetResult()
}
