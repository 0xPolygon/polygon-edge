package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	ErrHeaderNotFound           = errors.New("header not found")
	ErrLatestNotFound           = errors.New("latest header not found")
	ErrNegativeBlockNumber      = errors.New("invalid argument 0: block number must not be negative")
	ErrFailedFetchGenesis       = errors.New("error fetching genesis block header")
	ErrNoDataInContractCreation = errors.New("contract creation without data provided")
)

type latestHeaderGetter interface {
	Header() *types.Header
}

// GetNumericBlockNumber returns block number based on current state or specified number
func GetNumericBlockNumber(number BlockNumber, store latestHeaderGetter) (uint64, error) {
	switch number {
	case LatestBlockNumber, PendingBlockNumber:
		latest := store.Header()
		if latest == nil {
			return 0, ErrLatestNotFound
		}

		return latest.Number, nil

	case EarliestBlockNumber:
		return 0, nil

	default:
		if number < 0 {
			return 0, ErrNegativeBlockNumber
		}

		return uint64(number), nil
	}
}

type headerGetter interface {
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
}

// GetBlockHeader returns a header using the provided number
func GetBlockHeader(number BlockNumber, store headerGetter) (*types.Header, error) {
	switch number {
	case PendingBlockNumber, LatestBlockNumber:
		return store.Header(), nil

	case EarliestBlockNumber:
		header, ok := store.GetHeaderByNumber(uint64(0))
		if !ok {
			return nil, ErrFailedFetchGenesis
		}

		return header, nil

	default:
		// Convert the block number from hex to uint64
		header, ok := store.GetHeaderByNumber(uint64(number))
		if !ok {
			return nil, fmt.Errorf("error fetching block number %d header", uint64(number))
		}

		return header, nil
	}
}

type txLookupAndBlockGetter interface {
	ReadTxLookup(types.Hash) (types.Hash, bool)
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
}

// GetTxAndBlockByTxHash returns the tx and the block including the tx by given tx hash
func GetTxAndBlockByTxHash(txHash types.Hash, store txLookupAndBlockGetter) (*types.Transaction, *types.Block) {
	blockHash, ok := store.ReadTxLookup(txHash)
	if !ok {
		return nil, nil
	}

	block, ok := store.GetBlockByHash(blockHash, true)
	if !ok {
		return nil, nil
	}

	for _, txn := range block.Transactions {
		if txn.Hash == txHash {
			return txn, block
		}
	}

	return nil, nil
}

type blockGetter interface {
	headerGetter
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
}

func GetHeaderFromBlockNumberOrHash(bnh BlockNumberOrHash, store blockGetter) (*types.Header, error) {
	// The filter is empty, use the latest block by default
	if bnh.BlockNumber == nil && bnh.BlockHash == nil {
		bnh.BlockNumber, _ = createBlockNumberPointer(latest)
	}

	if bnh.BlockNumber != nil {
		// block number
		header, err := GetBlockHeader(*bnh.BlockNumber, store)
		if err != nil {
			return nil, fmt.Errorf("failed to get the header of block %d: %w", *bnh.BlockNumber, err)
		}

		return header, nil
	}

	// block hash
	block, ok := store.GetBlockByHash(*bnh.BlockHash, false)
	if !ok {
		return nil, fmt.Errorf("could not find block referenced by the hash %s", bnh.BlockHash.String())
	}

	return block.Header, nil
}

type dataGetter interface {
	headerGetter
	GetNonce(types.Address) uint64
	GetBaseFee() uint64
	GetAccount(root types.Hash, addr types.Address) (*Account, error)
}

