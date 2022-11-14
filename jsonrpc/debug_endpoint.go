package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
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

// getNumericBlockNumber returns block number based on current state or specified number
func (d *Debug) getNumericBlockNumber(number BlockNumber) (uint64, error) {
	switch number {
	case LatestBlockNumber:
		return d.store.Header().Number, nil

	case EarliestBlockNumber:
		return 0, nil

	case PendingBlockNumber:
		return 0, fmt.Errorf("fetching the pending header is not supported")

	default:
		if number < 0 {
			return 0, fmt.Errorf("invalid argument 0: block number larger than int64")
		}

		return uint64(number), nil
	}
}

// getBlockHeader returns a header using the provided number
func (d *Debug) getBlockHeader(number BlockNumber) (*types.Header, error) {
	switch number {
	case LatestBlockNumber:
		return d.store.Header(), nil

	case EarliestBlockNumber:
		header, ok := d.store.GetHeaderByNumber(uint64(0))
		if !ok {
			return nil, fmt.Errorf("error fetching genesis block header")
		}

		return header, nil

	case PendingBlockNumber:
		return nil, fmt.Errorf("fetching the pending header is not supported")

	default:
		// Convert the block number from hex to uint64
		header, ok := d.store.GetHeaderByNumber(uint64(number))
		if !ok {
			return nil, fmt.Errorf("error fetching block number %d header", uint64(number))
		}

		return header, nil
	}
}

// getHeaderFromBlockNumberOrHash returns a header using the provided number or hash
func (d *Debug) getHeaderFromBlockNumberOrHash(bnh *BlockNumberOrHash) (*types.Header, error) {
	var (
		header *types.Header
		err    error
	)

	if bnh.BlockNumber != nil {
		header, err = d.getBlockHeader(*bnh.BlockNumber)
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

func (d *Debug) getNextNonce(address types.Address, number BlockNumber) (uint64, error) {
	if number == PendingBlockNumber {
		// Grab the latest pending nonce from the TxPool
		//
		// If the account is not initialized in the local TxPool,
		// return the latest nonce from the world state
		res := d.store.GetNonce(address)

		return res, nil
	}

	header, err := d.getBlockHeader(number)
	if err != nil {
		return 0, err
	}

	acc, err := d.store.GetAccount(header.StateRoot, address)

	//nolint:govet
	if errors.As(err, &ErrStateNotFound) {
		// If the account doesn't exist / isn't initialized,
		// return a nonce value of 0
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return acc.Nonce, nil
}

func (d *Debug) decodeTxn(arg *txnArgs) (*types.Transaction, error) {
	// set default values
	if arg.From == nil {
		arg.From = &types.ZeroAddress
		arg.Nonce = argUintPtr(0)
	} else if arg.Nonce == nil {
		// get nonce from the pool
		nonce, err := d.getNextNonce(*arg.From, LatestBlockNumber)
		if err != nil {
			return nil, err
		}
		arg.Nonce = argUintPtr(nonce)
	}

	if arg.Value == nil {
		arg.Value = argBytesPtr([]byte{})
	}

	if arg.GasPrice == nil {
		arg.GasPrice = argBytesPtr([]byte{})
	}

	var input []byte
	if arg.Data != nil {
		input = *arg.Data
	} else if arg.Input != nil {
		input = *arg.Input
	}

	if arg.To == nil {
		if input == nil {
			return nil, fmt.Errorf("contract creation without data provided")
		}
	}

	if input == nil {
		input = []byte{}
	}

	if arg.Gas == nil {
		arg.Gas = argUintPtr(0)
	}

	txn := &types.Transaction{
		From:     *arg.From,
		Gas:      uint64(*arg.Gas),
		GasPrice: new(big.Int).SetBytes(*arg.GasPrice),
		Value:    new(big.Int).SetBytes(*arg.Value),
		Input:    input,
		Nonce:    uint64(*arg.Nonce),
	}
	if arg.To != nil {
		txn.To = arg.To
	}

	txn.ComputeHash()

	return txn, nil
}
