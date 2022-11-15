package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	ErrHeaderNotFound = errors.New("header not found")
)

type latestHeaderGetter interface {
	Header() *types.Header
}

// GetNumericBlockNumber returns block number based on current state or specified number
func GetNumericBlockNumber(number BlockNumber, store latestHeaderGetter) (uint64, error) {
	switch number {
	case LatestBlockNumber:
		return store.Header().Number, nil

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

type headerGetter interface {
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
}

// GetBlockHeader returns a header using the provided number
func GetBlockHeader(number BlockNumber, store headerGetter) (*types.Header, error) {
	switch number {
	case LatestBlockNumber:
		return store.Header(), nil

	case EarliestBlockNumber:
		header, ok := store.GetHeaderByNumber(uint64(0))
		if !ok {
			return nil, fmt.Errorf("error fetching genesis block header")
		}

		return header, nil

	case PendingBlockNumber:
		return nil, fmt.Errorf("fetching the pending header is not supported")

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
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
}

func GetHeaderFromBlockNumberOrHash(bnh BlockNumberOrHash, store blockGetter) (*types.Header, error) {
	var (
		header *types.Header
		err    error
	)

	if bnh.BlockNumber != nil {
		header, err = GetBlockHeader(*bnh.BlockNumber, store)
		if err != nil {
			return nil, fmt.Errorf("failed to get the header of block %d: %w", *bnh.BlockNumber, err)
		}
	} else if bnh.BlockHash != nil {
		block, ok := store.GetBlockByHash(*bnh.BlockHash, false)
		if !ok {
			return nil, fmt.Errorf("could not find block referenced by the hash %s", bnh.BlockHash.String())
		}

		header = block.Header
	}

	if header == nil {
		return nil, ErrHeaderNotFound
	}

	return header, nil
}

type nonceGetter interface {
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
	GetNonce(types.Address) uint64
	GetAccount(root types.Hash, addr types.Address) (*Account, error)
}

func GetNextNonce(address types.Address, number BlockNumber, store nonceGetter) (uint64, error) {
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
	if errors.As(err, &ErrStateNotFound) {
		// If the account doesn't exist / isn't initialized,
		// return a nonce value of 0
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return acc.Nonce, nil
}

func DecodeTxn(arg *txnArgs, store nonceGetter) (*types.Transaction, error) {
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
