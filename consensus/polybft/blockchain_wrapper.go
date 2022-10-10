package polybft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
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
	// CurrentHeader returns the header of blockchain block head
	CurrentHeader() *types.Header

	// CommitBlock commits a block to the chain.
	CommitBlock(stateBlock *StateBlock) error

	// NewBlockBuilder is a factory method that returns a block builder on top of 'parent'.
	NewBlockBuilder(parent *types.Header, coinbase types.Address) (blockBuilder, error)

	// ProcessBlock builds a final block from given 'block' on top of 'parent'.
	ProcessBlock(parent *types.Header, block *types.Block) (*StateBlock, error)

	// GetStateProviderForBlock returns a reference to make queries to the state at 'block'.
	GetStateProviderForBlock(block *types.Header) (contract.Provider, error)

	// GetStateProvider returns a reference to make queries to the provided state.
	GetStateProvider(transition *state.Transition) contract.Provider

	// GetHeaderByNumber returns a reference to block header for the given block number.
	GetHeaderByNumber(number uint64) (*types.Header, bool)

	// GetHeaderByHash returns a reference to block header for the given block hash
	GetHeaderByHash(hash types.Hash) (*types.Header, bool)

	// GetSystemState creates a new instance of SystemState interface
	GetSystemState(config *PolyBFTConfig, provider contract.Provider) SystemState

	SubscribeEvents() blockchain.Subscription
}

var _ blockchainBackend = &blockchainWrapper{}

type blockchainWrapper struct {
	executor   *state.Executor
	blockchain *blockchain.Blockchain
}

// CurrentHeader returns the header of blockchain block head
func (p *blockchainWrapper) CurrentHeader() *types.Header {
	return p.blockchain.Header()
}

// CommitBlock commits a block to the chain
func (p *blockchainWrapper) CommitBlock(stateBlock *StateBlock) error {
	// logs := buildLogsFromReceipts(stateBlock.Receipts, stateBlock.Block.GetHeader())
	// status, err := p.blockchain.WriteBlockAndSetHead(stateBlock.Block, stateBlock.Receipts, logs, stateBlock.State, true)
	// if err != nil {
	// 	return err
	// } else if status != core.CanonStatTy {
	// 	return fmt.Errorf("non canonical change")
	// }
	return p.blockchain.WriteBlock(stateBlock.Block, "consensus")
}

// ProcessBlock builds a final block from given 'block' on top of 'parent'
func (p *blockchainWrapper) ProcessBlock(parent *types.Header, block *types.Block) (*StateBlock, error) {
	// TODO: Call validate block in polybft
	header := block.Header.Copy()

	//transition, err := p.executor.BeginTxn(parent.Hash, header, b.params.Coinbase)
	transition, err := p.executor.BeginTxn(parent.Hash, header, types.Address{})
	if err != nil {
		return nil, err
	}

	// Nemanja - apply transactions from block

	_, root := transition.Commit()

	if root != block.Header.StateRoot {
		return nil, fmt.Errorf("incorrect state root: (%s, %s)", root, block.Header.StateRoot)
	}

	// build the block
	found := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   header,
		Txns:     block.Transactions,
		Receipts: transition.Receipts(),
	})

	return &StateBlock{
		Block:    found,
		Receipts: transition.Receipts(),
		State:    transition,
	}, nil
}

// GetStateProviderForBlock is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetStateProviderForBlock(header *types.Header) (contract.Provider, error) {
	parentHeader, found := p.GetHeaderByHash(header.ParentHash)
	if !found {
		return nil, fmt.Errorf("failed to retrieve parent header for hash %s", header.ParentHash.String())
	}

	transition, err := p.executor.BeginTxn(parentHeader.StateRoot, header, types.ZeroAddress)
	if err != nil {
		return nil, err
	}

	return NewStateProvider(transition), nil
}

// GetStateProvider returns a reference to make queries to the provided state
func (p *blockchainWrapper) GetStateProvider(transition *state.Transition) contract.Provider {
	return NewStateProvider(transition)
}

// GetHeaderByNumber is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetHeaderByNumber(number uint64) (*types.Header, bool) {
	return p.blockchain.GetHeaderByNumber(number)
}

// GetHeaderByHash is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return p.blockchain.GetHeaderByHash(hash)
}

// NewBlockBuilder is an implementation of blockchainBackend interface
func (p *blockchainWrapper) NewBlockBuilder(
	parent *types.Header, coinbase types.Address) (blockBuilder, error) {
	return NewBlockBuilder(&BlockBuilderParams{
		Parent:      parent,
		Coinbase:    coinbase,
		ChainConfig: p.blockchain.Config(),
		//ChainContext:  p.blockchain,
		//TxPoolFactory: blockbuilder.NewEthTxPool(p.eth.TxPool()),
		Executor: p.executor,
		GasLimit: 10000000, // TODO: Nemanja - see what to do with this (p.eth.GenesisGasLimit(),)
	}), nil
}

// GetSystemState is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetSystemState(config *PolyBFTConfig, provider contract.Provider) SystemState {
	return NewSystemState(config, provider)
}

func (p *blockchainWrapper) SubscribeEvents() blockchain.Subscription {
	return p.blockchain.SubscribeEvents()
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
