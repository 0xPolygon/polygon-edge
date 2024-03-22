package jsonrpc

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/jsonrpc"
)

// EthClient is a wrapper around jsonrpc.Client
type EthClient struct {
	client *jsonrpc.Client
}

// NewEthClient creates a new EthClient
func NewEthClient(url string) (*EthClient, error) {
	client, err := jsonrpc.NewClient(url)
	if err != nil {
		return nil, err
	}

	return &EthClient{client}, nil
}

// GetCode returns the code of a contract
func (e *EthClient) GetCode(addr types.Address, block BlockNumberOrHash) (string, error) {
	var res string
	if err := e.client.Call("eth_getCode", &res, addr, block.String()); err != nil {
		return "", err
	}

	return res, nil
}

// GetStorageAt returns the value from a storage position at a given address
func (e *EthClient) GetStorageAt(addr types.Address, slot types.Hash, block BlockNumberOrHash) (types.Hash, error) {
	var hash types.Hash
	err := e.client.Call("eth_getStorageAt", &hash, addr, slot, block.String())

	return hash, err
}

// BlockNumber returns the number of most recent block
func (e *EthClient) BlockNumber() (uint64, error) {
	var out string
	if err := e.client.Call("eth_blockNumber", &out); err != nil {
		return 0, err
	}

	return common.ParseUint64orHex(&out)
}

// GetBlockByNumber returns information about a block by block number.
func (e *EthClient) GetBlockByNumber(i BlockNumber, full bool) (*types.Block, error) {
	var b *types.Block
	if err := e.client.Call("eth_getBlockByNumber", &b, i.String(), full); err != nil {
		return nil, err
	}

	return b, nil
}

// GetBlockByHash returns information about a block by hash.
func (e *EthClient) GetBlockByHash(hash types.Hash, full bool) (*types.Block, error) {
	var b *types.Block
	if err := e.client.Call("eth_getBlockByHash", &b, hash, full); err != nil {
		return nil, err
	}

	return b, nil
}

// GetTransactionByHash returns a transaction by hash
func (e *EthClient) GetTransactionByHash(hash types.Hash) (*types.Transaction, error) {
	var txn *types.Transaction
	err := e.client.Call("eth_getTransactionByHash", &txn, hash)

	return txn, err
}

// SendRawTransaction sends a signed transaction in rlp format
func (e *EthClient) SendRawTransaction(data []byte) (types.Hash, error) {
	var hash types.Hash

	hexData := "0x" + hex.EncodeToString(data)
	err := e.client.Call("eth_sendRawTransaction", &hash, hexData)

	return hash, err
}

// SendTransaction creates new message call transaction or a contract creation
func (e *EthClient) SendTransaction(txn *types.Transaction) (types.Hash, error) {
	var hash types.Hash
	err := e.client.Call("eth_sendTransaction", &hash, txn)

	return hash, err
}

// GetTransactionReceipt returns the receipt of a transaction by transaction hash
func (e *EthClient) GetTransactionReceipt(hash types.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := e.client.Call("eth_getTransactionReceipt", &receipt, hash)

	return receipt, err
}

// GetNonce returns the nonce of the account
func (e *EthClient) GetNonce(addr types.Address, blockNumber BlockNumberOrHash) (uint64, error) {
	var nonce string
	if err := e.client.Call("eth_getTransactionCount", &nonce, addr, blockNumber.String()); err != nil {
		return 0, err
	}

	return common.ParseUint64orHex(&nonce)
}

// GetBalance returns the balance of the account of given address
func (e *EthClient) GetBalance(addr types.Address, blockNumber BlockNumberOrHash) (*big.Int, error) {
	var out string
	if err := e.client.Call("eth_getBalance", &out, addr, blockNumber.String()); err != nil {
		return nil, err
	}

	return common.ParseUint256orHex(&out)
}

// GasPrice returns the current price per gas in wei
func (e *EthClient) GasPrice() (uint64, error) {
	var out string
	if err := e.client.Call("eth_gasPrice", &out); err != nil {
		return 0, err
	}

	return common.ParseUint64orHex(&out)
}

// Call executes a new message call immediately without creating a transaction on the blockchain
func (e *EthClient) Call(msg *CallMsg, block BlockNumber, override *StateOverride) (string, error) {
	var out string
	if err := e.client.Call("eth_call", &out, msg, block.String(), override); err != nil {
		return "", err
	}

	return out, nil
}

// EstimateGas generates and returns an estimate of how much gas is necessary to allow the transaction to complete
func (e *EthClient) EstimateGas(msg *CallMsg) (uint64, error) {
	var out string
	if err := e.client.Call("eth_estimateGas", &out, msg); err != nil {
		return 0, err
	}

	return common.ParseUint64orHex(&out)
}

// ChainID returns the id of the chain
func (e *EthClient) ChainID() (*big.Int, error) {
	var out string
	if err := e.client.Call("eth_chainId", &out); err != nil {
		return nil, err
	}

	return common.ParseUint256orHex(&out)
}

// MaxPriorityFeePerGas returns a fee per gas that is an estimate of how much you can pay as a priority fee, or 'tip',
// to get a transaction included in the current block (EIP-1559)
func (e *EthClient) MaxPriorityFeePerGas() (*big.Int, error) {
	var out string
	if err := e.client.Call("eth_maxPriorityFeePerGas", &out); err != nil {
		return big.NewInt(0), err
	}

	return common.ParseUint256orHex(&out)
}
