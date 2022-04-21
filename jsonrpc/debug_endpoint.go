package jsonrpc

import (
	"encoding/json"
	"fmt"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
)

// Debug is the debug jsonrpc endpoint
type Debug struct {
	store ethStore
}

type ExecutionResult struct {
	Gas         uint64             `json:"gas"`
	Failed      bool               `json:"failed"`
	ReturnValue string             `json:"returnValue"`
	StructLogs  []evm.StructLogRes `json:"structLogs"`
}

// TraceTransaction returns the version of the web3 client (web3_clientVersion)
func (d *Debug) TraceTransaction(hash types.Hash, config types.LoggerConfig) (interface{}, error) {
	// findSealedTx is a helper method for checking the world state
	// for the transaction with the provided hash
	findSealedTx := func() (*types.Transaction, *types.Block) {
		// Check the chain state for the transaction
		blockHash, ok := d.store.ReadTxLookup(hash)
		if !ok {
			// Block not found in storage
			return nil, nil
		}

		block, ok := d.store.GetBlockByHash(blockHash, true)

		preBlock, ok := d.store.GetBlockByNumber(block.Number()-1, false)

		if !ok {
			// Block receipts not found in storage
			return nil, nil
		}

		// Find the transaction within the block
		for _, txn := range block.Transactions {
			if txn.Hash == hash {
				return txn, preBlock
			}
		}

		return nil, nil
	}

	msg, block := findSealedTx()

	if msg == nil {
		return nil, fmt.Errorf("hash not found")
	}

	traceTransaction := func(gas uint64) (*ExecutionResult, error) {
		// Create a dummy transaction with the new gas
		txn := msg.Copy()
		txn.Gas = gas
		txn.SetLoggerConfig(&config)
		result, err := d.store.ApplyTxn(block.Header, txn)
		if err != nil {
			return nil, err
		}

		logs, _err := result.Tracer.FormatLogs()

		if _err != nil {
			return nil, err
		}

		var logsRes []evm.StructLogRes

		json.Unmarshal(logs, &logsRes)

		return &ExecutionResult{
			Gas:         result.GasUsed,
			Failed:      result.Failed(),
			ReturnValue: types.BytesToHash(result.ReturnValue).String(),
			StructLogs:  logsRes,
		}, nil
	}

	ret, err := traceTransaction(msg.Gas)

	return ret, err
}
