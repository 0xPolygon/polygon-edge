package polybft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/maticnetwork/bor/core"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

// // ethereumBackend is an interface that wraps the methods called on ethereum backend protocol
// type ethereumBackend interface {
// 	// PeersLen returns the number of peers the node is connected to
// 	PeersLen() int
// 	// TxPool returns the current transaction pool
// 	TxPool() *core.TxPool
// 	// Broadcast block broadcasts the newly inserted block to the rest of the peers
// 	BroadcastBlock(block *types.Block, propagate bool)
// 	// GenesisGasLimit returns the initial gas limit for a block
// 	GenesisGasLimit() uint64
// }

// blockchain is an interface that wraps the methods called on blockchain
type blockchainBackend interface {
	// OnNewBlockInserted is invoked when new block is finalized by consensus protocol.
	OnNewBlockInserted(block *types.Block)

	// CurrentHeader returns the header of blockchain block head
	CurrentHeader() *types.Header

	// CommitBlock commits a block to the chain.
	CommitBlock(stateBlock *StateBlock) error

	// NewBlockBuilder is a factory method that returns a block builder on top of 'parent'.
	NewBlockBuilder(parent *types.Header) (blockBuilder, error)

	// ProcessBlock builds a final block from given 'block' on top of 'parent'.
	ProcessBlock(parent *types.Header, block *types.Block) (*StateBlock, error)

	// GetStateProviderForBlock returns a reference to make queries to the state at 'block'.
	GetStateProviderForBlock(block *types.Header) (contract.Provider, error)

	// GetStateProviderForDB returns a reference to make queries to the provided state.
	GetStateProviderForDB(transition *state.Transition) contract.Provider

	// GetHeaderByNumber returns a reference to block header for the given block number.
	GetHeaderByNumber(number uint64) (*types.Header, bool)

	// GetHeaderByHash returns a reference to block header for the given block hash
	GetHeaderByHash(hash types.Hash) (*types.Header, bool)

	// SubscribeChainHeadEvent subscribes to block insert event on chain.
	// SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription

	// GetSystemState creates a new instance of SystemState interface
	GetSystemState(config *PolyBFTConfig, provider contract.Provider) SystemState

	// PeersLen returns the number of peers the node is connected to
	//PeersLen() int

	// SetCoinbase sets the coinbase data
	SetCoinbase(coinbase types.Address)
}

var _ blockchainBackend = &blockchainWrapper{}

type blockchainWrapper struct {
	blockchain *blockchain.Blockchain
	//eth        ethereumBackend
	coinbase types.Address
}

// CurrentHeader returns the header of blockchain block head
func (p *blockchainWrapper) CurrentHeader() *types.Header {
	return p.blockchain.Header()
}

// CommitBlock commits a block to the chain
func (p *blockchainWrapper) CommitBlock(stateBlock *StateBlock) error {
	logs := buildLogsFromReceipts(stateBlock.Receipts, stateBlock.Block.GetHeader())
	status, err := p.blockchain.WriteBlockAndSetHead(stateBlock.Block, stateBlock.Receipts, logs, stateBlock.State, true)
	if err != nil {
		return err
	} else if status != core.CanonStatTy {
		return fmt.Errorf("non canonical change")
	}
	return nil
}

// PeersLen returns the number of peers the node is connected to
// func (p *blockchainWrapper) PeersLen() int {
// 	return p.eth.PeersLen()
// }

// SetCoinbase sets the coinbase data
func (p *blockchainWrapper) SetCoinbase(coinbase types.Address) {
	p.coinbase = coinbase
}

