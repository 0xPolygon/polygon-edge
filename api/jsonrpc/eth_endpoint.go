package jsonrpc

import (
	"fmt"
	"math/big"
	"strconv"

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

// BlockNumber returns current block number
func (e *Eth) BlockNumber() (interface{}, error) {
	h, ok := e.d.minimal.Blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("error fetching current block")
	}

	return types.Uint64(h.Number), nil
}

// SendRawTransaction sends a raw transaction
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

// GetBalance returns the account's balance
func (e *Eth) GetBalance(address string, number string) (interface{}, error) {
	addr := types.StringToAddress(address)

	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
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
	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
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

// GasPrice returns the average gas price based on the last x blocks
func (e *Eth) GasPrice() (interface{}, error) {

	// Grab the average gas price and convert it to a hex value
	avgGasPrice := hex.EncodeBig(e.d.minimal.Blockchain.GetAvgGasPrice())

	return avgGasPrice, nil
}

// Call executes a smart contract call using the transaction object data
func (e *Eth) Call(transaction *types.Transaction, number string) (interface{}, error) {

	// Fetch the requested header
	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
	}

	transition, err := e.d.minimal.Blockchain.Executor().BeginTxn(header.StateRoot, header)
	if err != nil {
		return nil, err
	}

	// The return value of the execution is saved in the transition (returnValue field)
	_, _, err = transition.Apply(transaction)
	if err != nil {
		return nil, err
	}

	return transition.ReturnValue(), nil
}

// GetBlockHeader returns the specific header of the requested block
// latest, earliest, pending, or a specific block number
func (e *Eth) GetBlockHeader(number string) (*types.Header, error) {

	blockchainRef := e.d.minimal.Blockchain

	blockRef := "latest" // default value

	if number != "" {
		blockRef = number
	}

	switch blockRef {
	case "latest":
		header, ok := blockchainRef.Header()
		if !ok {
			return nil, fmt.Errorf("Error fetching latest header")
		}

		return header, nil
	case "earliest":
		return nil, fmt.Errorf("Fetching the earliest header is not supported")
	case "pending":
		return nil, fmt.Errorf("Fetching the pending header is not supported")
	default:
		// Convert the block number from hex to uint64
		blockNumber := hex.DecodeHexToBig(number)

		header, ok := blockchainRef.GetHeaderByNumber(blockNumber.Uint64())
		if !ok {
			return nil, fmt.Errorf("Error fetching block number %d header", blockNumber.Uint64())
		}
		return header, nil
	}
}

// EstimateGasParams - Optional params used for the EstimateGas call
type EstimateGasParams struct {
	transaction *types.Transaction
	number      string
}

