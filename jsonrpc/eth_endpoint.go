package jsonrpc

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
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

	header, err := e.GetBlockHeader(block)
	return header, err
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hashStr string, full bool) (interface{}, error) {

	hashedString := types.Hash{}
	if err := hashedString.UnmarshalText([]byte(hashStr)); err != nil {
		return nil, err
	}

	block, ok := e.d.store.GetBlockByHash(hashedString, full)
	if !ok {
		return nil, fmt.Errorf("unable to get block by hash %v", hashStr)
	}

	return block, nil
}

// BlockNumber returns current block number
func (e *Eth) BlockNumber() (interface{}, error) {
	h := e.d.store.Header()

	if h == nil {
		return nil, fmt.Errorf("header has a nil value")
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
	if err := tx.UnmarshalRLP(raw.Bytes()); err != nil {
		return nil, err
	}
	tx.ComputeHash()

	if err := e.d.store.AddTx(tx); err != nil {
		return nil, err
	}
	return tx.Hash.String(), nil
}

// SendTransaction creates new message call transaction or a contract creation, if the data field contains code.
func (e *Eth) SendTransaction(params map[string]interface{}) (interface{}, error) {
	var err error

	fmt.Println("-- params --")
	fmt.Println(params)

	txn := &types.Transaction{}
	txn.From = types.StringToAddress(params["from"].(string))

	to := types.StringToAddress(params["to"].(string))
	txn.To = &to

	//input := hex.MustDecodeHex(params["data"].(string))
	gasPrice := hex.MustDecodeHex(params["gasPrice"].(string))

	gas := params["gas"].(string)
	if value, ok := params["value"]; ok {
		fmt.Println("-- value --")
		fmt.Println(value)

		txn.Value = hex.MustDecodeHex(value.(string))
	}

	if nonce, ok := params["nonce"]; ok {
		nonceString := nonce.(string)
		txn.Nonce, err = types.ParseUint64orHex(&nonceString)
		if err != nil {
			panic(err)
		}
	}

	// txn.Input = input
	txn.GasPrice = gasPrice
	txn.Gas, err = types.ParseUint64orHex(&gas)
	if err != nil {
		panic(err)
	}
	txn.ComputeHash()

	if err := e.d.store.AddTx(txn); err != nil {
		return nil, err
	}
	return txn.Hash.String(), nil
}

// GetTransactionReceipt returns account nonce
func (e *Eth) GetTransactionReceipt(hash string) (interface{}, error) {
	hashedString := types.Hash{}
	if err := hashedString.UnmarshalText([]byte(hash)); err != nil {
		return nil, err
	}

	header, err := e.GetBlockHeader(LatestBlockNumber)
	if err != nil {
		return nil, err
	}

	receipts, err := e.d.store.GetReceiptsByHash(header.ReceiptsRoot)

	// TODO find a more optimal solution
	for _, receipt := range receipts {
		if receipt.TxHash == hashedString {
			return receipt, nil
		}
	}

	return nil, nil
}

// GetStorageAt returns the contract storage at the index position
func (e *Eth) GetStorageAt(address string, index types.Hash, number BlockNumber) (interface{}, error) {

	addr := types.StringToAddress(address)

	// Fetch the requested header
	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
	}

	// Get the storage for the passed in location
	result, err := e.d.store.GetStorage(header.StateRoot, addr, index)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GasPrice returns the average gas price based on the last x blocks
func (e *Eth) GasPrice() (interface{}, error) {

	// Grab the average gas price and convert it to a hex value
	avgGasPrice := hex.EncodeBig(e.d.store.GetAvgGasPrice())

	return avgGasPrice, nil
}

// Call executes a smart contract call using the transaction object data
func (e *Eth) Call(transaction *types.Transaction, number BlockNumber) (interface{}, error) {

	// Fetch the requested header
	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
	}

	// The return value of the execution is saved in the transition (returnValue field)
	returnValue, failed, err := e.d.store.ApplyTxn(header, transaction)

	if err != nil {
		return nil, err
	}

	if failed {
		return nil, fmt.Errorf("unable to execute call")
	}

	return returnValue, nil
}

// GetBlockHeader returns the specific header of the requested block
// latest, earliest, pending, or a specific block number
// See codec.go for specific default values
func (e *Eth) GetBlockHeader(number BlockNumber) (*types.Header, error) {
	switch number {
	case LatestBlockNumber:
		return e.d.store.Header(), nil

	case EarliestBlockNumber:
		return nil, fmt.Errorf("fetching the earliest header is not supported")

	case PendingBlockNumber:
		return nil, fmt.Errorf("fetching the pending header is not supported")

	default:
		// Convert the block number from hex to uint64
		header, ok := e.d.store.GetHeaderByNumber(uint64(number))
		if !ok {
			return nil, fmt.Errorf("Error fetching block number %d header", uint64(number))
		}
		return header, nil
	}
}

