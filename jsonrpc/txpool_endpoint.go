package jsonrpc

import (
	"fmt"

	"github.com/0xPolygon/minimal/types"
)

// Txpool is the txpool jsonrpc endpoint
type Txpool struct {
	d *Dispatcher
}

type ContentResponse struct {
	Pending   map[types.Address]map[uint64]*transaction		`json:"pending"`
	Queued 		map[types.Address]map[uint64]*transaction		`json:"queued"`
}

type InspectResponse struct {
	Pending   map[string]map[string]string	`json:"pending"`
	Queued		map[string]map[string]string	`json:"queued"`
}

type StatusResponse struct {
	Pending   uint64	`json:"pending"`
	Queued 		uint64	`json:"queued"`
}

/** 
 * Content - implemented according to https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_content
 */
func (t *Txpool) Content() (interface{}, error) {
	pendingTxs, queuedTxs := t.d.store.GetTxs()
	pendingRpcTxns := make(map[types.Address]map[uint64]*transaction)
	for address, nonces := range pendingTxs {
		pendingRpcTxns[address] = make(map[uint64]*transaction)
		for nonce, tx := range nonces {
			// placeholder block to pass toTransaction
			mockBlock := &types.Block{
				Header: &types.Header{
					Hash: types.Hash{0x0},
					Number: uint64(0),
				},
			}
			// using 0 as txIndex
			pendingRpcTxns[address][nonce] = toTransaction(tx, mockBlock, 0)
		}
  }
	queuedRpcTxns := make(map[types.Address]map[uint64]*transaction)
	for address, nonces := range queuedTxs {
		queuedRpcTxns[address] = make(map[uint64]*transaction)
		for nonce, tx := range nonces {
			// placeholder block to pass toTransaction
			mockBlock := &types.Block{
				Header: &types.Header{
					Hash: types.Hash{0x0},
					Number: uint64(0),
				},
			}
			// using 0 as txIndex
			queuedRpcTxns[address][nonce] = toTransaction(tx, mockBlock, 0)
		}
  }

	resp := ContentResponse{
		Pending: pendingRpcTxns,
		Queued: queuedRpcTxns,
	}
	
	return resp, nil
}

/** 
 * Inspect - implemented according to https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_inspect
 */
func (t *Txpool) Inspect() (interface{}, error) {

	pendingTxs, queuedTxs := t.d.store.GetTxs()
	pendingRpcTxns := make(map[string]map[string]string)
	for address, nonces := range pendingTxs {
		pendingRpcTxns[address.String()] = make(map[string]string)
		for nonce, tx := range nonces {
			msg := fmt.Sprintf("%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice)
			pendingRpcTxns[address.String()][fmt.Sprint(nonce)] = msg
		}
  }

	queuedRpcTxns := make(map[string]map[string]string)
	for address, nonces := range queuedTxs {
		queuedRpcTxns[address.String()] = make(map[string]string)
		for nonce, tx := range nonces {
			msg := fmt.Sprintf("%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice)
			queuedRpcTxns[address.String()][fmt.Sprint(nonce)] = msg
		}
  }

	resp := InspectResponse{
		Pending: pendingRpcTxns,
		Queued: queuedRpcTxns,
	}

	return resp, nil
}

/** 
 * Status - implemented according to https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_content
 */
func (t *Txpool) Status() (interface{}, error) {
	pendingTxs, queuedTxs := t.d.store.GetTxs()
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
		Queued: uint64(queuedCount),
	}

	return resp, nil
}