func GetNextNonce(address types.Address, number BlockNumber, store dataGetter) (uint64, error) {
	if number == PendingBlockNumber {
		// Grab the latest pending nonce from the TxPool
		//
		// If the account is not initialized in the local TxPool,
		// return the latest nonce from the world state
		res := store.GetNonce(address)

		return res, nil
	}

	header, err := GetBlockHeader(number, store)
	if err != nil {
		return 0, err
	}

	acc, err := store.GetAccount(header.StateRoot, address)

	//nolint:govet
	if errors.Is(err, ErrStateNotFound) {
		// If the account doesn't exist / isn't initialized,
		// return a nonce value of 0
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return acc.Nonce, nil
}

func DecodeTxn(arg *txnArgs, store dataGetter) (*types.Transaction, error) {
	// set default values
	if arg.From == nil {
		arg.From = &types.ZeroAddress
		arg.Nonce = argUintPtr(0)
	} else if arg.Nonce == nil {
		// get nonce from the pool
		nonce, err := GetNextNonce(*arg.From, LatestBlockNumber, store)
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

	if arg.GasTipCap == nil {
		arg.GasTipCap = argBytesPtr([]byte{})
	}

	if arg.GasFeeCap == nil {
		arg.GasFeeCap = argBytesPtr([]byte{})
	}

	var input []byte
	if arg.Data != nil {
		input = *arg.Data
	} else if arg.Input != nil {
		input = *arg.Input
	}

	if arg.To == nil && input == nil {
		return nil, ErrNoDataInContractCreation
	}

	if input == nil {
		input = []byte{}
	}

	if arg.Gas == nil {
		arg.Gas = argUintPtr(0)
	}

	txn := &types.Transaction{
		From:      *arg.From,
		Gas:       uint64(*arg.Gas),
		GasPrice:  new(big.Int).SetBytes(*arg.GasPrice),
		GasTipCap: new(big.Int).SetBytes(*arg.GasTipCap),
		GasFeeCap: new(big.Int).SetBytes(*arg.GasFeeCap),
		Value:     new(big.Int).SetBytes(*arg.Value),
		Input:     input,
		Nonce:     uint64(*arg.Nonce),
	}

	if arg.To != nil {
		txn.To = arg.To
	}

	txn = fillTxFees(txn, store.GetBaseFee())

	txn.ComputeHash()

	return txn, nil
}

// fillTxFees fills fee-related fields depending on the provided input.
// Basically, there must be either gas price OR gas fee cap and gas tip cap provided.
//
// Here is the logic:
//   - use gas price for gas tip cap and gas fee cap if base fee is nil;
//   - otherwise, if base fee is not provided:
//   - use gas price for gas tip cap and gas fee cap if gas price is not nil;
//   - otherwise, if base tip cap and base fee cap are provided:
//   - gas price should be min(gasFeeCap, gasTipCap * baseFee);
func fillTxFees(tx *types.Transaction, baseFee uint64) *types.Transaction {
	if baseFee == 0 {
		// If there's no basefee, then it must be a non-1559 execution
		if tx.GasPrice == nil {
			tx.GasPrice = new(big.Int)
		}

		tx.GasFeeCap = new(big.Int).Set(tx.GasPrice)
		tx.GasTipCap = new(big.Int).Set(tx.GasPrice)

		return tx
	}

	// A basefee is provided, necessitating 1559-type execution
	if tx.GasPrice != nil {
		// User specified the legacy gas field, convert to 1559 gas typing
		tx.GasFeeCap = new(big.Int).Set(tx.GasPrice)
		tx.GasTipCap = new(big.Int).Set(tx.GasPrice)

		return tx
	}

	// User specified 1559 gas feilds (or none), use those
	if tx.GasFeeCap == nil {
		tx.GasFeeCap = new(big.Int)
	}

	if tx.GasTipCap == nil {
		tx.GasTipCap = new(big.Int)
	}

	// Backfill the legacy gasPrice for EVM execution, unless we're all zeroes
	tx.GasPrice = new(big.Int)

	if tx.GasFeeCap.BitLen() > 0 || tx.GasTipCap.BitLen() > 0 {
		tx.GasPrice = common.BigMin(
			new(big.Int).Add(
				tx.GasTipCap,
				new(big.Int).SetUint64(baseFee),
			),
			tx.GasFeeCap,
		)
	}

	return tx
}
