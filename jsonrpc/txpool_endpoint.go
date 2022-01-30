package jsonrpc

import (
	"fmt"
	"strconv"

	"github.com/0xPolygon/polygon-edge/types"
)

// txPoolStore provides access to the methods needed for txpool endpoint
type txPoolStore interface {
	// GetTxs gets tx pool transactions currently pending for inclusion and currently queued for validation
	GetTxs(inclQueued bool) (map[types.Address][]*types.Transaction, map[types.Address][]*types.Transaction)

	// GetCapacity returns the current and max capacity of the pool in slots
	GetCapacity() (uint64, uint64)
}

// TxPool is the txpool jsonrpc endpoint
type TxPool struct {
	store txPoolStore
}

type ContentResponse struct {
	Pending map[types.Address]map[uint64]*txpoolTransaction `json:"pending"`
	Queued  map[types.Address]map[uint64]*txpoolTransaction `json:"queued"`
}

type InspectResponse struct {
	Pending         map[string]map[string]string `json:"pending"`
	Queued          map[string]map[string]string `json:"queued"`
	CurrentCapacity uint64                       `json:"currentCapacity"`
	MaxCapacity     uint64                       `json:"maxCapacity"`
}

type StatusResponse struct {
	Pending uint64 `json:"pending"`
	Queued  uint64 `json:"queued"`
}

type txpoolTransaction struct {
	Nonce       argUint64      `json:"nonce"`
	GasPrice    argBig         `json:"gasPrice"`
	Gas         argUint64      `json:"gas"`
	To          *types.Address `json:"to"`
	Value       argBig         `json:"value"`
	Input       argBytes       `json:"input"`
	Hash        types.Hash     `json:"hash"`
	From        types.Address  `json:"from"`
	BlockHash   types.Hash     `json:"blockHash"`
	BlockNumber interface{}    `json:"blockNumber"`
	TxIndex     interface{}    `json:"transactionIndex"`
}

func toTxPoolTransaction(t *types.Transaction) *txpoolTransaction {
	return &txpoolTransaction{
		Nonce:       argUint64(t.Nonce),
		GasPrice:    argBig(*t.GasPrice),
		Gas:         argUint64(t.Gas),
		To:          t.To,
		Value:       argBig(*t.Value),
		Input:       t.Input,
		Hash:        t.Hash,
		From:        t.From,
		BlockHash:   types.ZeroHash,
		BlockNumber: nil,
		TxIndex:     nil,
	}
}

// Create response for txpool_content request.
// See https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_content.
func (t *TxPool) Content() (interface{}, error) {
	pendingTxs, queuedTxs := t.store.GetTxs(true)

	// collect pending
	pendingRPCTxs := make(map[types.Address]map[uint64]*txpoolTransaction)
	for addr, txs := range pendingTxs {
		pendingRPCTxs[addr] = make(map[uint64]*txpoolTransaction, len(txs))

		for _, tx := range txs {
			nonce := tx.Nonce
			rpcTx := toTxPoolTransaction(tx)

			pendingRPCTxs[addr][nonce] = rpcTx
		}
	}

	// collect enqueued
	queuedRPCTxs := make(map[types.Address]map[uint64]*txpoolTransaction)
	for addr, txs := range queuedTxs {
		queuedRPCTxs[addr] = make(map[uint64]*txpoolTransaction, len(txs))

		for _, tx := range txs {
			nonce := tx.Nonce
			rpcTx := toTxPoolTransaction(tx)

			queuedRPCTxs[addr][nonce] = rpcTx
		}
	}

	resp := ContentResponse{
		Pending: pendingRPCTxs,
		Queued:  queuedRPCTxs,
	}

	return resp, nil
}

// Create response for txpool_inspect request.
// See https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_inspect.
func (t *TxPool) Inspect() (interface{}, error) {
	pendingTxs, queuedTxs := t.store.GetTxs(true)

	// collect pending
	pendingRPCTxs := make(map[string]map[string]string)
	for addr, txs := range pendingTxs {
		pendingRPCTxs[addr.String()] = make(map[string]string, len(txs))

		for _, tx := range txs {
			nonceStr := strconv.FormatUint(tx.Nonce, 10)
			pendingRPCTxs[addr.String()][nonceStr] = fmt.Sprintf(
				"%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice,
			)
		}
	}

	// collect enqueued
	queuedRPCTxs := make(map[string]map[string]string)
	for addr, txs := range queuedTxs {
		queuedRPCTxs[addr.String()] = make(map[string]string, len(txs))

		for _, tx := range txs {
			nonceStr := strconv.FormatUint(tx.Nonce, 10)
			queuedRPCTxs[addr.String()][nonceStr] = fmt.Sprintf(
				"%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice,
			)
		}
	}

	// get capacity of the TxPool
	current, max := t.store.GetCapacity()

	resp := InspectResponse{
		Pending:         pendingRPCTxs,
		Queued:          queuedRPCTxs,
		CurrentCapacity: current,
		MaxCapacity:     max,
	}

	return resp, nil
}

// Create response for txpool_status request.
// See https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_status.
func (t *TxPool) Status() (interface{}, error) {
	pendingTxs, queuedTxs := t.store.GetTxs(true)

	var pendingCount int

	for _, t := range pendingTxs {
		pendingCount += len(t)
	}

	var queuedCount int

	for _, t := range queuedTxs {
		queuedCount += len(t)
	}

	resp := StatusResponse{
		Pending: uint64(pendingCount),
		Queued:  uint64(queuedCount),
	}

	return resp, nil
}
