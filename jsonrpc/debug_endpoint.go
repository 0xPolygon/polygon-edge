package jsonrpc

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
)

type debugBlockchainStore interface {
	// ReadTxLookup returns a block hash in which a given txn was mined
	ReadTxLookup(txnHash types.Hash) (types.Hash, bool)

	// GetBlockByHash gets a block using the provided hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	// GetBlockByNumber gets a block using the provided height
	GetBlockByNumber(num uint64, full bool) (*types.Block, bool)

	TraceMinedBlock(*types.Block, runtime.Tracer) ([]interface{}, error)

	TraceMinedTxn(*types.Block, types.Hash, runtime.Tracer) (interface{}, error)

	TraceCall(*types.Transaction, *types.Header, runtime.Tracer) (interface{}, error)
}

type debugTxPoolStore interface {
	GetNonce(types.Address) uint64
}

type debugStateStore interface {
	GetAccount(root types.Hash, addr types.Address) (*state.Account, error)
}

type debugStore interface {
	debugBlockchainStore
	debugTxPoolStore
	debugStateStore
}

// Debug is the debug jsonrpc endpoint
type Debug struct {
	*endpointHelper

	store debugStore
}

type TraceConfig struct {
	tracer.Config
	// Tracer  *string
	// Timeout *string
	// Reexec  *uint64
}

func (d *Debug) TraceBlockByNumber(
	blockNumber BlockNumber,
	config *TraceConfig,
) (interface{}, error) {
	num, err := d.getNumericBlockNumber(blockNumber)
	if err != nil {
		return nil, err
	}

	block, ok := d.store.GetBlockByNumber(num, true)
	if !ok {
		return nil, errors.New("block not found")
	}

	return d.traceBlock(block, config)
}

func (d *Debug) TraceBlockByHash(
	blockHash types.Hash,
	config *TraceConfig,
) (interface{}, error) {
	block, ok := d.store.GetBlockByHash(blockHash, true)
	if !ok {
		return nil, errors.New("block not found")
	}

	return d.traceBlock(block, config)
}

func (d *Debug) TraceBlock(
	input string,
	config *TraceConfig,
) (interface{}, error) {
	blockByte, decodeErr := hex.DecodeHex(input)
	if decodeErr != nil {
		return nil, fmt.Errorf("unable to decode block, %w", decodeErr)
	}

	block := &types.Block{}
	if err := block.UnmarshalRLP(blockByte); err != nil {
		return nil, err
	}

	return d.traceBlock(block, config)
}

func (d *Debug) TraceTransaction(
	txHash types.Hash,
	config *TraceConfig,
) (interface{}, error) {
	tx, block := d.getTxAndBlockByTxHash(txHash)
	if tx == nil {
		// TODO: check error schema
		return nil, fmt.Errorf("tx %s not found", txHash.String())
	}

	if block.Number() == 0 {
		return nil, errors.New("genesis is not traceable")
	}

	tracer := tracer.NewStructTracer()

	return d.store.TraceMinedTxn(block, tx.Hash, tracer)
}

func (d *Debug) TraceCall(
	arg *txnArgs,
	filter BlockNumberOrHash,
	config *TraceConfig,
) (interface{}, error) {
	var (
		header *types.Header
		err    error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err = d.getHeaderFromBlockNumberOrHash(&filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	tx, err := d.decodeTxn(arg)
	if err != nil {
		return nil, err
	}

	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if tx.Gas == 0 {
		tx.Gas = header.GasLimit
	}

	tracer := tracer.NewStructTracer()

	return d.store.TraceCall(tx, header, tracer)
}

func (d *Debug) traceBlock(
	block *types.Block,
	config *TraceConfig,
) (interface{}, error) {
	if block.Number() == 0 {
		return nil, errors.New("genesis is not traceable")
	}

	tracer := tracer.NewStructTracer()

	return d.store.TraceMinedBlock(block, tracer)
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
