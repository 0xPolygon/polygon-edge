package jsonrpc

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"
	"math/big"
)

type ethTxPoolStore interface {
	// GetNonce returns the next nonce for this address
	GetNonce(addr types.Address) uint64

	// AddTx adds a new transaction to the tx pool
	AddTx(tx *types.Transaction) error

	// GetPendingTx gets the pending transaction from the transaction pool, if it's present
	GetPendingTx(txHash types.Hash) (*types.Transaction, bool)
}

type ethStateStore interface {
	GetAccount(root types.Hash, addr types.Address) (*state.Account, error)
	GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error)
	GetForksInTime(blockNumber uint64) chain.ForksInTime
	GetCode(hash types.Hash) ([]byte, error)
}

type ethBlockchainStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// GetHeaderByNumber returns the header by number
	GetHeaderByNumber(block uint64) (*types.Header, bool)

	// GetBlockByHash gets a block using the provided hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	// GetBlockByNumber returns a block using the provided number
	GetBlockByNumber(num uint64, full bool) (*types.Block, bool)

	// ReadTxLookup returns a block hash in which a given txn was mined
	ReadTxLookup(txnHash types.Hash) (types.Hash, bool)

	// GetReceiptsByHash returns the receipts for a block hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)

	// GetAvgGasPrice returns the average gas price
	GetAvgGasPrice() *big.Int

	// ApplyTxn applies a transaction object to the blockchain
	ApplyTxn(header *types.Header, txn *types.Transaction) (*runtime.ExecutionResult, error)

	// GetSyncProgression retrieves the current sync progression, if any
	GetSyncProgression() *progress.Progression
}

// ethStore provides access to the methods needed by eth endpoint
type ethStore interface {
	ethTxPoolStore
	ethStateStore
	ethBlockchainStore
}

// Eth is the eth jsonrpc endpoint
type Eth struct {
	logger        hclog.Logger
	store         ethStore
	chainID       uint64
	filterManager *FilterManager
	priceLimit    uint64
}

var (
	ErrInsufficientFunds = errors.New("insufficient funds for execution")
)

// ChainId returns the chain id of the client
//nolint:stylecheck
func (e *Eth) ChainId() (interface{}, error) {
	return argUintPtr(e.chainID), nil
}

func (e *Eth) getHeaderFromBlockNumberOrHash(bnh *BlockNumberOrHash) (*types.Header, error) {
	var (
		header *types.Header
		err    error
	)

	if bnh.BlockNumber != nil {
		header, err = e.getBlockHeader(*bnh.BlockNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to get the header of block %d: %w", *bnh.BlockNumber, err)
		}
	} else if bnh.BlockHash != nil {
		block, ok := e.store.GetBlockByHash(*bnh.BlockHash, false)
		if !ok {
			return nil, fmt.Errorf("could not find block referenced by the hash %s", bnh.BlockHash.String())
		}

		header = block.Header
	}

	return header, nil
}

func (e *Eth) Syncing() (interface{}, error) {
	if syncProgression := e.store.GetSyncProgression(); syncProgression != nil {
		// Node is bulk syncing, return the status
		return progression{
			Type:          string(syncProgression.SyncType),
			StartingBlock: hex.EncodeUint64(syncProgression.StartingBlock),
			CurrentBlock:  hex.EncodeUint64(syncProgression.CurrentBlock),
			HighestBlock:  hex.EncodeUint64(syncProgression.HighestBlock),
		}, nil
	}

	// Node is not bulk syncing
	return false, nil
}

