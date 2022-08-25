package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"

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

// getNextNonce returns the next nonce for the account for the specified block
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

// TraceTransaction returns the version of the web3 client (web3_clientVersion)
func (d *Debug) TraceTransaction(hash types.Hash, config *TraceConfig) (interface{}, error) {
	findSealedTx := func() (*types.Transaction, *types.Block, *types.Block, uint64) {
		// Check the chain state for the transaction
		blockHash, ok := d.store.ReadTxLookup(hash)
		if !ok {
			// Block not found in storage
			return nil, nil, nil, 0
		}

		block, ok := d.store.GetBlockByHash(blockHash, true)
		parent, ok := d.store.GetBlockByNumber(block.Number()-1, false)

		if !ok {
			// Block receipts not found in storage
			return nil, nil, nil, 0
		}

		// Find the transaction within the block
		for txIndx, txn := range block.Transactions {
			if txn.Hash == hash {
				return txn, block, parent, uint64(txIndx)
			}
		}

		return nil, nil, nil, 0
	}
	// get transaction + block(parent)
	msg, block, parentBlock, txIndx := findSealedTx()

	if msg == nil {
		return nil, fmt.Errorf("hash not found")
	}
	// construct tracer
	txctx := &tracers.Context{
		BlockHash: block.Hash(), // ? parent or now ?
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
	_, err = d.store.ApplyMessage(parentBlock.Header, txn, runtime.TraceConfig{Debug: true, Tracer: tracer, NoBaseFee: true})

	if err != nil {
		return nil, err
	}

	return tracer.GetResult()
}

// TraceCall lets you trace a given eth_call. It collects the structured logs
// created during the execution of EVM if the given transaction was added on
// top of the provided block and returns them as a JSON object.
// You can provide -2 as a block number to trace on top of the pending block.
func (d *Debug) TraceCall(args *txnArgs, blockNrOrHash BlockNumberOrHash, config *TraceConfig) (interface{}, error) {
	var (
		header *types.Header
		err    error
	)

	// The filter is empty, use the latest block by default
	if blockNrOrHash.BlockNumber == nil && blockNrOrHash.BlockHash == nil {
		blockNrOrHash.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err = d.getHeaderFromBlockNumberOrHash(&blockNrOrHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	transaction, err := d.decodeTxn(args)

	if err != nil {
		return nil, err
	}
	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if transaction.Gas == 0 {
		transaction.Gas = header.GasLimit
	}

	var traceConfig *TraceConfig
	if config != nil {
		traceConfig = &TraceConfig{
			Config:  config.Config,
			Tracer:  config.Tracer,
			Timeout: config.Timeout,
			Reexec:  config.Reexec,
		}
	}
	if traceConfig == nil {
		traceConfig = &TraceConfig{}
	}

	var tracer tracers.Tracer
	tracer = logger.NewStructLogger(traceConfig.Config)
	if traceConfig.Tracer != nil {
		tracer, err = tracers.New(*traceConfig.Tracer, new(tracers.Context))
		if err != nil {
			return nil, err
		}
	}
	// The return value of the execution is saved in the transition (returnValue field)
	txn := transaction.Copy()
	txn.Gas = transaction.Gas
	_, err = d.store.ApplyMessage(header, txn, runtime.TraceConfig{Debug: true, Tracer: tracer, NoBaseFee: true})

	if err != nil {
		return nil, err
	}

	return tracer.GetResult()

}
