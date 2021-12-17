package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/fastrlp"
)

// Eth is the eth jsonrpc endpoint
type Eth struct {
	d *Dispatcher
}

// ChainId returns the chain id of the client
func (e *Eth) ChainId() (interface{}, error) {
	return argUintPtr(e.d.chainID), nil
}

func GetNumericBlockNumber(number BlockNumber, e *Eth) (uint64, error) {
	switch number {
	case LatestBlockNumber:
		return e.d.store.Header().Number, nil

	case EarliestBlockNumber:
		return 0, fmt.Errorf("fetching the earliest header is not supported")

	case PendingBlockNumber:
		return 0, fmt.Errorf("fetching the pending header is not supported")

	default:
		if number < 0 {
			return 0, fmt.Errorf("invalid argument 0: block number larger than int64")
		}
		return uint64(number), nil
	}
}

// GetBlockByNumber returns information about a block by block number
func (e *Eth) GetBlockByNumber(number BlockNumber, fullTx bool) (interface{}, error) {
	num, err := GetNumericBlockNumber(number, e)
	if err != nil {
		return nil, err
	}
	block, ok := e.d.store.GetBlockByNumber(num, true)
	if !ok {
		return nil, nil
	}
	return toBlock(block, fullTx), nil
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hash types.Hash, fullTx bool) (interface{}, error) {
	block, ok := e.d.store.GetBlockByHash(hash, true)
	if !ok {
		return nil, nil
	}
	return toBlock(block, fullTx), nil
}

func (e *Eth) GetBlockTransactionCountByNumber(number BlockNumber) (interface{}, error) {
	num, err := GetNumericBlockNumber(number, e)
	if err != nil {
		return nil, err
	}
	block, ok := e.d.store.GetBlockByNumber(num, true)
	if !ok {
		return nil, nil
	}
	return len(block.Transactions), nil
}

// BlockNumber returns current block number
func (e *Eth) BlockNumber() (interface{}, error) {
	h := e.d.store.Header()
	if h == nil {
		return nil, fmt.Errorf("header has a nil value")
	}
	return argUintPtr(h.Number), nil
}

// SendRawTransaction sends a raw transaction
func (e *Eth) SendRawTransaction(input string) (interface{}, error) {
	buf := hex.MustDecodeHex(input)

	tx := &types.Transaction{}
	if err := tx.UnmarshalRLP(buf); err != nil {
		return nil, err
	}
	tx.ComputeHash()

	if err := e.d.store.AddTx(tx); err != nil {
		return nil, err
	}
	return tx.Hash.String(), nil
}

// SendTransaction creates new message call transaction or a contract creation, if the data field contains code.
func (e *Eth) SendTransaction(arg *txnArgs) (interface{}, error) {
	transaction, err := e.d.decodeTxn(arg)
	if err != nil {
		return nil, err
	}
	if err := e.d.store.AddTx(transaction); err != nil {
		return nil, err
	}
	return transaction.Hash.String(), nil
}

// GetTransactionByHash returns a transaction by its hash.
// If the transaction is still pending -> return the txn with some fields omitted
// If the transaction is sealed into a block -> return the whole txn with all fields
func (e *Eth) GetTransactionByHash(hash types.Hash) (interface{}, error) {
	// findSealedTx is a helper method for checking the world state
	// for the transaction with the provided hash
	findSealedTx := func() *transaction {
		// Check the chain state for the transaction
		blockHash, ok := e.d.store.ReadTxLookup(hash)
		if !ok {
			// Block not found in storage
			return nil
		}
		block, ok := e.d.store.GetBlockByHash(blockHash, true)
		if !ok {
			// Block receipts not found in storage
			return nil
		}

		// Find the transaction within the block
		for idx, txn := range block.Transactions {
			if txn.Hash == hash {
				return toTransaction(
					txn,
					argUintPtr(block.Number()),
					argHashPtr(block.Hash()),
					&idx,
				)
			}
		}

		return nil
	}

	// findPendingTx is a helper method for checking the TxPool
	// for the pending transaction with the provided hash
	findPendingTx := func() *transaction {
		// Check the TxPool for the transaction if it's pending
		if pendingTx, pendingFound := e.d.store.GetPendingTx(hash); pendingFound {
			return toPendingTransaction(pendingTx)
		}

		// Transaction not found in the TxPool
		return nil
	}

	// 1. Check the chain state for the txn
	if resultTxn := findSealedTx(); resultTxn != nil {
		return resultTxn, nil
	}

	// 2. Check the TxPool for the txn
	if resultTxn := findPendingTx(); resultTxn != nil {
		return resultTxn, nil
	}

	// Transaction not found in state or TxPool
	e.d.logger.Warn(
		fmt.Sprintf("Transaction with hash [%s] not found", hash),
	)

	return nil, nil
}

