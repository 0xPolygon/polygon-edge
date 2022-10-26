package jsonrpc

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

type endpointHelperTxPoolStore interface {
	// GetNonce returns the next nonce for this address
	GetNonce(addr types.Address) uint64
}

type endpointHelperBlockchainStore interface {
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
	GetBlockByHash(types.Hash, bool) (*types.Block, bool)
}

type endpointHelperStateStore interface {
	GetAccount(root types.Hash, addr types.Address) (*state.Account, error)
}

type endpointHelperStore interface {
	endpointHelperTxPoolStore
	endpointHelperStateStore
	endpointHelperBlockchainStore
}

// baseEndpoint implements the common methods to be used in endpoint handlers
type endpointHelper struct {
	store endpointHelperStore
}

func (h *endpointHelper) getNumericBlockNumber(number BlockNumber) (uint64, error) {
	switch number {
	case LatestBlockNumber:
		return h.store.Header().Number, nil

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

func (h *endpointHelper) getBlockHeader(number BlockNumber) (*types.Header, error) {
	switch number {
	case LatestBlockNumber:
		return h.store.Header(), nil

	case EarliestBlockNumber:
		header, ok := h.store.GetHeaderByNumber(uint64(0))
		if !ok {
			return nil, fmt.Errorf("error fetching genesis block header")
		}

		return header, nil

	case PendingBlockNumber:
		return nil, fmt.Errorf("fetching the pending header is not supported")

	default:
		// Convert the block number from hex to uint64
		header, ok := h.store.GetHeaderByNumber(uint64(number))
		if !ok {
			return nil, fmt.Errorf("error fetching block number %d header", uint64(number))
		}

		return header, nil
	}
}

func (h *endpointHelper) getHeaderFromBlockNumberOrHash(bnh *BlockNumberOrHash) (*types.Header, error) {
	var (
		header *types.Header
		err    error
	)

	if bnh.BlockNumber != nil {
		header, err = h.getBlockHeader(*bnh.BlockNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to get the header of block %d: %w", *bnh.BlockNumber, err)
		}
	} else if bnh.BlockHash != nil {
		block, ok := h.store.GetBlockByHash(*bnh.BlockHash, false)
		if !ok {
			return nil, fmt.Errorf("could not find block referenced by the hash %s", bnh.BlockHash.String())
		}

		header = block.Header
	}

	return header, nil
}

func (h *endpointHelper) getNextNonce(address types.Address, number BlockNumber) (uint64, error) {
	if number == PendingBlockNumber {
		// Grab the latest pending nonce from the TxPool
		//
		// If the account is not initialized in the local TxPool,
		// return the latest nonce from the world state
		res := h.store.GetNonce(address)

		return res, nil
	}

	header, err := h.getBlockHeader(number)
	if err != nil {
		return 0, err
	}

	acc, err := h.store.GetAccount(header.StateRoot, address)

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

func (h *endpointHelper) decodeTxn(arg *txnArgs) (*types.Transaction, error) {
	// set default values
	if arg.From == nil {
		arg.From = &types.ZeroAddress
		arg.Nonce = argUintPtr(0)
	} else if arg.Nonce == nil {
		// get nonce from the pool
		nonce, err := h.getNextNonce(*arg.From, LatestBlockNumber)
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