// EstimateGas estimates the gas needed to execute a transaction
func (e *Eth) EstimateGas(params EstimateGasParams) (interface{}, error) {

	const standardGas uint64 = 21000

	// Fetch the requested header
	header, err := e.GetBlockHeader(params.number)
	if err != nil {
		return nil, err
	}

	transaction := params.transaction

	var (
		lowEnd  uint64 = standardGas
		highEnd uint64
		cap     uint64
	)

	// If the gas limit was passed in, use it as a ceiling
	if transaction.Gas != 0 && uint64(transaction.Gas) >= standardGas {
		highEnd = uint64(transaction.Gas)
	} else {
		// If not, use the referenced block number
		highEnd = header.GasLimit
	}

	gasPriceInt := hex.DecodeHexToBig(transaction.GasPrice.String())
	valueInt := hex.DecodeHexToBig(transaction.Value.String())

	// If the sender address is present, recalculate the ceiling to his balance
	if transaction.GasPrice != nil && gasPriceInt.BitLen() != 0 {

		// Get the account balance
		s := e.d.minimal.Blockchain.Executor().State()
		snap, err := s.NewSnapshotAt(header.StateRoot)

		var accountBalance *big.Int

		if err != nil {
			return nil, err
		}

		if acc, ok := state.NewTxn(s, snap).GetAccount(transaction.From); ok {
			accountBalance = acc.Balance
		}

		if err != nil {
			return 0, err
		}

		available := new(big.Int).Set(accountBalance)

		if transaction.Value != nil {
			if valueInt.Cmp(available) >= 0 {
				return 0, fmt.Errorf("insufficient funds for transfer")
			}

			available.Sub(available, valueInt)
		}

		allowance := new(big.Int).Div(available, gasPriceInt)

		// If the allowance is larger than maximum uint64, skip checking
		if allowance.IsUint64() && highEnd > allowance.Uint64() {
			highEnd = allowance.Uint64()
		}
	}

	if highEnd > types.GasCap.Uint64() {
		// The high end is greater than the environment gas cap
		highEnd = types.GasCap.Uint64()
	}

	cap = highEnd

	// Run the transaction with the estimated gas
	testTransaction := func(gas uint64) (bool, error) {

		transition, err := e.d.minimal.Blockchain.Executor().BeginTxn(header.StateRoot, header)
		if err != nil {
			return true, err
		}

		// Create a dummy transaction with the new gas
		txn := &types.Transaction{}
		txn.Nonce = transaction.Nonce
		txn.GasPrice = transaction.GasPrice
		txn.Value = transaction.Value
		txn.Input = transaction.Input
		txn.V = transaction.V
		txn.R = transaction.R
		txn.S = transaction.S
		txn.Hash = transaction.Hash
		txn.From = transaction.From
		txn.To = transaction.To

		txn.Gas = gas

		_, failed, err := transition.Apply(transaction)
		if err != nil {
			return failed, err
		}

		return failed, nil
	}

	// Start the binary search for the lowest possible gas price
	for lowEnd <= highEnd {
		mid := (lowEnd + highEnd) / 2

		failed, err := testTransaction(mid)

		if err != nil {
			return 0, err
		}

		if failed {
			// If the transaction failed => increase the gas
			lowEnd = mid + 1
		} else {
			// If the transaction didn't fail => lower the gas
			highEnd = mid - 1
		}
	}

	// Check the edge case if even the highest cap is not enough to complete the transaction
	if highEnd == cap {
		failed, err := testTransaction(cap)

		if err != nil {
			return 0, err
		}

		if failed {
			return 0, fmt.Errorf("gas required exceeds allowance (%d)", cap)
		}
	}

	return hex.EncodeUint64(highEnd), nil
}

// GetLogs returns an array of logs matching the filter options
func (e *Eth) GetLogs(filterOptions LogFilter) ([]*types.Log, error) {

	var referenceFrom uint64
	var referenceTo uint64

	// Fetch the requested from header
	header, err := e.GetBlockHeader(strconv.FormatUint((uint64)(filterOptions.fromBlock), 10))
	if err != nil {
		return nil, err
	}

	referenceFrom = header.Number

	// Fetch the requested to header
	header, err = e.GetBlockHeader(strconv.FormatUint((uint64)(filterOptions.toBlock), 10))
	if err != nil {
		return nil, err
	}

	referenceTo = header.Number

	var result []*types.Log

	// TODO Refactor this to use blooms

	for i := referenceFrom; i < referenceTo; i++ {

		body, err := e.d.minimal.Blockchain.GetHeaderByNumber(i)
		if err {
			return nil, fmt.Errorf("Error fetching header for block %d", i)
		}

		receipts := e.d.minimal.Blockchain.GetReceiptsByHash(body.ReceiptsRoot)

		for _, receipt := range receipts {
			for _, log := range receipt.Logs {
				if filterOptions.Match(log) {
					result = append(result, log)
				}
			}
		}
	}

	return result, nil
}

// GetTransactionCount returns account nonce
func (e *Eth) GetTransactionCount(address string, number string) (interface{}, error) {
	addr := types.StringToAddress(address)

	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
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

	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
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