// GetTransactionReceipt returns a transaction receipt by his hash
func (e *Eth) GetTransactionReceipt(hash types.Hash) (interface{}, error) {
	blockHash, ok := e.d.store.ReadTxLookup(hash)
	if !ok {
		// txn not found
		return nil, nil
	}

	block, ok := e.d.store.GetBlockByHash(blockHash, true)
	if !ok {
		// block not found
		e.d.logger.Warn(
			fmt.Sprintf("Block with hash [%s] not found", blockHash.String()),
		)
		return nil, nil
	}

	receipts, err := e.d.store.GetReceiptsByHash(blockHash)
	if err != nil {
		// block receipts not found
		e.d.logger.Warn(
			fmt.Sprintf("Receipts for block with hash [%s] not found", blockHash.String()),
		)
		return nil, nil
	}
	if len(receipts) == 0 {
		// Receipts not written yet on the db
		e.d.logger.Warn(
			fmt.Sprintf("No receipts found for block with hash [%s]", blockHash.String()),
		)
		return nil, nil
	}
	// find the transaction in the body
	indx := -1
	for i, txn := range block.Transactions {
		if txn.Hash == hash {
			indx = i
			break
		}
	}
	if indx == -1 {
		// txn not found
		return nil, nil
	}

	txn := block.Transactions[indx]
	raw := receipts[indx]

	logs := make([]*Log, len(raw.Logs))
	for indx, elem := range raw.Logs {
		logs[indx] = &Log{
			Address:     elem.Address,
			Topics:      elem.Topics,
			Data:        argBytes(elem.Data),
			BlockHash:   block.Hash(),
			BlockNumber: argUint64(block.Number()),
			TxHash:      txn.Hash,
			TxIndex:     argUint64(indx),
			LogIndex:    argUint64(indx),
			Removed:     false,
		}
	}
	res := &receipt{
		Root:              raw.Root,
		CumulativeGasUsed: argUint64(raw.CumulativeGasUsed),
		LogsBloom:         raw.LogsBloom,
		Status:            argUint64(*raw.Status),
		TxHash:            txn.Hash,
		TxIndex:           argUint64(indx),
		BlockHash:         block.Hash(),
		BlockNumber:       argUint64(block.Number()),
		GasUsed:           argUint64(raw.GasUsed),
		ContractAddress:   raw.ContractAddress,
		FromAddr:          txn.From,
		ToAddr:            txn.To,
		Logs:              logs,
	}
	return res, nil
}

// GetStorageAt returns the contract storage at the index position
func (e *Eth) GetStorageAt(
	address types.Address,
	index types.Hash,
	number *BlockNumber,
) (interface{}, error) {
	// Set the block number to latest
	if number == nil {
		number, _ = createBlockNumberPointer("latest")
	}
	// Fetch the requested header
	header, err := e.d.getBlockHeaderImpl(*number)
	if err != nil {
		return nil, err
	}

	// Get the storage for the passed in location
	result, err := e.d.store.GetStorage(header.StateRoot, address, index)
	if err != nil {
		if errors.As(err, &ErrStateNotFound) {
			return argBytesPtr(types.ZeroHash[:]), nil
		}
		return nil, err
	}
	// Parse the RLP value
	p := &fastrlp.Parser{}
	v, err := p.Parse(result)
	if err != nil {
		return argBytesPtr(types.ZeroHash[:]), nil
	}
	data, err := v.Bytes()
	if err != nil {
		return argBytesPtr(types.ZeroHash[:]), nil
	}
	return argBytesPtr(data), nil
}