func GetNumericBlockNumber(number BlockNumber, e *Eth) (uint64, error) {
	switch number {
	case LatestBlockNumber:
		return e.store.Header().Number, nil

	case EarliestBlockNumber:
		return 0, nil

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

	block, ok := e.store.GetBlockByNumber(num, true)

	if !ok {
		return nil, nil
	}

	return toBlock(block, fullTx), nil
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hash types.Hash, fullTx bool) (interface{}, error) {
	block, ok := e.store.GetBlockByHash(hash, true)
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

	block, ok := e.store.GetBlockByNumber(num, true)

	if !ok {
		return nil, nil
	}

	return len(block.Transactions), nil
}

// BlockNumber returns current block number
func (e *Eth) BlockNumber() (interface{}, error) {
	h := e.store.Header()
	if h == nil {
		return nil, fmt.Errorf("header has a nil value")
	}

	return argUintPtr(h.Number), nil
}

// SendRawTransaction sends a raw transaction
func (e *Eth) SendRawTransaction(input string) (interface{}, error) {
	buf, decodeErr := hex.DecodeHex(input)
	if decodeErr != nil {
		return nil, fmt.Errorf("unable to decode input, %w", decodeErr)
	}

	tx := &types.Transaction{}
	if err := tx.UnmarshalRLP(buf); err != nil {
		return nil, err
	}

	tx.ComputeHash()

	if err := e.store.AddTx(tx); err != nil {
		return nil, err
	}

	return tx.Hash.String(), nil
}

// SendTransaction rejects eth_sendTransaction json-rpc call as we don't support wallet management
func (e *Eth) SendTransaction(_ *txnArgs) (interface{}, error) {
	return nil, fmt.Errorf("request calls to eth_sendTransaction method are not supported," +
		" use eth_sendRawTransaction insead")
}

// GetTransactionByHash returns a transaction by its hash.
// If the transaction is still pending -> return the txn with some fields omitted
// If the transaction is sealed into a block -> return the whole txn with all fields
func (e *Eth) GetTransactionByHash(hash types.Hash) (interface{}, error) {
	// findSealedTx is a helper method for checking the world state
	// for the transaction with the provided hash
	findSealedTx := func() *transaction {
		// Check the chain state for the transaction
		blockHash, ok := e.store.ReadTxLookup(hash)
		if !ok {
			// Block not found in storage
			return nil
		}

		block, ok := e.store.GetBlockByHash(blockHash, true)

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
		if pendingTx, pendingFound := e.store.GetPendingTx(hash); pendingFound {
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
	e.logger.Warn(
		fmt.Sprintf("Transaction with hash [%s] not found", hash),
	)

	return nil, nil
}

// GetTransactionReceipt returns a transaction receipt by his hash
func (e *Eth) GetTransactionReceipt(hash types.Hash) (interface{}, error) {
	blockHash, ok := e.store.ReadTxLookup(hash)
	if !ok {
		// txn not found
		return nil, nil
	}

	block, ok := e.store.GetBlockByHash(blockHash, true)
	if !ok {
		// block not found
		e.logger.Warn(
			fmt.Sprintf("Block with hash [%s] not found", blockHash.String()),
		)

		return nil, nil
	}

	receipts, err := e.store.GetReceiptsByHash(blockHash)
	if err != nil {
		// block receipts not found
		e.logger.Warn(
			fmt.Sprintf("Receipts for block with hash [%s] not found", blockHash.String()),
		)

		return nil, nil
	}

	if len(receipts) == 0 {
		// Receipts not written yet on the db
		e.logger.Warn(
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
	filter BlockNumberOrHash,
) (interface{}, error) {
	var (
		header *types.Header
		err    error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err = e.getHeaderFromBlockNumberOrHash(&filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	// Get the storage for the passed in location
	result, err := e.store.GetStorage(header.StateRoot, address, index)
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
// taking into consideration operator defined price limit
func (e *Eth) GasPrice() (string, error) {
	// Fetch average gas price in uint64
	avgGasPrice := e.store.GetAvgGasPrice().Uint64()

	// Return --price-limit flag defined value if it is greater than avgGasPrice
	return hex.EncodeUint64(common.Max(e.priceLimit, avgGasPrice)), nil
}

// Call executes a smart contract call using the transaction object data
func (e *Eth) Call(arg *txnArgs, filter BlockNumberOrHash) (interface{}, error) {
	var (
		header *types.Header
		err    error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err = e.getHeaderFromBlockNumberOrHash(&filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	transaction, err := e.decodeTxn(arg)

	if err != nil {
		return nil, err
	}
	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if transaction.Gas == 0 {
		transaction.Gas = header.GasLimit
	}

	// The return value of the execution is saved in the transition (returnValue field)
	result, err := e.store.ApplyTxn(header, transaction)
	if err != nil {
		return nil, err
	}

	// Check if an EVM revert happened
	if result.Reverted() {
		return nil, constructErrorFromRevert(result)
	}

	if result.Failed() {
		return nil, fmt.Errorf("unable to execute call: %w", result.Err)
	}

	return argBytesPtr(result.ReturnValue), nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (e *Eth) EstimateGas(arg *txnArgs, rawNum *BlockNumber) (interface{}, error) {
	transaction, err := e.decodeTxn(arg)
	if err != nil {
		return nil, err
	}

	number := LatestBlockNumber
	if rawNum != nil {
		number = *rawNum
	}

	// Fetch the requested header
	header, err := e.getBlockHeader(number)
	if err != nil {
		return nil, err
	}

	forksInTime := e.store.GetForksInTime(uint64(number))

	var standardGas uint64
	if transaction.IsContractCreation() && forksInTime.Homestead {
		standardGas = state.TxGasContractCreation
	} else {
		standardGas = state.TxGas
	}

	var (
		lowEnd  = standardGas
		highEnd uint64
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

	var availableBalance *big.Int

	// If the sender address is present, figure out how much available funds
	// are we working with
	if transaction.From != types.ZeroAddress {
		// Get the account balance
		// If the account is not initialized yet in state,
		// assume it's an empty account
		accountBalance := big.NewInt(0)
		acc, err := e.store.GetAccount(header.StateRoot, transaction.From)

		if err != nil && !errors.As(err, &ErrStateNotFound) {
			// An unrelated error occurred, return it
			return nil, err
		} else if err == nil {
			// No error when fetching the account,
			// read the balance from state
			accountBalance = acc.Balance
		}

		availableBalance = new(big.Int).Set(accountBalance)

		if transaction.Value != nil {
			if valueInt.Cmp(availableBalance) > 0 {
				return 0, ErrInsufficientFunds
			}

			availableBalance.Sub(availableBalance, valueInt)
		}
	}

	// Recalculate the gas ceiling based on the available funds (if any)
	// and the passed in gas price (if present)
	if gasPriceInt.BitLen() != 0 && // Gas price has been set
		availableBalance != nil && // Available balance is found
		availableBalance.Cmp(big.NewInt(0)) > 0 { // Available balance > 0
		gasAllowance := new(big.Int).Div(availableBalance, gasPriceInt)

		// Check the gas allowance for this account, make sure high end is capped to it
		if gasAllowance.IsUint64() && highEnd > gasAllowance.Uint64() {
			e.logger.Debug(
				fmt.Sprintf(
					"Gas estimation high-end capped by allowance [%d]",
					gasAllowance.Uint64(),
				),
			)

			highEnd = gasAllowance.Uint64()
		}
	}

	// Checks if executor level valid gas errors occurred
	isGasApplyError := func(err error) bool {
		return errors.As(err, &state.ErrNotEnoughIntrinsicGas)
	}

	// Checks if EVM level valid gas errors occurred
	isGasEVMError := func(err error) bool {
		return errors.Is(err, runtime.ErrOutOfGas) ||
			errors.Is(err, runtime.ErrCodeStoreOutOfGas)
	}

	// Checks if the EVM reverted during execution
	isEVMRevertError := func(err error) bool {
		return errors.Is(err, runtime.ErrExecutionReverted)
	}

	// Run the transaction with the specified gas value.
	// Returns a status indicating if the transaction failed and the accompanying error
	testTransaction := func(gas uint64, shouldOmitErr bool) (bool, error) {
		// Create a dummy transaction with the new gas
		txn := transaction.Copy()
		txn.Gas = gas

		result, applyErr := e.store.ApplyTxn(header, txn)

		if applyErr != nil {
			// Check the application error.
			// Gas apply errors are valid, and should be ignored
			if isGasApplyError(applyErr) && shouldOmitErr {
				// Specifying the transaction failed, but not providing an error
				// is an indication that a valid error occurred due to low gas,
				// which will increase the lower bound for the search
				return true, nil
			}

			return true, applyErr
		}

		// Check if an out of gas error happened during EVM execution
		if result.Failed() {
			if isGasEVMError(result.Err) && shouldOmitErr {
				// Specifying the transaction failed, but not providing an error
				// is an indication that a valid error occurred due to low gas,
				// which will increase the lower bound for the search
				return true, nil
			}

			if isEVMRevertError(result.Err) {
				// The EVM reverted during execution, attempt to extract the
				// error message and return it
				return true, constructErrorFromRevert(result)
			}

			return true, result.Err
		}

		return false, nil
	}

	// Start the binary search for the lowest possible gas price
	for lowEnd < highEnd {
		mid := (lowEnd + highEnd) / 2

		failed, testErr := testTransaction(mid, true)
		if testErr != nil &&
			!isEVMRevertError(testErr) {
			// Reverts are ignored in the binary search, but are checked later on
			// during the execution for the optimal gas limit found
			return 0, testErr
		}

		if failed {
			// If the transaction failed => increase the gas
			lowEnd = mid + 1
		} else {
			// If the transaction didn't fail => make this ok value the high end
			highEnd = mid
		}
	}

	// Check if the highEnd is a good value to make the transaction pass
	failed, err := testTransaction(highEnd, false)
	if failed {
		// The transaction shouldn't fail, for whatever reason, at highEnd
		return 0, fmt.Errorf(
			"unable to apply transaction even for the highest gas limit %d: %w",
			highEnd,
			err,
		)
	}

	return hex.EncodeUint64(highEnd), nil
}

// GetFilterLogs returns an array of logs for the specified filter
func (e *Eth) GetFilterLogs(id string) (interface{}, error) {
	logFilter, err := e.filterManager.GetLogFilterFromID(id)
	if err != nil {
		return nil, err
	}

	return e.filterManager.GetLogsForQuery(logFilter.query)
}

// GetLogs returns an array of logs matching the filter options
func (e *Eth) GetLogs(query *LogQuery) (interface{}, error) {
	return e.filterManager.GetLogsForQuery(query)
}

// GetBalance returns the account's balance at the referenced block.
func (e *Eth) GetBalance(address types.Address, filter BlockNumberOrHash) (interface{}, error) {
	var (
		header *types.Header
		err    error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err = e.getHeaderFromBlockNumberOrHash(&filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	// Extract the account balance
	acc, err := e.store.GetAccount(header.StateRoot, address)
	if errors.As(err, &ErrStateNotFound) {
		// Account not found, return an empty account
		return argUintPtr(0), nil
	} else if err != nil {
		return nil, err
	}

	return argBigPtr(acc.Balance), nil
}

// GetTransactionCount returns account nonce
func (e *Eth) GetTransactionCount(address types.Address, filter BlockNumberOrHash) (interface{}, error) {
	var (
		blockNumber BlockNumber
		header      *types.Header
		err         error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	if filter.BlockNumber == nil {
		header, err = e.getHeaderFromBlockNumberOrHash(&filter)
		if err != nil {
			return nil, fmt.Errorf("failed to get header from block hash or block number")
		}

		blockNumber = BlockNumber(header.Number)
	} else {
		blockNumber = *filter.BlockNumber
	}

	nonce, err := e.getNextNonce(address, blockNumber)
	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			return argUintPtr(0), nil
		}

		return nil, err
	}

	return argUintPtr(nonce), nil
}

// GetCode returns account code at given block number
func (e *Eth) GetCode(address types.Address, filter BlockNumberOrHash) (interface{}, error) {
	var (
		header *types.Header
		err    error
	)

	// The filter is empty, use the latest block by default
	if filter.BlockNumber == nil && filter.BlockHash == nil {
		filter.BlockNumber, _ = createBlockNumberPointer("latest")
	}

	header, err = e.getHeaderFromBlockNumberOrHash(&filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get header from block hash or block number")
	}

	emptySlice := []byte{}
	acc, err := e.store.GetAccount(header.StateRoot, address)

	if errors.As(err, &ErrStateNotFound) {
		// If the account doesn't exist / is not initialized yet,
		// return the default value
		return "0x", nil
	} else if err != nil {
		return argBytesPtr(emptySlice), err
	}

	code, err := e.store.GetCode(types.BytesToHash(acc.CodeHash))
	if err != nil {
		// TODO This is just a workaround. Figure out why CodeHash is populated for regular accounts
		return argBytesPtr(emptySlice), nil
	}

	return argBytesPtr(code), nil
}

// NewFilter creates a filter object, based on filter options, to notify when the state changes (logs).
func (e *Eth) NewFilter(filter *LogQuery) (interface{}, error) {
	return e.filterManager.NewLogFilter(filter, nil), nil
}

// NewBlockFilter creates a filter in the node, to notify when a new block arrives
func (e *Eth) NewBlockFilter() (interface{}, error) {
	return e.filterManager.NewBlockFilter(nil), nil
}

// GetFilterChanges is a polling method for a filter, which returns an array of logs which occurred since last poll.
func (e *Eth) GetFilterChanges(id string) (interface{}, error) {
	return e.filterManager.GetFilterChanges(id)
}

// UninstallFilter uninstalls a filter with given ID
func (e *Eth) UninstallFilter(id string) (bool, error) {
	ok := e.filterManager.Uninstall(id)

	return ok, nil
}

// Unsubscribe uninstalls a filter in a websocket
func (e *Eth) Unsubscribe(id string) (bool, error) {
	ok := e.filterManager.Uninstall(id)

	return ok, nil
}

func (e *Eth) getBlockHeader(number BlockNumber) (*types.Header, error) {
	switch number {
	case LatestBlockNumber:
		return e.store.Header(), nil

	case EarliestBlockNumber:
		header, ok := e.store.GetHeaderByNumber(uint64(0))
		if !ok {
			return nil, fmt.Errorf("error fetching genesis block header")
		}

		return header, nil

	case PendingBlockNumber:
		return nil, fmt.Errorf("fetching the pending header is not supported")

	default:
		// Convert the block number from hex to uint64
		header, ok := e.store.GetHeaderByNumber(uint64(number))
		if !ok {
			return nil, fmt.Errorf("error fetching block number %d header", uint64(number))
		}

		return header, nil
	}
}

// getNextNonce returns the next nonce for the account for the specified block
func (e *Eth) getNextNonce(address types.Address, number BlockNumber) (uint64, error) {
	if number == PendingBlockNumber {
		// Grab the latest pending nonce from the TxPool
		//
		// If the account is not initialized in the local TxPool,
		// return the latest nonce from the world state
		res := e.store.GetNonce(address)

		return res, nil
	}

	header, err := e.getBlockHeader(number)
	if err != nil {
		return 0, err
	}

	acc, err := e.store.GetAccount(header.StateRoot, address)

	if errors.As(err, &ErrStateNotFound) {
		// If the account doesn't exist / isn't initialized,
		// return a nonce value of 0
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return acc.Nonce, nil
}

func (e *Eth) decodeTxn(arg *txnArgs) (*types.Transaction, error) {
	// set default values
	if arg.From == nil {
		arg.From = &types.ZeroAddress
		arg.Nonce = argUintPtr(0)
	} else if arg.Nonce == nil {
		// get nonce from the pool
		nonce, err := e.getNextNonce(*arg.From, LatestBlockNumber)
		if err != nil {
			return nil, err
		}
		arg.Nonce = argUintPtr(nonce)
	}

	if arg.Value == nil {
		arg.Value = argBytesPtr([]byte{})
	}

	if arg.GasPrice == nil {
		arg.GasPrice = argBytesPtr([]byte{})
	}

	var input []byte
	if arg.Data != nil {
		input = *arg.Data
	} else if arg.Input != nil {
		input = *arg.Input
	}

	if arg.To == nil {
		if input == nil {
			return nil, fmt.Errorf("contract creation without data provided")
		}
	}

	if input == nil {
		input = []byte{}
	}

	if arg.Gas == nil {
		arg.Gas = argUintPtr(0)
	}

	txn := &types.Transaction{
		From:     *arg.From,
		Gas:      uint64(*arg.Gas),
		GasPrice: new(big.Int).SetBytes(*arg.GasPrice),
		Value:    new(big.Int).SetBytes(*arg.Value),
		Input:    input,
		Nonce:    uint64(*arg.Nonce),
	}
	if arg.To != nil {
		txn.To = arg.To
	}

	txn.ComputeHash()

	return txn, nil
}
