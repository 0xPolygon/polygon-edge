package jsonrpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/umbracle/go-web3"
)

// Eth is the eth namespace
type Eth struct {
	c *Client
}

// Eth returns the reference to the eth namespace
func (c *Client) Eth() *Eth {
	return c.endpoints.e
}

// GetCode returns the code of a contract
func (e *Eth) GetCode(addr web3.Address, block web3.BlockNumberOrHash) (string, error) {
	var res string
	if err := e.c.Call("eth_getCode", &res, addr, block.Location()); err != nil {
		return "", err
	}
	return res, nil
}

// Accounts returns a list of addresses owned by client.
func (e *Eth) Accounts() ([]web3.Address, error) {
	var out []web3.Address
	if err := e.c.Call("eth_accounts", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetStorageAt returns the value from a storage position at a given address.
func (e *Eth) GetStorageAt(addr web3.Address, slot web3.Hash, block web3.BlockNumberOrHash) (web3.Hash, error) {
	var hash web3.Hash
	err := e.c.Call("eth_getStorageAt", &hash, addr, slot, block.Location())
	return hash, err
}

// BlockNumber returns the number of most recent block.
func (e *Eth) BlockNumber() (uint64, error) {
	var out string
	if err := e.c.Call("eth_blockNumber", &out); err != nil {
		return 0, err
	}
	return parseUint64orHex(out)
}

// GetBlockByNumber returns information about a block by block number.
func (e *Eth) GetBlockByNumber(i web3.BlockNumber, full bool) (*web3.Block, error) {
	var b *web3.Block
	if err := e.c.Call("eth_getBlockByNumber", &b, i.String(), full); err != nil {
		return nil, err
	}
	return b, nil
}

// GetBlockByHash returns information about a block by hash.
func (e *Eth) GetBlockByHash(hash web3.Hash, full bool) (*web3.Block, error) {
	var b *web3.Block
	if err := e.c.Call("eth_getBlockByHash", &b, hash, full); err != nil {
		return nil, err
	}
	return b, nil
}

// GetFilterChanges returns the filter changes for log filters
func (e *Eth) GetFilterChanges(id string) ([]*web3.Log, error) {
	var raw string
	err := e.c.Call("eth_getFilterChanges", &raw, id)
	if err != nil {
		return nil, err
	}
	var res []*web3.Log
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetTransactionByHash returns a transaction by his hash
func (e *Eth) GetTransactionByHash(hash web3.Hash) (*web3.Transaction, error) {
	var txn *web3.Transaction
	err := e.c.Call("eth_getTransactionByHash", &txn, hash)
	return txn, err
}

// GetFilterChangesBlock returns the filter changes for block filters
func (e *Eth) GetFilterChangesBlock(id string) ([]web3.Hash, error) {
	var raw string
	err := e.c.Call("eth_getFilterChanges", &raw, id)
	if err != nil {
		return nil, err
	}
	var res []web3.Hash
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, err
	}
	return res, nil
}

// NewFilter creates a new log filter
func (e *Eth) NewFilter(filter *web3.LogFilter) (string, error) {
	var id string
	err := e.c.Call("eth_newFilter", &id, filter)
	return id, err
}

// NewBlockFilter creates a new block filter
func (e *Eth) NewBlockFilter() (string, error) {
	var id string
	err := e.c.Call("eth_newBlockFilter", &id, nil)
	return id, err
}

// UninstallFilter uninstalls a filter
func (e *Eth) UninstallFilter(id string) (bool, error) {
	var res bool
	err := e.c.Call("eth_uninstallFilter", &res, id)
	return res, err
}

// SendRawTransaction sends a signed transaction in rlp format.
func (e *Eth) SendRawTransaction(data []byte) (web3.Hash, error) {
	var hash web3.Hash
	hexData := "0x" + hex.EncodeToString(data)
	err := e.c.Call("eth_sendRawTransaction", &hash, hexData)
	return hash, err
}

// SendTransaction creates new message call transaction or a contract creation.
func (e *Eth) SendTransaction(txn *web3.Transaction) (web3.Hash, error) {
	var hash web3.Hash
	err := e.c.Call("eth_sendTransaction", &hash, txn)
	return hash, err
}

// GetTransactionReceipt returns the receipt of a transaction by transaction hash.
func (e *Eth) GetTransactionReceipt(hash web3.Hash) (*web3.Receipt, error) {
	var receipt *web3.Receipt
	err := e.c.Call("eth_getTransactionReceipt", &receipt, hash)
	return receipt, err
}

// GetNonce returns the nonce of the account
func (e *Eth) GetNonce(addr web3.Address, blockNumber web3.BlockNumberOrHash) (uint64, error) {
	var nonce string
	if err := e.c.Call("eth_getTransactionCount", &nonce, addr, blockNumber.Location()); err != nil {
		return 0, err
	}
	return parseUint64orHex(nonce)
}

// GetBalance returns the balance of the account of given address.
func (e *Eth) GetBalance(addr web3.Address, blockNumber web3.BlockNumberOrHash) (*big.Int, error) {
	var out string
	if err := e.c.Call("eth_getBalance", &out, addr, blockNumber.Location()); err != nil {
		return nil, err
	}
	b, ok := new(big.Int).SetString(out[2:], 16)
	if !ok {
		return nil, fmt.Errorf("failed to convert to big.int")
	}
	return b, nil
}

// GasPrice returns the current price per gas in wei.
func (e *Eth) GasPrice() (uint64, error) {
	var out string
	if err := e.c.Call("eth_gasPrice", &out); err != nil {
		return 0, err
	}
	return parseUint64orHex(out)
}

// Call executes a new message call immediately without creating a transaction on the block chain.
func (e *Eth) Call(msg *web3.CallMsg, block web3.BlockNumber) (string, error) {
	var out string
	if err := e.c.Call("eth_call", &out, msg, block.String()); err != nil {
		return "", err
	}
	return out, nil
}

// EstimateGasContract estimates the gas to deploy a contract
func (e *Eth) EstimateGasContract(bin []byte) (uint64, error) {
	var out string
	msg := map[string]interface{}{
		"data": "0x" + hex.EncodeToString(bin),
	}
	if err := e.c.Call("eth_estimateGas", &out, msg); err != nil {
		return 0, err
	}
	return parseUint64orHex(out)
}

// EstimateGas generates and returns an estimate of how much gas is necessary to allow the transaction to complete.
func (e *Eth) EstimateGas(msg *web3.CallMsg) (uint64, error) {
	var out string
	if err := e.c.Call("eth_estimateGas", &out, msg); err != nil {
		return 0, err
	}
	return parseUint64orHex(out)
}

// GetLogs returns an array of all logs matching a given filter object
func (e *Eth) GetLogs(filter *web3.LogFilter) ([]*web3.Log, error) {
	var out []*web3.Log
	if err := e.c.Call("eth_getLogs", &out, filter); err != nil {
		return nil, err
	}
	return out, nil
}

// ChainID returns the id of the chain
func (e *Eth) ChainID() (*big.Int, error) {
	var out string
	if err := e.c.Call("eth_chainId", &out); err != nil {
		return nil, err
	}
	return parseBigInt(out), nil
}