// GasPrice returns the average gas price based on the last x blocks
func (e *Eth) GasPrice() (interface{}, error) {
	// Grab the average gas price and convert it to a hex value
	avgGasPrice := hex.EncodeBig(e.d.store.GetAvgGasPrice())

	return avgGasPrice, nil
}

// Call executes a smart contract call using the transaction object data
func (e *Eth) Call(
	arg *txnArgs,
	number *BlockNumber,
) (interface{}, error) {
	if number == nil {
		number, _ = createBlockNumberPointer("latest")
	}
	transaction, err := e.d.decodeTxn(arg)
	if err != nil {
		return nil, err
	}
	// Fetch the requested header
	header, err := e.d.getBlockHeaderImpl(*number)
	if err != nil {
		return nil, err
	}

	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if transaction.Gas == 0 {
		transaction.Gas = header.GasLimit
	}

	// The return value of the execution is saved in the transition (returnValue field)
	result, err := e.d.store.ApplyTxn(header, transaction)
	if err != nil {
		return nil, err
	}

	if result.Failed() {
		return nil, fmt.Errorf("unable to execute call: %s", result.Err.Error())
	}
	return argBytesPtr(result.ReturnValue), nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (e *Eth) EstimateGas(
	arg *txnArgs,
	rawNum *BlockNumber,
) (interface{}, error) {
	transaction, err := e.d.decodeTxn(arg)
	if err != nil {
		return nil, err
	}

	number := LatestBlockNumber
	if rawNum != nil {
		number = *rawNum
	}

	// Fetch the requested header
	header, err := e.d.getBlockHeaderImpl(number)
	if err != nil {
		return nil, err
	}

	forksInTime := e.d.store.GetForksInTime(uint64(number))

	var standardGas uint64
	if transaction.IsContractCreation() && forksInTime.Homestead {
		standardGas = state.TxGasContractCreation
	} else {
		standardGas = state.TxGas
	}

	var (
		lowEnd  = standardGas
		highEnd uint64
		gasCap  uint64
	)

	// If the gas limit was passed in, use it as a ceiling
	if transaction.Gas != 0 && transaction.Gas >= standardGas {
		highEnd = transaction.Gas
	} else {
		// If not, use the referenced block number
		highEnd = header.GasLimit
	}

	gasPriceInt := new(big.Int).Set(transaction.GasPrice)
	valueInt := new(big.Int).Set(transaction.Value)

	// If the sender address is present, recalculate the ceiling to his balance
	if transaction.From != types.ZeroAddress && transaction.GasPrice != nil && gasPriceInt.BitLen() != 0 {
		// Get the account balance

		// If the account is not initialized yet in state,
		// assume it's an empty account
		accountBalance := big.NewInt(0)
		acc, err := e.d.store.GetAccount(header.StateRoot, transaction.From)
		if err != nil && !errors.As(err, &ErrStateNotFound) {
			// An unrelated error occurred, return it
			return nil, err
		} else if err == nil {
			// No error when fetching the account,
			// read the balance from state
			accountBalance = acc.Balance
		}

		available := new(big.Int).Set(accountBalance)

		if transaction.Value != nil {
			if valueInt.Cmp(available) >= 0 {
				return nil, fmt.Errorf("insufficient funds for execution")
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

		result, err := e.d.store.ApplyTxn(header, txn)

		if err != nil {
			return true, err
		}

		return result.Failed(), nil
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

	// we stopped the binary search at the last gas limit
	// at which the txn could not be executed
	highEnd++

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
func (e *Eth) GetLogs(filterOptions *LogFilter) (interface{}, error) {
	result := make([]*Log, 0)
	parseReceipts := func(header *types.Header) error {
		receipts, err := e.d.store.GetReceiptsByHash(header.Hash)
		if err != nil {
			return err
		}

		for indx, receipt := range receipts {
			for logIndx, log := range receipt.Logs {
				if filterOptions.Match(log) {
					result = append(result, &Log{
						Address:     log.Address,
						Topics:      log.Topics,
						Data:        argBytes(log.Data),
						BlockNumber: argUint64(header.Number),
						BlockHash:   header.Hash,
						TxHash:      receipt.TxHash,
						TxIndex:     argUint64(indx),
						LogIndex:    argUint64(logIndx),
					})
				}
			}
		}
		return nil
	}

	if filterOptions.BlockHash != nil {
		block, ok := e.d.store.GetBlockByHash(*filterOptions.BlockHash, false)
		if !ok {
			return nil, fmt.Errorf("not found")
		}
		if err := parseReceipts(block.Header); err != nil {
			return nil, err
		}
		return result, nil
	}

	head := e.d.store.Header().Number

	resolveNum := func(num BlockNumber) uint64 {
		if num == PendingBlockNumber || num == EarliestBlockNumber {
			num = LatestBlockNumber
		}
		if num == LatestBlockNumber {
			return head
		}
		return uint64(num)
	}

	from := resolveNum(filterOptions.fromBlock)
	to := resolveNum(filterOptions.toBlock)

	if to < from {
		return nil, fmt.Errorf("incorrect range")
	}
	for i := from; i <= to; i++ {
		header, ok := e.d.store.GetHeaderByNumber(i)
		if !ok {
			break
		}
		if header.Number == 0 {
			// do not check logs in genesis
			continue
		}
		if err := parseReceipts(header); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetBalance returns the account's balance at the referenced block
func (e *Eth) GetBalance(
	address types.Address,
	number *BlockNumber,
) (interface{}, error) {
	if number == nil {
		number, _ = createBlockNumberPointer("latest")
	}
	header, err := e.d.getBlockHeaderImpl(*number)
	if err != nil {
		return nil, err
	}

	acc, err := e.d.store.GetAccount(header.StateRoot, address)
	if errors.As(err, &ErrStateNotFound) {
		// Account not found, return an empty account
		return argUintPtr(0), nil
	} else if err != nil {
		return nil, err
	}

	return argBigPtr(acc.Balance), nil
}

// GetTransactionCount returns account nonce
func (e *Eth) GetTransactionCount(
	address types.Address,
	number *BlockNumber,
) (interface{}, error) {
	if number == nil {
		number, _ = createBlockNumberPointer("latest")
	}
	nonce, err := e.d.getNextNonce(address, *number)
	if err != nil {
		if errors.As(err, &ErrStateNotFound) {
			return argUintPtr(0), nil
		}
		return nil, err
	}
	return argUintPtr(nonce), nil
}

// GetCode returns account code at given block number
func (e *Eth) GetCode(address types.Address, number *BlockNumber) (interface{}, error) {
	// Set the block number to latest
	if number == nil {
		number, _ = createBlockNumberPointer("latest")
	}
	header, err := e.d.getBlockHeaderImpl(*number)
	if err != nil {
		return nil, err
	}

	emptySlice := []byte{}
	acc, err := e.d.store.GetAccount(header.StateRoot, address)
	if errors.As(err, &ErrStateNotFound) {
		// If the account doesn't exist / is not initialized yet,
		// return the default value
		return "0x", nil
	} else if err != nil {
		return argBytesPtr(emptySlice), err
	}

	code, err := e.d.store.GetCode(types.BytesToHash(acc.CodeHash))
	if err != nil {
		// TODO This is just a workaround. Figure out why CodeHash is populated for regular accounts
		return argBytesPtr(emptySlice), nil
	}

	return argBytesPtr(code), nil
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
	return e.d.filterManager.GetFilterChanges(id)
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