// Checks if the default value for the block number should be used (latest)
func resolveBlockNumber(number *BlockNumber) BlockNumber {
	currentNumber := LatestBlockNumber

	if number != nil {
		currentNumber = *number
	}
	return currentNumber
}

// EstimateGas estimates the gas needed to execute a transaction
func (e *Eth) EstimateGas(transaction *types.Transaction, number *BlockNumber) (interface{}, error) {

	const standardGas uint64 = 21000

	currentNumber := resolveBlockNumber(number)

	// Fetch the requested header
	header, err := e.GetBlockHeader(currentNumber)
	if err != nil {
		return nil, err
	}

	var (
		lowEnd  = standardGas
		highEnd uint64
		gasCap  uint64
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
		acc, err := e.d.store.GetAccount(header.StateRoot, transaction.From)
		if err != nil {
			return nil, err
		}

		var accountBalance *big.Int
		accountBalance = acc.Balance

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

	gasCap = highEnd

	// Run the transaction with the estimated gas
	testTransaction := func(gas uint64) (bool, error) {
		// Create a dummy transaction with the new gas
		txn := transaction.Copy()
		txn.Gas = gas

		_, failed, err := e.d.store.ApplyTxn(header, txn)
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
	if highEnd == gasCap {
		failed, err := testTransaction(gasCap)

		if err != nil {
			return 0, err
		}

		if failed {
			return 0, fmt.Errorf("gas required exceeds allowance (%d)", gasCap)
		}
	}

	return hex.EncodeUint64(highEnd), nil
}

// GetLogs returns an array of logs matching the filter options
func (e *Eth) GetLogs(filterOptions *LogFilter) ([]*types.Log, error) {

	var referenceFrom uint64
	var referenceTo uint64

	if filterOptions.fromBlock > filterOptions.toBlock {
		return nil, fmt.Errorf("invalid block search range")
	}

	// Fetch the requested from header
	header, err := e.GetBlockHeader(filterOptions.fromBlock)
	if err != nil {
		return nil, err
	}

	referenceFrom = header.Number

	// Fetch the requested to header
	header, err = e.GetBlockHeader(filterOptions.toBlock)
	if err != nil {
		return nil, err
	}

	referenceTo = header.Number

	var result []*types.Log

	for i := referenceFrom; i < referenceTo; i++ {

		body, ok := e.d.store.GetHeaderByNumber(i)
		if !ok {
			return nil, fmt.Errorf("Error fetching header for block %d", i)
		}

		receipts, err := e.d.store.GetReceiptsByHash(body.ReceiptsRoot)
		if err != nil {
			return nil, err
		}

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

// GetBalance returns the account's balance at the referenced block
func (e *Eth) GetBalance(address string, number BlockNumber) (interface{}, error) {
	addr := types.StringToAddress(address)

	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
	}

	acc, err := e.d.store.GetAccount(header.StateRoot, addr)
	if err != nil {
		return nil, err
	}
	res := types.EncodeBigInt(acc.Balance)
	return &res, nil
}

// GetTransactionCount returns account nonce
func (e *Eth) GetTransactionCount(address string, number BlockNumber) (interface{}, error) {
	addr := types.StringToAddress(address)

	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
	}

	acc, err := e.d.store.GetAccount(header.StateRoot, addr)
	if err != nil {
		return "0x0", nil
	}

	return types.Uint64(acc.Nonce), nil
}

// GetCode returns account code at given block number
func (e *Eth) GetCode(address string, number BlockNumber) (interface{}, error) {
	addr := types.StringToAddress(address)

	header, err := e.GetBlockHeader(number)
	if err != nil {
		return nil, err
	}

	acc, err := e.d.store.GetAccount(header.StateRoot, addr)
	if err != nil {
		return nil, err
	}

	code, err := e.d.store.GetCode(types.BytesToHash(acc.CodeHash))
	if err != nil {
		return "0x", fmt.Errorf("unable to fetch account code: %v", err)
	}

	return types.HexBytes(code), nil
}

// NewFilter creates a filter object, based on filter options, to notify when the state changes (logs).
func (e *Eth) NewFilter(filter *LogFilter) (interface{}, error) {
	return e.d.filterManager.NewLogFilter(filter, nil), nil
}

// NewBlockFilter creates a filter in the node, to notify when a new block arrives
func (e *Eth) NewBlockFilter() (interface{}, error) {
	return e.d.filterManager.NewBlockFilter(nil), nil
}

// GetFilterChanges is a polling method for a filter, which returns an array of logs which occurred since last poll.
func (e *Eth) GetFilterChanges(id string) (interface{}, error) {
	return nil, nil
}

// UninstallFilter uninstalls a filter with given ID
func (e *Eth) UninstallFilter(id string) (bool, error) {
	ok := e.d.filterManager.Uninstall(id)
	return ok, nil
}

// Unsubscribe uninstalls a filter in a websocket
func (e *Eth) Unsubscribe(id string) (bool, error) {
	ok := e.d.filterManager.Uninstall(id)
	return ok, nil
}
