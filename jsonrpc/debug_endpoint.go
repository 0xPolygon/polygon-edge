package jsonrpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer/calltracer"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer/structtracer"
	"github.com/0xPolygon/polygon-edge/types"
)

const callTracerName = "callTracer"

var (
	defaultTraceTimeout = 5 * time.Second

	// ErrExecutionTimeout indicates the execution was terminated due to timeout
	ErrExecutionTimeout = errors.New("execution timeout")
	// ErrTraceGenesisBlock is an error returned when tracing genesis block which can't be traced
	ErrTraceGenesisBlock = errors.New("genesis is not traceable")
	// ErrNoConfig is an error returns when config is empty
	ErrNoConfig = errors.New("missing config object")
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
	GetAccount(root types.Hash, addr types.Address) (*Account, error)
}

type debugStore interface {
	debugBlockchainStore
	debugTxPoolStore
	debugStateStore
}

// Debug is the debug jsonrpc endpoint
type Debug struct {
	store      debugStore
	throttling *Throttling
}

func NewDebug(store debugStore, requestsPerSecond uint64) *Debug {
	return &Debug{
		store:      store,
		throttling: NewThrottling(requestsPerSecond, time.Second),
	}
}

type TraceConfig struct {
	EnableMemory      bool    `json:"enableMemory"`
	DisableStack      bool    `json:"disableStack"`
	DisableStorage    bool    `json:"disableStorage"`
	EnableReturnData  bool    `json:"enableReturnData"`
	DisableStructLogs bool    `json:"disableStructLogs"`
	Timeout           *string `json:"timeout"`
	Tracer            string  `json:"tracer"`
}

func (d *Debug) TraceBlockByNumber(
	blockNumber BlockNumber,
	config *TraceConfig,
) (interface{}, error) {
	return d.throttling.AttemptRequest(
		context.Background(),
		func() (interface{}, error) {
			num, err := GetNumericBlockNumber(blockNumber, d.store)
			if err != nil {
				return nil, err
			}

			block, ok := d.store.GetBlockByNumber(num, true)
			if !ok {
				return nil, fmt.Errorf("block %d not found", num)
			}

			return d.traceBlock(block, config)
		},
	)
}

func (d *Debug) TraceBlockByHash(
	blockHash types.Hash,
	config *TraceConfig,
) (interface{}, error) {
	return d.throttling.AttemptRequest(
		context.Background(),
		func() (interface{}, error) {
			block, ok := d.store.GetBlockByHash(blockHash, true)
			if !ok {
				return nil, fmt.Errorf("block %s not found", blockHash)
			}

			return d.traceBlock(block, config)
		},
	)
}

func (d *Debug) TraceBlock(
	input string,
	config *TraceConfig,
) (interface{}, error) {
	return d.throttling.AttemptRequest(
		context.Background(),
		func() (interface{}, error) {
			blockByte, decodeErr := hex.DecodeHex(input)
			if decodeErr != nil {
				return nil, fmt.Errorf("unable to decode block, %w", decodeErr)
			}

			block := &types.Block{}
			if err := block.UnmarshalRLP(blockByte); err != nil {
				return nil, err
			}

			return d.traceBlock(block, config)
		},
	)
}

func (d *Debug) TraceTransaction(
	txHash types.Hash,
	config *TraceConfig,
) (interface{}, error) {
	return d.throttling.AttemptRequest(
		context.Background(),
		func() (interface{}, error) {
			tx, block := GetTxAndBlockByTxHash(txHash, d.store)
			if tx == nil {
				return nil, fmt.Errorf("tx %s not found", txHash.String())
			}

			if block.Number() == 0 {
				return nil, ErrTraceGenesisBlock
			}

			tracer, cancel, err := newTracer(config)
			if err != nil {
				return nil, err
			}

			defer cancel()

			return d.store.TraceTxn(block, tx.Hash, tracer)
		},
	)
}

func (d *Debug) TraceCall(
	arg *txnArgs,
	filter BlockNumberOrHash,
	config *TraceConfig,
) (interface{}, error) {
	return d.throttling.AttemptRequest(
		context.Background(),
		func() (interface{}, error) {
			header, err := GetHeaderFromBlockNumberOrHash(filter, d.store)
			if err != nil {
				return nil, ErrHeaderNotFound
			}

			tx, err := DecodeTxn(arg, header.Number, d.store, true)
			if err != nil {
				return nil, err
			}

			// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
			if tx.Gas == 0 {
				tx.Gas = header.GasLimit
			}

			tracer, cancel, err := newTracer(config)
			if err != nil {
				return nil, err
			}

			defer cancel()

			return d.store.TraceCall(tx, header, tracer)
		},
	)
}

func (d *Debug) traceBlock(
	block *types.Block,
	config *TraceConfig,
) (interface{}, error) {
	if block.Number() == 0 {
		return nil, ErrTraceGenesisBlock
	}

	tracer, cancel, err := newTracer(config)
	if err != nil {
		return nil, err
	}

	defer cancel()

	return d.store.TraceBlock(block, tracer)
}

// newTracer creates new tracer by config
func newTracer(config *TraceConfig) (
	tracer.Tracer,
	context.CancelFunc,
	error,
) {
	var (
		timeout = defaultTraceTimeout
		err     error
	)

	if config == nil {
		return nil, nil, ErrNoConfig
	}

	if config.Timeout != nil {
		if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
			return nil, nil, err
		}
	}

	var tracer tracer.Tracer

	if config.Tracer == callTracerName {
		tracer = &calltracer.CallTracer{}
	} else {
		tracer = structtracer.NewStructTracer(structtracer.Config{
			EnableMemory:     config.EnableMemory && !config.DisableStructLogs,
			EnableStack:      !config.DisableStack && !config.DisableStructLogs,
			EnableStorage:    !config.DisableStorage && !config.DisableStructLogs,
			EnableReturnData: config.EnableReturnData,
			EnableStructLogs: !config.DisableStructLogs,
		})
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)

	go func() {
		<-timeoutCtx.Done()

		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			tracer.Cancel(ErrExecutionTimeout)
		}
	}()

	// cancellation of context is done by caller
	return tracer, cancel, nil
}
