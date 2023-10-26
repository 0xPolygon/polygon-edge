package jsonrpc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/gasprice"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
)

type ethTxPoolStore interface {
	// AddTx adds a new transaction to the tx pool
	AddTx(tx *types.Transaction) error

	// GetPendingTx gets the pending transaction from the transaction pool, if it's present
	GetPendingTx(txHash types.Hash) (*types.Transaction, bool)

	// GetNonce returns the next nonce for this address
	GetNonce(addr types.Address) uint64

	// GetBaseFee returns the current base fee of TxPool
	GetBaseFee() uint64
}

type Account struct {
	Balance *big.Int
	Nonce   uint64
}

type ethStateStore interface {
	GetAccount(root types.Hash, addr types.Address) (*Account, error)
	GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error)
	GetForksInTime(blockNumber uint64) chain.ForksInTime
	GetCode(root types.Hash, addr types.Address) ([]byte, error)
}

type ethBlockchainStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// GetHeaderByNumber gets a header using the provided number
	GetHeaderByNumber(uint64) (*types.Header, bool)

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
	ApplyTxn(
		header *types.Header,
		txn *types.Transaction,
		override types.StateOverride,
		nonPayable bool,
	) (*runtime.ExecutionResult, error)

	// GetSyncProgression retrieves the current sync progression, if any
	GetSyncProgression() *progress.Progression
}

type ethFilter interface {
	// FilterExtra filters extra data from header extra that is not included in block hash
	FilterExtra(extra []byte) ([]byte, error)
}

