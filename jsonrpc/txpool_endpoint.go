package jsonrpc

import (
	"fmt"
	"strconv"

	"github.com/0xPolygon/minimal/types"
)

// Txpool is the txpool jsonrpc endpoint
type Txpool struct {
  d *Dispatcher
}

type ContentResponse struct {
  Pending   map[types.Address]map[uint64]*gethTransaction   `json:"pending"`
  Queued    map[types.Address]map[uint64]*gethTransaction   `json:"queued"`
}

type InspectResponse struct {
  Pending   map[string]map[string]string  `json:"pending"`
  Queued    map[string]map[string]string  `json:"queued"`
}

type StatusResponse struct {
  Pending   uint64  `json:"pending"`
  Queued    uint64  `json:"queued"`
}

type gethTransaction struct {
  Nonce       argUint64      `json:"nonce"`
  GasPrice    argBig         `json:"gasPrice"`
  Gas         argUint64      `json:"gas"`
  To          *types.Address `json:"to"`
  Value       argBig         `json:"value"`
  Input       argBytes       `json:"input"`
  V           argByte        `json:"v"`
  R           argBytes       `json:"r"`
  S           argBytes       `json:"s"`
  Hash        types.Hash     `json:"hash"`
  From        types.Address  `json:"from"`
  BlockHash   types.Hash     `json:"blockHash"`
  BlockNumber interface{}    `json:"blockNumber"`
  TxIndex     interface{}    `json:"transactionIndex"`
}

func toGethTransaction(t *types.Transaction) *gethTransaction {
  if t.R == nil {
    t.R = []byte{0}
  }
  if t.S == nil {
    t.S = []byte{0}
  }

  return &gethTransaction{
    Nonce:        argUint64(t.Nonce),
    GasPrice:     argBig(*t.GasPrice),
    Gas:          argUint64(t.Gas),
    To:           t.To,
    Value:        argBig(*t.Value),
    Input:        argBytes(t.Input),
    V:            argByte(t.V),
    R:            argBytes(t.R),
    S:            argBytes(t.S),
    Hash:         t.Hash,
    From:         t.From,
    BlockHash:    types.ZeroHash,
    BlockNumber:  nil,
    TxIndex:      nil,
  }
}

/** 
 * Content - implemented according to https://geth.ethereum.org/docs/rpc/ns-txpool#txpool_content
 */
func (t *Txpool) Content() (interface{}, error) {
  pendingTxs, queuedTxs := t.d.store.GetTxs()
  pendingRpcTxns := make(map[types.Address]map[uint64]*gethTransaction)
  for address, nonces := range pendingTxs {
    pendingRpcTxns[address] = make(map[uint64]*gethTransaction)
    for nonce, tx := range nonces {
      pendingRpcTxns[address][nonce] = toGethTransaction(tx)
    }
  }
  queuedRpcTxns := make(map[types.Address]map[uint64]*gethTransaction)
  for address, nonces := range queuedTxs {
    queuedRpcTxns[address] = make(map[uint64]*gethTransaction)
    for nonce, tx := range nonces {
      queuedRpcTxns[address][nonce] = toGethTransaction(tx)
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
      pendingRpcTxns[address.String()][strconv.FormatUint(nonce, 10)] = msg
    }
  }

  queuedRpcTxns := make(map[string]map[string]string)
  for address, nonces := range queuedTxs {
    queuedRpcTxns[address.String()] = make(map[string]string)
    for nonce, tx := range nonces {
      msg := fmt.Sprintf("%d wei + %d gas x %d wei", tx.Value, tx.Gas, tx.GasPrice)
      queuedRpcTxns[address.String()][strconv.FormatUint(nonce, 10)] = msg
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