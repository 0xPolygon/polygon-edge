package jsonrpc

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
)

// Eth is the eth jsonrpc endpoint
type Eth struct {
	d *Dispatcher
}

// GetBlockByNumber returns information about a block by block number
func (e *Eth) GetBlockByNumber(blockNumber string, full bool) (interface{}, error) {
	block, err := stringToBlockNumber(blockNumber)
	if err != nil {
		return nil, err
	}
	if block < 0 {
		return nil, fmt.Errorf("this data cannot be provided yet")
	}

	// TODO, show full blocks
	header, _ := e.d.minimal.Blockchain.GetHeaderByNumber(uint64(block))
	return header, nil
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hashStr string, full bool) (interface{}, error) {
	return nil, nil
}

// CurrentBlock returns current block number
func (e *Eth) BlockNumber() (interface{}, error) {
	h, ok := e.d.minimal.Blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("error fetching current block")
	}

	return types.Uint64(h.Number), nil
}

func (e *Eth) SendRawTransaction(input string) (interface{}, error) {
	raw := new(types.HexBytes)
	err := raw.Scan(input)
	if err != nil {
		return nil, err
	}
	tx := &types.Transaction{}
	p := &fastrlp.Parser{}

	v, err := p.Parse(raw.Bytes())
	if err != nil {
		return nil, err
	}

	err = tx.UnmarshalRLP(p, v)
	if err != nil {
		return nil, err
	}

	signer := crypto.NewEIP155Signer(types.DeriveChainId(new(big.Int).SetBytes([]byte{tx.V})).Uint64())
	tx.From, err = signer.Sender(tx)
	if err != nil {
		return nil, err
	}

	if err := e.d.minimal.Sealer.AddTx(tx); err != nil {
		panic(err)
	}

	tx.ComputeHash()
	return tx.Hash.String(), nil
}

// SendTransaction creates new message call transaction or a contract creation, if the data field contains code.
func (e *Eth) SendTransaction(params map[string]interface{}) (interface{}, error) {
	var err error

	txn := &types.Transaction{}
	txn.From = types.StringToAddress(params["from"].(string))

	to := types.StringToAddress(params["to"].(string))
	txn.To = &to

	input := hex.MustDecodeHex(params["data"].(string))
	gasPrice := hex.MustDecodeHex(params["gasPrice"].(string))

	gas := params["gas"].(string)
	if value, ok := params["value"]; ok {
		txn.Value = hex.MustDecodeHex(value.(string))
	}

	if nonce, ok := params["nonce"]; ok {
		nonceString := nonce.(string)
		txn.Nonce, err = types.ParseUint64orHex(&nonceString)
		if err != nil {
			panic(err)
		}
	}

	txn.Input = input
	txn.GasPrice = gasPrice
	txn.Gas, err = types.ParseUint64orHex(&gas)
	if err != nil {
		panic(err)
	}

	if err := e.d.minimal.Sealer.AddTx(txn); err != nil {
		panic(err)
	}

	txn.ComputeHash()
	return txn.Hash.String(), nil
}

// GetTransactionReceipt returns account nonce
func (e *Eth) GetTransactionReceipt(hash string) (interface{}, error) {
	return nil, fmt.Errorf("transaction not found")
}

// CurrentBlock returns current block number
func (e *Eth) GetBalance(address string, number string) (interface{}, error) {
	addr := types.StringToAddress(address)
	header, ok := e.d.minimal.Blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("error getting header")
	}

	s := e.d.minimal.Blockchain.Executor().State()
	snap, err := s.NewSnapshotAt(header.StateRoot)
	if err != nil {
		return nil, err
	}

	if acc, ok := state.NewTxn(s, snap).GetAccount(addr); ok {
		return (*types.Big)(acc.Balance), nil
	}

	return new(types.Big), nil
}

// GetStorageAt returns the contract storage at the index position
func (e *Eth) GetStorageAt(address string, index []byte, number string) (interface{}, error) {

	addr := types.StringToAddress(address)

	// Fetch the requested header
	header, ok := e.d.minimal.Blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("error getting header")
	}

	// Fetch the world state snapshot
	s := e.d.minimal.Blockchain.Executor().State()
	snap, err := s.NewSnapshotAt(header.StateRoot)
	if err != nil {
		return nil, err
	}

	acc, ok := state.NewTxn(s, snap).GetAccount(addr)
	if !ok {
		return nil, fmt.Errorf("error getting account state")
	}

	// Fetch the Storage state snapshot
	snap, err = s.NewSnapshotAt(acc.Root)
	if err != nil {
		return nil, err
	}

	// Get the storage for the passed in location
	result, ok := snap.Get(index)
	if !ok {
		return nil, fmt.Errorf("error getting storage snapshot")
	}

	return result, nil
}

// GetTransactionCount returns account nonce
func (e *Eth) GetTransactionCount(address string, number string) (interface{}, error) {
	addr := types.StringToAddress(address)
	header, ok := e.d.minimal.Blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("error getting header")
	}

	s := e.d.minimal.Blockchain.Executor().State()
	snap, err := s.NewSnapshotAt(header.StateRoot)
	if err != nil {
		return nil, err
	}

	if acc, ok := state.NewTxn(s, snap).GetAccount(addr); ok {
		return types.Uint64(acc.Nonce), nil
	}

	return "0x0", nil
}

// GetCode returns account code at given block number
func (e *Eth) GetCode(address string, number string) (interface{}, error) {
	addr := types.StringToAddress(address)
	header, ok := e.d.minimal.Blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("error getting header")
	}

	s := e.d.minimal.Blockchain.Executor().State()
	snap, err := s.NewSnapshotAt(header.StateRoot)
	if err != nil {
		return nil, err
	}

	if acc, ok := state.NewTxn(s, snap).GetAccount(addr); ok {
		code, ok := snap.Get(acc.CodeHash)
		if !ok {
			return "0x", nil
		}

		return types.HexBytes(code), nil
	}

	return "0x", nil
}

// NewFilter creates a filter object, based on filter options, to notify when the state changes (logs).
func (e *Eth) NewFilter(filter *LogFilter) (interface{}, error) {
	return e.d.filterManager.NewLogFilter(filter), nil
}

// NewBlockFilter creates a filter in the node, to notify when a new block arrives
func (e *Eth) NewBlockFilter() (interface{}, error) {
	return e.d.filterManager.NewBlockFilter(), nil
}

// GetFilterChanges is a polling method for a filter, which returns an array of logs which occurred since last poll.
func (e *Eth) GetFilterChanges(id string) (interface{}, error) {
	return nil, nil
}

// UninstallFilter uninstalls a filter with given ID
func (e *Eth) UninstallFilter(id string) {
	// TODO: Not sure about the return field here but it needs one
	e.d.filterManager.Uninstall(id)
}

/*
TODO
func (e *Eth) GetLogs() {

}
*/
