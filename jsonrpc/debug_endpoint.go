package jsonrpc

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
)

type debugBlockchainStore interface {
	// ReadTxLookup returns a block hash in which a given txn was mined
	ReadTxLookup(txnHash types.Hash) (types.Hash, bool)

	// GetBlockByHash gets a block using the provided hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	TraceSealedTxn(*types.Block, types.Hash, runtime.Tracer) (*runtime.ExecutionResult, error)
}

type debugStore interface {
	debugBlockchainStore
}

// Debug is the debug jsonrpc endpoint
type Debug struct {
	store debugStore
}

type TraceConfig struct {
	tracer.Config
	// Tracer  *string
	// Timeout *string
	// Reexec  *uint64
}

func (d *Debug) TraceTransaction(
	txHash types.Hash,
	config *TraceConfig,
) (interface{}, error) {
	tx, block := d.getTxAndBlockByTxHash(txHash)
	if tx == nil {
		// TODO: check error schema
		return nil, fmt.Errorf("Tx %s not found", txHash.String())
	}

	if block.Number() == 0 {
		return nil, errors.New("genesis is not traceable")
	}

	tracer := tracer.NewStructTracer()

	_, err := d.store.TraceSealedTxn(block, tx.Hash, tracer)
	if err != nil {
		return nil, err
	}

	return tracer.GetResult(), nil
}

func (d *Debug) getTxAndBlockByTxHash(txHash types.Hash) (*types.Transaction, *types.Block) {
	blockHash, ok := d.store.ReadTxLookup(txHash)
	if !ok {
		return nil, nil
	}

	block, ok := d.store.GetBlockByHash(blockHash, true)
	if !ok {
		return nil, nil
	}

	for _, txn := range block.Transactions {
		if txn.Hash == txHash {
			return txn, block
		}
	}

	return nil, nil
}