// ProcessBlock builds a final block from given 'block' on top of 'parent'
func (p *blockchainWrapper) ProcessBlock(parent *types.Header, block *types.Block) (*blockbuilder.StateBlock, error) {
	// TODO: Call validate block in polybft

	state, err := p.blockchain.StateAt(parent.Root)
	if err != nil {
		return nil, err
	}

	// TO DO Nemanja - no transactions for now, leave empty receipts
	var receipts []*types.Receipt

	// var usedGas uint64
	// gasPool := core.GasPool(block.GasLimit)
	// header := block.Header()

	// for index, txn := range block.Transactions() {
	// 	state.Prepare(txn.Hash(), index)
	// 	receipt, err := core.ApplyTransaction(
	// 		p.blockchain.Config(),
	// 		p.blockchain,
	// 		&header.Coinbase,
	// 		&gasPool,
	// 		state,
	// 		header,
	// 		txn,
	// 		&usedGas,
	// 		vm.Config{},
	// 	)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	receipts = append(receipts, receipt)
	// }

	// build the state
	root := state.IntermediateRoot(true)
	if root != block.Header.StateRoot {
		return nil, fmt.Errorf("incorrect state root: (%s, %s)", root, block.Root())
	}

	// build the final block: Nemanja it is the same as propsal since there is no transactions
	found := NewFinalBlock(header, block.Transactions, receipts)
	if found.Hash() != block.Hash() {
		return nil, fmt.Errorf("incorrect block hash: (%s, %s)", found.Hash(), block.Hash())
	}

	builtBlock := &blockbuilder.StateBlock{
		Block:    found,
		Receipts: receipts,
		State:    state,
	}
	return builtBlock, nil
}

// SubscribeChainHeadEvent is an implementation of blockchain interface
// func (p *blockchainWrapper) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
// 	return p.blockchain.SubscribeChainHeadEvent(ch)
// }

// StateAt is an implementation of blockchain interface
func (p *blockchainWrapper) GetStateProviderForBlock(block *types.Header) (contract.Provider, error) {
	// TODO: executor.BeginTxn(block.StateRoot,...)
	// this returns transition object for given StateRoot
	state, err := p.blockchain.StateAt(block.StateRoot)
	if err != nil {
		return nil, fmt.Errorf("state not found") // this is critical
	}

	return NewStateProvider(transition), nil
}

// GetStateProviderForDB returns a reference to make queries to the provided state
func (p *blockchainWrapper) GetStateProviderForDB(transition *state.Transition) contract.Provider {
	return NewStateProvider(transition)
}

// GetHeaderByNumber is an implementation of blockchain interface
func (p *blockchainWrapper) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	return p.blockchain.GetHeaderByNumber(number)
}

// GetHeaderByHash is an implementation of blockchain interface
func (p *blockchainWrapper) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return p.blockchain.GetHeaderByHash(hash)
}

// NewBlockBuilder is an implementation of blockchain interface
func (p *blockchainWrapper) NewBlockBuilder(parent *types.Header) (blockBuilder, error) {
	stt, err := p.blockchain.StateAt(parent.Root)
	if err != nil {
		return nil, fmt.Errorf("state not found") // this is critical
	}

	return NewBlockBuilder(&BlockBuilderParams{
		Parent:      parent,
		Coinbase:    p.coinbase,
		ChainConfig: p.blockchain.Config(),
		//ChainContext:  p.blockchain,
		//TxPoolFactory: blockbuilder.NewEthTxPool(p.eth.TxPool()),
		StateDB:  stt,
		GasLimit: 100000000000, // TO DO Nemanja - see what to do with this (p.eth.GenesisGasLimit(),)
	}), nil
}

// GetSystemState is an implementation of blockchain interface
func (p *blockchainWrapper) GetSystemState(config *params.PolyBFTConfig, provider contract.Provider) SystemState {
	return NewSystemState(config, provider)
}

// OnNewBlockInserted is an implementation of blockchain interface
func (p *blockchainWrapper) OnNewBlockInserted(block *types.Block) {
	// TO DO Nemanja - probably we do not need this method
	//p.eth.BroadcastBlock(block, true)
}

var _ contract.Provider = &stateProvider{}

type stateProvider struct {
	transition *state.Transition
}

// NewStateProvider initializes EVM against given state and chain config and returns stateProvider instance
// which is an abstraction for smart contract calls
func NewStateProvider(transition *state.Transition) contract.Provider {
	return &stateProvider{transition: transition}
}

// Call implements the contract.Provider interface to make contract calls directly to the state
func (s *stateProvider) Call(addr ethgo.Address, input []byte, opts *contract.CallOpts) ([]byte, error) {
	result := s.transition.Call2(types.ZeroAddress, types.Address(addr), input, big.NewInt(0), 10000000)
	if result.Err != nil {
		return nil, result.Err
	}
	return result.ReturnValue, nil
}

// Txn is part of the contract.Provider interface to make Ethereum transactions. We disable this function
// since the system state does not make any transaction
func (s *stateProvider) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	panic("we do not make transaction in system state")
}
