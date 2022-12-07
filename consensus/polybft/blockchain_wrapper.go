package polybft

import (
	"fmt"
	"math/big"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

const (
	consensusSource = "consensus"
)

// blockchain is an interface that wraps the methods called on blockchain
type blockchainBackend interface {
	// CurrentHeader returns the header of blockchain block head
	CurrentHeader() *types.Header

	// CommitBlock commits a block to the chain.
	CommitBlock(block *types.Block) ([]*types.Receipt, error)

	// NewBlockBuilder is a factory method that returns a block builder on top of 'parent'.
	NewBlockBuilder(parent *types.Header, coinbase types.Address,
		txPool txPoolInterface, blockTime time.Duration, logger hclog.Logger) (blockBuilder, error)

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
	GetSystemState(validatorSetAddr types.Address, stateReceiverAddr types.Address, provider contract.Provider) SystemState

	SubscribeEvents() blockchain.Subscription

	// GetChainID returns chain id of the current blockchain
	GetChainID() uint64
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
func (p *blockchainWrapper) CommitBlock(block *types.Block) ([]*types.Receipt, error) {
	err := p.blockchain.WriteBlock(block, consensusSource)
	if err != nil {
		return nil, err
	}

	return p.blockchain.GetCachedReceipts(block.Hash())
}

// ProcessBlock builds a final block from given 'block' on top of 'parent'
func (p *blockchainWrapper) ProcessBlock(parent *types.Header, block *types.Block) (*StateBlock, error) {
	// TODO: Call validate block in polybft
	header := block.Header.Copy()

	transition, err := p.executor.BeginTxn(parent.StateRoot, header, types.BytesToAddress(header.Miner))
	if err != nil {
		return nil, err
	}

	// apply transactions from block
	for _, tx := range block.Transactions {
		if err := transition.Write(tx); err != nil {
			return nil, fmt.Errorf("process block tx error, tx = %v, err = %w", tx.Hash, err)
		}
	}

	_, root := transition.Commit()

	if root != block.Header.StateRoot {
		return nil, fmt.Errorf("incorrect state root: (%s, %s)", root, block.Header.StateRoot)
	}

	// build the block
	builtBlock := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   header,
		Txns:     block.Transactions,
		Receipts: transition.Receipts(),
	})

	return &StateBlock{
		Block:    builtBlock,
		Receipts: transition.Receipts(),
		State:    transition,
	}, nil
}

// GetStateProviderForBlock is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetStateProviderForBlock(header *types.Header) (contract.Provider, error) {
	transition, err := p.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
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
	parent *types.Header, coinbase types.Address,
	txPool txPoolInterface, blockTime time.Duration, logger hclog.Logger) (blockBuilder, error) {
	gasLimit, err := p.blockchain.CalculateGasLimit(parent.Number + 1)
	if err != nil {
		return nil, err
	}

	return NewBlockBuilder(&BlockBuilderParams{
		BlockTime: blockTime,
		Parent:    parent,
		Coinbase:  coinbase,
		Executor:  p.executor,
		GasLimit:  gasLimit,
		TxPool:    txPool,
		Logger:    logger,
	})
}

// GetSystemState is an implementation of blockchainBackend interface
func (p *blockchainWrapper) GetSystemState(
	validatorSetAddr types.Address,
	stateReceiverAddr types.Address,
	provider contract.Provider) SystemState {
	return NewSystemState(validatorSetAddr, stateReceiverAddr, provider)
}

func (p *blockchainWrapper) SubscribeEvents() blockchain.Subscription {
	return p.blockchain.SubscribeEvents()
}

func (p *blockchainWrapper) GetChainID() uint64 {
	return uint64(p.blockchain.Config().ChainID)
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
	result := s.transition.Call2(contracts.SystemCaller, types.Address(addr), input, big.NewInt(0), 10000000)
	if result.Failed() {
		return nil, result.Err
	}

	return result.ReturnValue, nil
}

// Txn is part of the contract.Provider interface to make Ethereum transactions. We disable this function
// since the system state does not make any transaction
func (s *stateProvider) Txn(ethgo.Address, ethgo.Key, []byte) (contract.Txn, error) {
	panic("we do not make transaction in system state")
}
