package jsonrpc

import (
	"math/big"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

// blockchain is the interface with the blockchain required
// by the filter manager
type blockchainInterface interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// GetReceiptsByHash returns the receipts for a hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)

	// Subscribe subscribes for chain head events
	SubscribeEvents() blockchain.Subscription

	// GetHeaderByNumber returns the header by number
	GetHeaderByNumber(block uint64) (*types.Header, bool)

	// GetAvgGasPrice returns the average gas price
	GetAvgGasPrice() *big.Int

	// AddTx adds a new transaction to the tx pool
	AddTx(tx *types.Transaction) error

	// State returns a reference to the state
	State() state.State

	//// GetStateHelper returns a helper for state related functions
	//GetStateHelper() *state.StateHelper

	// BeginTxn starts a transition object
	BeginTxn(parentRoot types.Hash, header *types.Header) (*state.Transition, error)

	// GetBlockByHash gets a block using the provided hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)
}

type nullBlockchainInterface struct {
}

func (b *nullBlockchainInterface) Header() *types.Header {
	return nil
}

func (b *nullBlockchainInterface) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return nil, nil
}

func (b *nullBlockchainInterface) SubscribeEvents() blockchain.Subscription {
	return nil
}

func (b *nullBlockchainInterface) GetHeaderByNumber(block uint64) (*types.Header, bool) {
	return nil, false
}

func (b *nullBlockchainInterface) GetAvgGasPrice() *big.Int {
	return nil
}

func (b *nullBlockchainInterface) AddTx(tx *types.Transaction) error {
	return nil
}

func (b *nullBlockchainInterface) State() state.State {
	return nil
}

//func (b *nullBlockchainInterface) GetStateHelper() *state.StateHelperInterface {
//	return nil
//}

func (b *nullBlockchainInterface) BeginTxn(parentRoot types.Hash, header *types.Header) (*state.Transition, error) {
	return nil, nil
}

func (b *nullBlockchainInterface) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	return nil, false
}
