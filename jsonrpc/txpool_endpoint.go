package jsonrpc

import (
	"fmt"
	"strconv"

	"github.com/0xPolygon/polygon-sdk/types"
)

// Txpool is the txpool jsonrpc endpoint
type Txpool struct {
	d *Dispatcher
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
		Input:       argBytes(t.Input),
		Hash:        t.Hash,
		From:        t.From,
		BlockHash:   types.ZeroHash,
		BlockNumber: nil,
		TxIndex:     nil,
	}
}

// Create response for txpool_content request.
// See https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_content.
func (t *Txpool) Content() (interface{}, error) {
	pendingTxs, queuedTxs := t.d.store.GetTxs(true)

	// collect pending
	pendingRpcTxns := make(map[types.Address]map[uint64]*txpoolTransaction)
	for addr, txs := range pendingTxs {
		pendingRpcTxns[addr] = map[uint64]*txpoolTransaction{}

		for _, tx := range txs {
			nonce := tx.Nonce
			rpcTx := toTxPoolTransaction(tx)

			pendingRpcTxns[addr][nonce] = rpcTx
		}
	}

	// collect enqueued
	queuedRpcTxns := make(map[types.Address]map[uint64]*txpoolTransaction)
	for addr, txs := range queuedTxs {

		queuedRpcTxns = make(map[types.Address]map[uint64]*txpoolTransaction)
		for _, tx := range txs {
			nonce := tx.Nonce
			rpcTx := toTxPoolTransaction(tx)

			queuedRpcTxns[addr][nonce] = rpcTx
		}
	}

	resp := ContentResponse{
		Pending: pendingRpcTxns,
		Queued:  queuedRpcTxns,
	}

	return resp, nil
}

// Create response for txpool_inspect request.
// See https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_inspect.
func (t *Txpool) Inspect() (interface{}, error) {
	pendingTxs, queuedTxs := t.d.store.GetTxs(true)

	// collect pending
	pendingRpcTxns := make(map[string]map[string]string)
	for addr, txs := range pendingTxs {
		pendingRpcTxns[addr.String()] = make(map[string]string)

		for _, tx := range txs {
			msg := fmt.Sprintf("%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice)
			nonceStr := strconv.FormatUint(tx.Nonce, 10)

			pendingRpcTxns[addr.String()][nonceStr] = msg
		}
	}

	// collect enqueued
	queuedRpcTxns := make(map[string]map[string]string)
	for addr, txs := range queuedTxs {
		queuedRpcTxns[addr.String()] = make(map[string]string)

		for _, tx := range txs {
			msg := fmt.Sprintf("%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice)
			nonceStr := strconv.FormatUint(tx.Nonce, 10)

			queuedRpcTxns[addr.String()][nonceStr] = msg
		}
	}

	// get capacity of the TxPool
	current, max := t.d.store.GetCapacity()

	resp := InspectResponse{
		Pending:         pendingRpcTxns,
		Queued:          queuedRpcTxns,
		CurrentCapacity: current,
		MaxCapacity:     max,
	}

	return resp, nil
}

// Create response for txpool_status request.
// See https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_status.
func (t *Txpool) Status() (interface{}, error) {
	pendingTxs, queuedTxs := t.d.store.GetTxs(true)
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