// ethStore provides access to the methods needed by eth endpoint
type ethStore interface {
	ethTxPoolStore
	ethStateStore
	ethBlockchainStore
	ethFilter
	gasprice.GasStore
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
//
//nolint:stylecheck
func (e *Eth) ChainId() (interface{}, error) {
	return argUintPtr(e.chainID), nil
}

func (e *Eth) Syncing() (interface{}, error) {
	if syncProgression := e.store.GetSyncProgression(); syncProgression != nil {
		// Node is bulk syncing, return the status
		return progression{
			Type:          string(syncProgression.SyncType),
			StartingBlock: argUint64(syncProgression.StartingBlock),
			CurrentBlock:  argUint64(syncProgression.CurrentBlock),
			HighestBlock:  argUint64(syncProgression.HighestBlock),
		}, nil
	}

	// Node is not bulk syncing
	return false, nil
}

// GetBlockByNumber returns information about a block by block number
func (e *Eth) GetBlockByNumber(number BlockNumber, fullTx bool) (interface{}, error) {
	num, err := GetNumericBlockNumber(number, e.store)
	if err != nil {
		return nil, err
	}

	block, ok := e.store.GetBlockByNumber(num, true)
	if !ok {
		return nil, nil
	}

	if err := e.filterExtra(block); err != nil {
		return nil, err
	}

	return toBlock(block, fullTx), nil
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hash types.Hash, fullTx bool) (interface{}, error) {
	block, ok := e.store.GetBlockByHash(hash, true)
	if !ok {
		return nil, nil
	}

	if err := e.filterExtra(block); err != nil {
		return nil, err
	}

	return toBlock(block, fullTx), nil
}

func (e *Eth) filterExtra(block *types.Block) error {
	// we need to copy it because the store returns header from storage directly
	// and not a copy, so changing it, actually changes it in storage as well
	headerCopy := block.Header.Copy()

	filteredExtra, err := e.store.FilterExtra(headerCopy.ExtraData)
	if err != nil {
		return err
	}

	headerCopy.ExtraData = filteredExtra
	// no need to recompute hash (filtered out data is not in the hash in the first place)
	block.Header = headerCopy

	return nil
}

func (e *Eth) GetBlockTransactionCountByNumber(number BlockNumber) (interface{}, error) {
	num, err := GetNumericBlockNumber(number, e.store)
	if err != nil {
		return nil, err
	}

	block, ok := e.store.GetBlockByNumber(num, true)

	if !ok {
		return nil, nil
	}

	return *common.EncodeUint64(uint64(len(block.Transactions))), nil
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
func (e *Eth) SendRawTransaction(buf argBytes) (interface{}, error) {
	tx := &types.Transaction{}
	if err := tx.UnmarshalRLP(buf); err != nil {
		return nil, err
	}

	// tx hash will be calculated inside e.store.AddTx
	if err := e.store.AddTx(tx); err != nil {
		return nil, err
	}

	return tx.Hash.String(), nil
}

// SendTransaction rejects eth_sendTransaction json-rpc call as we don't support wallet management
func (e *Eth) SendTransaction(_ *txnArgs) (interface{}, error) {
	return nil, fmt.Errorf("request calls to eth_sendTransaction method are not supported," +
		" use eth_sendRawTransaction instead")
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
		if txn, idx := types.FindTxByHash(block.Transactions, hash); txn != nil {
			txn.GasPrice = txn.GetGasPrice(block.Header.BaseFee)

			return toTransaction(
				txn,
				argUintPtr(block.Number()),
				argHashPtr(block.Hash()),
				&idx,
			)
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
	logIndex := 0
	txn, txIndex := types.FindTxByHash(block.Transactions, hash)

	if txIndex == -1 {
		// txn not found
		return nil, nil
	}

	for i := 0; i < txIndex; i++ {
		// accumulate receipt logs indexes from block transactions
		// that are before the desired transaction
		logIndex += len(receipts[i].Logs)
	}

	raw := receipts[txIndex]
	logs := toLogs(raw.Logs, uint64(logIndex), uint64(txIndex), block.Header, hash)

	return toReceipt(raw, txn, uint64(txIndex), block.Header, logs), nil
}

// GetStorageAt returns the contract storage at the index position
func (e *Eth) GetStorageAt(
	address types.Address,
	index types.Hash,
	filter BlockNumberOrHash,
) (interface{}, error) {
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	// Get the storage for the passed in location
	result, err := e.store.GetStorage(header.StateRoot, address, index)
	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			return argBytesPtr(types.ZeroHash[:]), nil
		}

		return nil, err
	}

	return argBytesPtr(result), nil
}

// GasPrice exposes "getGasPrice"'s function logic to public RPC interface
func (e *Eth) GasPrice() (interface{}, error) {
	gasPrice, err := e.getGasPrice()
	if err != nil {
		return nil, err
	}

	return argUint64(gasPrice), nil
}

// getGasPrice returns the average gas price based on the last x blocks
// taking into consideration operator defined price limit
func (e *Eth) getGasPrice() (uint64, error) {
	// Return --price-limit flag defined value if it is greater than avgGasPrice/baseFee+priorityFee
	if e.store.GetForksInTime(e.store.Header().Number).London {
		priorityFee, err := e.store.MaxPriorityFeePerGas()
		if err != nil {
			return 0, err
		}

		return common.Max(e.priceLimit, priorityFee.Uint64()+e.store.GetBaseFee()), nil
	}

	// Fetch average gas price in uint64
	avgGasPrice := e.store.GetAvgGasPrice().Uint64()

	return common.Max(e.priceLimit, avgGasPrice), nil
}

// fillTransactionGasPrice fills transaction gas price if no provided
func (e *Eth) fillTransactionGasPrice(tx *types.Transaction) error {
	if tx.GetGasPrice(e.store.GetBaseFee()).BitLen() > 0 {
		return nil
	}

	estimatedGasPrice, err := e.getGasPrice()
	if err != nil {
		return err
	}

	if tx.Type == types.DynamicFeeTx {
		tx.GasFeeCap = new(big.Int).SetUint64(estimatedGasPrice)
	} else {
		tx.GasPrice = new(big.Int).SetUint64(estimatedGasPrice)
	}

	return nil
}

type overrideAccount struct {
	Nonce     *argUint64                 `json:"nonce"`
	Code      *argBytes                  `json:"code"`
	Balance   *argUint64                 `json:"balance"`
	State     *map[types.Hash]types.Hash `json:"state"`
	StateDiff *map[types.Hash]types.Hash `json:"stateDiff"`
}

func (o *overrideAccount) ToType() types.OverrideAccount {
	res := types.OverrideAccount{}

	if o.Nonce != nil {
		res.Nonce = (*uint64)(o.Nonce)
	}

	if o.Code != nil {
		res.Code = *o.Code
	}

	if o.Balance != nil {
		res.Balance = new(big.Int).SetUint64(*(*uint64)(o.Balance))
	}

	if o.State != nil {
		res.State = *o.State
	}

	if o.StateDiff != nil {
		res.StateDiff = *o.StateDiff
	}

	return res
}

// StateOverride is the collection of overridden accounts.
type stateOverride map[types.Address]overrideAccount

// Call executes a smart contract call using the transaction object data
func (e *Eth) Call(arg *txnArgs, filter BlockNumberOrHash, apiOverride *stateOverride) (interface{}, error) {
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	transaction, err := DecodeTxn(arg, header.Number, e.store, true)
	if err != nil {
		return nil, err
	}

	// If the caller didn't supply the gas limit in the message, then we set it to maximum possible => block gas limit
	if transaction.Gas == 0 {
		transaction.Gas = header.GasLimit
	}

	// Force transaction gas price if empty
	if err = e.fillTransactionGasPrice(transaction); err != nil {
		return nil, err
	}

	var override types.StateOverride
	if apiOverride != nil {
		override = types.StateOverride{}
		for addr, o := range *apiOverride {
			override[addr] = o.ToType()
		}
	}

	// The return value of the execution is saved in the transition (returnValue field)
	result, err := e.store.ApplyTxn(header, transaction, override, true)
	if err != nil {
		return nil, err
	}

	// Check if an EVM revert happened
	if result.Reverted() {
		return []byte(hex.EncodeToString(result.ReturnValue)), constructErrorFromRevert(result)
	}

	if result.Failed() {
		return nil, fmt.Errorf("unable to execute call: %w", result.Err)
	}

	return argBytesPtr(result.ReturnValue), nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (e *Eth) EstimateGas(arg *txnArgs, rawNum *BlockNumber) (interface{}, error) {
	number := LatestBlockNumber
	if rawNum != nil {
		number = *rawNum
	}

	// Fetch the requested header
	header, err := GetBlockHeader(number, e.store)
	if err != nil {
		return nil, err
	}

	// testTransaction should execute tx with nonce always set to the current expected nonce for the account
	transaction, err := DecodeTxn(arg, header.Number, e.store, true)
	if err != nil {
		return nil, err
	}

	forksInTime := e.store.GetForksInTime(header.Number)

	if transaction.IsValueTransfer() {
		// if it is a simple value transfer or a contract creation,
		// we already know what is the transaction gas cost, no need to apply transaction
		gasCost, err := state.TransactionGasCost(transaction, forksInTime.Homestead, forksInTime.Istanbul)
		if err != nil {
			return nil, err
		}

		return argUint64(gasCost), nil
	}

	// Force transaction gas price if empty
	if err = e.fillTransactionGasPrice(transaction); err != nil {
		return nil, err
	}

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

	var availableBalance *big.Int

	// If the sender address is present, figure out how much available funds
	// are we working with
	if transaction.From != types.ZeroAddress {
		// Get the account balance
		// If the account is not initialized yet in state,
		// assume it's an empty account
		accountBalance := big.NewInt(0)
		acc, err := e.store.GetAccount(header.StateRoot, transaction.From)

		if err != nil && !errors.Is(err, ErrStateNotFound) {
			// An unrelated error occurred, return it
			return nil, err
		} else if err == nil {
			// No error when fetching the account,
			// read the balance from state
			accountBalance = acc.Balance
		}

		availableBalance = new(big.Int).Set(accountBalance)
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
		if errors.Is(err, state.ErrNotEnoughIntrinsicGas) {
			return true
		}

		var expected *state.TransitionApplicationError
		if errors.As(err, &expected) {
			return errors.Is(expected.Err, state.ErrNotEnoughIntrinsicGas)
		}

		return false
	}

	// Checks if EVM level valid gas errors occurred
	isGasEVMError := func(err error) bool {
		return errors.Is(err, runtime.ErrOutOfGas) || errors.Is(err, runtime.ErrCodeStoreOutOfGas)
	}

	// Checks if the EVM reverted during execution
	isEVMRevertError := func(err error) bool {
		return errors.Is(err, runtime.ErrExecutionReverted)
	}

	// Run the transaction with the specified gas value.
	// Returns a status indicating if the transaction failed, return value (data), and the accompanying error
	testTransaction := func(gas uint64, shouldOmitErr bool) (bool, interface{}, error) {
		var data interface{}

		transaction.Gas = gas

		result, applyErr := e.store.ApplyTxn(header, transaction, nil, true)

		if result != nil {
			data = []byte(hex.EncodeToString(result.ReturnValue))
		}

		if applyErr != nil {
			// Check the application error.
			// Gas apply errors are valid, and should be ignored
			if isGasApplyError(applyErr) && shouldOmitErr {
				// Specifying the transaction failed, but not providing an error
				// is an indication that a valid error occurred due to low gas,
				// which will increase the lower bound for the search
				return true, data, nil
			}

			return true, data, applyErr
		}

		// Check if an out of gas error happened during EVM execution
		if result.Failed() {
			if isGasEVMError(result.Err) && shouldOmitErr {
				// Specifying the transaction failed, but not providing an error
				// is an indication that a valid error occurred due to low gas,
				// which will increase the lower bound for the search
				return true, data, nil
			}

			if isEVMRevertError(result.Err) {
				// The EVM reverted during execution, attempt to extract the
				// error message and return it
				return true, data, constructErrorFromRevert(result)
			}

			return true, data, result.Err
		}

		return false, nil, nil
	}

	// Start the binary search for the lowest possible gas price
	for lowEnd < highEnd {
		mid := lowEnd + ((highEnd - lowEnd) >> 1) // (lowEnd + highEnd) / 2 can overflow

		failed, retVal, testErr := testTransaction(mid, true)
		if testErr != nil && !isEVMRevertError(testErr) {
			// Reverts are ignored in the binary search, but are checked later on
			// during the execution for the optimal gas limit found
			return retVal, testErr
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
	failed, retVal, err := testTransaction(highEnd, false)
	if failed {
		// The transaction shouldn't fail, for whatever reason, at highEnd
		return retVal, fmt.Errorf(
			"unable to apply transaction even for the highest gas limit %d: %w",
			highEnd,
			err,
		)
	}

	return argUint64(highEnd), nil
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
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	// Extract the account balance
	acc, err := e.store.GetAccount(header.StateRoot, address)
	if errors.Is(err, ErrStateNotFound) {
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
		filter.BlockNumber, _ = createBlockNumberPointer(latest)
	}

	if filter.BlockNumber == nil {
		header, err = GetHeaderFromBlockNumberOrHash(filter, e.store)
		if err != nil {
			return nil, fmt.Errorf("failed to get header from block hash or block number: %w", err)
		}

		blockNumber = BlockNumber(header.Number)
	} else {
		blockNumber = *filter.BlockNumber
	}

	nonce, err := GetNextNonce(address, blockNumber, e.store)
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
	header, err := GetHeaderFromBlockNumberOrHash(filter, e.store)
	if err != nil {
		return nil, err
	}

	emptySlice := []byte{}
	code, err := e.store.GetCode(header.StateRoot, address)

	if errors.Is(err, ErrStateNotFound) {
		// If the account doesn't exist / is not initialized yet,
		// return the default value
		return "0x", nil
	} else if err != nil {
		return argBytesPtr(emptySlice), err
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
	return e.filterManager.Uninstall(id), nil
}

// Unsubscribe uninstalls a filter in a websocket
func (e *Eth) Unsubscribe(id string) (bool, error) {
	return e.filterManager.Uninstall(id), nil
}

// MaxPriorityFeePerGas calculates the priority fee needed for transaction to be included in a block
func (e *Eth) MaxPriorityFeePerGas() (interface{}, error) {
	priorityFee, err := e.store.MaxPriorityFeePerGas()
	if err != nil {
		return nil, err
	}

	return argBigPtr(priorityFee), nil
}

func (e *Eth) FeeHistory(blockCount argUint64, newestBlock BlockNumber,
	rewardPercentiles []float64) (interface{}, error) {
	block, err := GetNumericBlockNumber(newestBlock, e.store)
	if err != nil {
		return nil, fmt.Errorf("could not parse newest block argument. Error: %w", err)
	}

	// Retrieve oldestBlock, baseFeePerGas, gasUsedRatio, and reward synchronously
	history, err := e.store.FeeHistory(uint64(blockCount), block, rewardPercentiles)
	if err != nil {
		return nil, err
	}

	// Create channels to receive the processed slices asynchronously
	baseFeePerGasCh := make(chan []argUint64)
	gasUsedRatioCh := make(chan []float64)
	rewardCh := make(chan [][]argUint64)

	// Process baseFeePerGas asynchronously
	go func() {
		baseFeePerGasCh <- convertToArgUint64Slice(history.BaseFeePerGas)
	}()

	// Process gasUsedRatio asynchronously
	go func() {
		gasUsedRatioCh <- history.GasUsedRatio
	}()

	// Process reward asynchronously
	go func() {
		rewardCh <- convertToArgUint64SliceSlice(history.Reward)
	}()

	// Wait for the processed slices from goroutines
	baseFeePerGasResult := <-baseFeePerGasCh
	gasUsedRatioResult := <-gasUsedRatioCh
	rewardResult := <-rewardCh

	result := &feeHistoryResult{
		OldestBlock:   *argUintPtr(history.OldestBlock),
		BaseFeePerGas: baseFeePerGasResult,
		GasUsedRatio:  gasUsedRatioResult,
		Reward:        rewardResult,
	}

	return result, nil
}
