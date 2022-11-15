package jsonrpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer/structtracer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	defaultTraceTimeout = 5 * time.Second

	// errExecutionTimeout indicates the execution was terminated due to timeout
	errExecutionTimeout = errors.New("execution timeout")
)

type debugBlockchainStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// GetHeaderByNumber gets a header using the provided number
	GetHeaderByNumber(uint64) (*types.Header, bool)

	// ReadTxLookup returns a block hash in which a given txn was mined
	ReadTxLookup(txnHash types.Hash) (types.Hash, bool)

	// GetBlockByHash gets a block using the provided hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	// GetBlockByNumber gets a block using the provided height
	GetBlockByNumber(num uint64, full bool) (*types.Block, bool)

	// TraceBlock traces all transactions in the given block
	TraceBlock(*types.Block, tracer.Tracer) ([]interface{}, error)

	// TraceTxn traces a transaction in the block, associated with the given hash
	TraceTxn(*types.Block, types.Hash, tracer.Tracer) (interface{}, error)

	// TraceCall traces a single call at the point when the given header is mined
	TraceCall(*types.Transaction, *types.Header, tracer.Tracer) (interface{}, error)
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
	store debugStore
}

type TraceConfig struct {
	EnableMemory     bool    `json:"enableMemory"`
	DisableStack     bool    `json:"disableStack"`
	DisableStorage   bool    `json:"disableStorage"`
	EnableReturnData bool    `json:"enableReturnData"`
	Timeout          *string `json:"timeout"`
}

func (d *Debug) TraceBlockByNumber(
	blockNumber BlockNumber,
	config *TraceConfig,
) (interface{}, error) {
	num, err := GetNumericBlockNumber(blockNumber, d.store)
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
	tx, block := GetTxAndBlockByTxHash(txHash, d.store)
	if tx == nil {
		return nil, fmt.Errorf("tx %s not found", txHash.String())
	}

	if block.Number() == 0 {
		return nil, errors.New("genesis is not traceable")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracer, err := newTracer(ctx, config)
	if err != nil {
		return nil, err
	}

	return d.store.TraceTxn(block, tx.Hash, tracer)
}

func (d *Debug) TraceCall(
	arg *txnArgs,
	filter BlockNumberOrHash,
	config *TraceConfig,
) (interface{}, error) {
	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err := d.getHeaderFromBlockNumberOrHash(&filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	tx, err := DecodeTxn(arg, d.store)
	if err != nil {
		return nil, err
	}

	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if tx.Gas == 0 {
		tx.Gas = header.GasLimit
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracer, err := newTracer(ctx, config)
	if err != nil {
		return nil, err
	}

	return d.store.TraceCall(tx, header, tracer)
}

func (d *Debug) traceBlock(
	block *types.Block,
	config *TraceConfig,
) (interface{}, error) {
	if block.Number() == 0 {
		return nil, errors.New("genesis is not traceable")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracer, err := newTracer(ctx, config)
	if err != nil {
		return nil, err
	}

	return d.store.TraceBlock(block, tracer)
}

// getHeaderFromBlockNumberOrHash returns a header using the provided number or hash
func (d *Debug) getHeaderFromBlockNumberOrHash(bnh *BlockNumberOrHash) (*types.Header, error) {
	var (
		header *types.Header
		err    error
	)

	if bnh.BlockNumber != nil {
		header, err = GetBlockHeader(*bnh.BlockNumber, d.store)
		if err != nil {
			return nil, fmt.Errorf("failed to get the header of block %d: %w", *bnh.BlockNumber, err)
		}
	} else if bnh.BlockHash != nil {
		block, ok := d.store.GetBlockByHash(*bnh.BlockHash, false)
		if !ok {
			return nil, fmt.Errorf("could not find block referenced by the hash %s", bnh.BlockHash.String())
		}

		header = block.Header
	}

	return header, nil
}

func newTracer(
	ctx context.Context,
	config *TraceConfig,
) (tracer.Tracer, error) {
	var (
		timeout = defaultTraceTimeout
		err     error
	)

	if config.Timeout != nil {
		if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
			return nil, err
		}
	}

	tracer := structtracer.NewStructTracer(structtracer.Config{
		EnableMemory:     config.EnableMemory,
		EnableStack:      !config.DisableStack,
		EnableStorage:    !config.DisableStorage,
		EnableReturnData: config.EnableReturnData,
		Limit:            0,
	})

	// cancellation of context is done by caller
	timeoutCtx, _ := context.WithTimeout(context.Background(), timeout)

	go func() {
		<-timeoutCtx.Done()

		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			tracer.Cancel(errExecutionTimeout)
		}
	}()

	return tracer, nil
}
