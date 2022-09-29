package polybft

/*
import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	blockbuilder "github.com/ethereum/go-ethereum/internal/block-builder"
	"github.com/ethereum/go-ethereum/internal/bls"
	"github.com/ethereum/go-ethereum/params"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/contract"
)

// ethereumBackend is an interface that wraps the methods called on ethereum backend protocol
type ethereumBackend interface {
	// PeersLen returns the number of peers the node is connected to
	PeersLen() int
	// TxPool returns the current transaction pool
	TxPool() *core.TxPool
	// Broadcast block broadcasts the newly inserted block to the rest of the peers
	BroadcastBlock(block *types.Block, propagate bool)
	// GenesisGasLimit returns the initial gas limit for a block
	GenesisGasLimit() uint64
}

// blockchain is an interface that wraps the methods called on blockchain
type blockchain interface {
	// OnNewBlockInserted is invoked when new block is finalized by consensus protocol.
	OnNewBlockInserted(block *types.Block)

	// CurrentHeader returns the header of blockchain block head
	CurrentHeader() *types.Header

	// CommitBlock commits a block to the chain.
	CommitBlock(stateBlock *blockbuilder.StateBlock) error

	// NewBlockBuilder is a factory method that returns a block builder on top of 'parent'.
	NewBlockBuilder(parent *types.Header) (blockBuilder, error)

	// ProcessBlock builds a final block from given 'block' on top of 'parent'.
	ProcessBlock(parent *types.Header, block *types.Block) (*blockbuilder.StateBlock, error)

	// GetStateProviderForBlock returns a reference to make queries to the state at 'block'.
	GetStateProviderForBlock(block *types.Header) (contract.Provider, error)

	// GetStateProviderForDB returns a reference to make queries to the provided state.
	GetStateProviderForDB(state vm.StateDB) contract.Provider

	// GetHeaderByNumber returns a reference to block header for the given block number.
	GetHeaderByNumber(number uint64) *types.Header

	// GetHeaderByHash returns a reference to block header for the given block hash
	GetHeaderByHash(hash common.Hash) *types.Header

	// SubscribeChainHeadEvent subscribes to block insert event on chain.
	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription

	// GetSystemState creates a new instance of SystemState interface
	GetSystemState(config *params.PolyBFTConfig, provider contract.Provider) SystemState

	// PeersLen returns the number of peers the node is connected to
	PeersLen() int

	// SetCoinbase sets the coinbase data
	SetCoinbase(coinbase common.Address)
}

var _ blockchain = &blockchainWrapper{}

type blockchainWrapper struct {
	blockchain *core.BlockChain
	eth        ethereumBackend
	coinbase   common.Address
}

// CurrentHeader returns the header of blockchain block head
func (p *blockchainWrapper) CurrentHeader() *types.Header {
	return p.blockchain.CurrentHeader()
}

// CommitBlock commits a block to the chain
func (p *blockchainWrapper) CommitBlock(stateBlock *blockbuilder.StateBlock) error {
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
func (p *blockchainWrapper) PeersLen() int {
	return p.eth.PeersLen()
}

// SetCoinbase sets the coinbase data
func (p *blockchainWrapper) SetCoinbase(coinbase common.Address) {
	p.coinbase = coinbase
}

// ProcessBlock builds a final block from given 'block' on top of 'parent'
func (p *blockchainWrapper) ProcessBlock(parent *types.Header, block *types.Block) (*blockbuilder.StateBlock, error) {
	// TODO: Call validate block in polybft

	state, err := p.blockchain.StateAt(parent.Root)
	if err != nil {
		return nil, err
	}

	header := block.Header()

	var usedGas uint64
	gasPool := core.GasPool(block.GasLimit())

	var receipts []*types.Receipt
	for index, txn := range block.Transactions() {
		state.Prepare(txn.Hash(), index)
		receipt, err := core.ApplyTransaction(
			p.blockchain.Config(),
			p.blockchain,
			&header.Coinbase,
			&gasPool,
			state,
			header,
			txn,
			&usedGas,
			vm.Config{},
		)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}

	// build the state
	root := state.IntermediateRoot(true)
	if root != block.Root() {
		return nil, fmt.Errorf("incorrect state root: (%s, %s)", root, block.Root())
	}

	// build the final block
	found := blockbuilder.NewFinalBlock(header, block.Transactions(), receipts)
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
func (p *blockchainWrapper) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return p.blockchain.SubscribeChainHeadEvent(ch)
}

// StateAt is an implementation of blockchain interface
func (p *blockchainWrapper) GetStateProviderForBlock(block *types.Header) (contract.Provider, error) {
	state, err := p.blockchain.StateAt(block.Root)
	if err != nil {
		return nil, fmt.Errorf("state not found") // this is critical
	}

	return NewStateProvider(state, p.blockchain.Config()), nil
}

// GetStateProviderForDB returns a reference to make queries to the provided state
func (p *blockchainWrapper) GetStateProviderForDB(state vm.StateDB) contract.Provider {
	return NewStateProvider(state, p.blockchain.Config())
}

// GetHeaderByNumber is an implementation of blockchain interface
func (p *blockchainWrapper) GetHeaderByNumber(number uint64) *types.Header {
	return p.blockchain.GetHeaderByNumber(number)
}

// GetHeaderByHash is an implementation of blockchain interface
func (p *blockchainWrapper) GetHeaderByHash(hash common.Hash) *types.Header {
	return p.blockchain.GetHeaderByHash(hash)
}

// NewBlockBuilder is an implementation of blockchain interface
func (p *blockchainWrapper) NewBlockBuilder(parent *types.Header) (blockBuilder, error) {
	stt, err := p.blockchain.StateAt(parent.Root)
	if err != nil {
		return nil, fmt.Errorf("state not found") // this is critical
	}

	return blockbuilder.NewBlockBuilder(&blockbuilder.Params{
		Parent:        parent,
		Coinbase:      p.coinbase,
		ChainConfig:   p.blockchain.Config(),
		ChainContext:  p.blockchain,
		TxPoolFactory: blockbuilder.NewEthTxPool(p.eth.TxPool()),
		StateDB:       stt,
		GasLimit:      p.eth.GenesisGasLimit(),
	}), nil
}

// GetSystemState is an implementation of blockchain interface
func (p *blockchainWrapper) GetSystemState(config *params.PolyBFTConfig, provider contract.Provider) SystemState {
	return NewSystemState(config, provider)
}

// OnNewBlockInserted is an implementation of blockchain interface
func (p *blockchainWrapper) OnNewBlockInserted(block *types.Block) {
	p.eth.BroadcastBlock(block, true)
}

type stateProvider struct {
	vm *vm.EVM
}

// NewStateProvider initializes EVM against given state and chain config and returns stateProvider instance
// which is an abstraction for smart contract calls
func NewStateProvider(state vm.StateDB, config *params.ChainConfig) contract.Provider {
	ctx := core.NewEVMBlockContext(&types.Header{Number: big.NewInt(0), Difficulty: big.NewInt(0)}, nil, &common.Address{})
	evm := vm.NewEVM(ctx, vm.TxContext{}, state, config, vm.Config{NoBaseFee: true})
	return &stateProvider{vm: evm}
}

// Call implements the contract.Provider interface to make contract calls directly to the state
func (s *stateProvider) Call(addr ethgo.Address, input []byte, opts *contract.CallOpts) ([]byte, error) {
	retVal, _, err := s.vm.Call(vm.AccountRef(common.Address{}), common.Address(addr), input, 10000000, big.NewInt(0))
	if err != nil {
		return nil, err
	}
	return retVal, nil
}

// Txn is part of the contract.Provider interface to make Ethereum transactions. We disable this function
// since the system state does not make any transaction
func (s *stateProvider) Txn(ethgo.Address, ethgo.Key, []byte, *contract.TxnOpts) (contract.Txn, error) {
	panic("we do not make transaction in system state")
}

// --- SystemState implementation ---

var stateFunctions, _ = abi.NewABIFromList([]string{
	"function getValidators() returns (tuple(address,bytes)[])",
	"function getEpoch() returns (uint64)",
})

var sidechainBridgeFunctions, _ = abi.NewABIFromList([]string{
	"function getNextExecutionIndex() returns (uint64)",
	"function getNextCommittedIndex() returns (uint64)",
})

// SystemState is an interface to interact with the consensus system contracts in the chain
type SystemState interface {
	// GetValidatorSet retrieves current validator set from the smart contract
	GetValidatorSet() (AccountSet, error)
	// GetEpoch retrieves current epoch number from the smart contract
	GetEpoch() (uint64, error)
	// GetNextExecutionIndex retrieves next bridge state sync index
	GetNextExecutionIndex() (uint64, error)
	// GetNextCommittedIndex retrieves next committed bridge state sync index
	GetNextCommittedIndex() (uint64, error)
}

var _ SystemState = &SystemStateImpl{}

// SystemStateImpl is implementation of SystemState interface
type SystemStateImpl struct {
	validatorContract       *contract.Contract
	sidechainBridgeContract *contract.Contract
}

// NewSystemState initializes new instance of systemState which abstracts smart contracts functions
func NewSystemState(config *params.PolyBFTConfig, provider contract.Provider) *SystemStateImpl {
	s := &SystemStateImpl{}
	s.validatorContract = contract.NewContract(
		ethgo.Address(config.ValidatorSetAddr),
		stateFunctions, contract.WithProvider(provider),
	)
	s.sidechainBridgeContract = contract.NewContract(
		ethgo.Address(config.SidechainBridgeAddr),
		sidechainBridgeFunctions,
		contract.WithProvider(provider),
	)
	return s
}

// GetValidatorSet retrieves current validator set from the smart contract
func (s *SystemStateImpl) GetValidatorSet() (AccountSet, error) {
	ret, err := s.validatorContract.Call("getValidators", ethgo.Latest)
	if err != nil {
		return nil, err
	}

	res := []*ValidatorAccount{}
	for _, i := range ret["0"].([]map[string]interface{}) {
		keyParts, err := abi.Decode(abi.MustNewType("uint[4]"), i["1"].([]byte))
		if err != nil {
			return nil, err
		}
		bigKey := keyParts.([4]*big.Int)
		blsKey, err := bls.UnmarshalPublicKeyFromBigInt(bigKey)
		if err != nil {
			return nil, err
		}

		res = append(res, &ValidatorAccount{
			Address: common.Address(i["0"].(ethgo.Address)),
			BlsKey:  blsKey,
		})
	}
	return AccountSet(res), nil
}

// GetEpoch retrieves current epoch number from the smart contract
func (s *SystemStateImpl) GetEpoch() (uint64, error) {
	rawResult, err := s.validatorContract.Call("getEpoch", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	return rawResult["0"].(uint64), nil
}

// GetNextExecutionIndex retrieves next bridge state sync index
func (s *SystemStateImpl) GetNextExecutionIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("getNextExecutionIndex", ethgo.Latest)
	if err != nil {
		return 0, err
	}
	return rawResult["0"].(uint64), nil
}

// GetNextCommittedIndex retrieves next committed bridge state sync index
func (s *SystemStateImpl) GetNextCommittedIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("getNextCommittedIndex", ethgo.Latest)
	if err != nil {
		return 0, err
	}
	return rawResult["0"].(uint64), nil
}

func buildLogsFromReceipts(entry []*types.Receipt, header *types.Header) []*types.Log {
	var logs []*types.Log
	for _, taskReceipt := range entry {
		for _, taskLog := range taskReceipt.Logs {
			log := new(types.Log)
			*log = *taskLog

			log.BlockHash = header.Hash()
			log.BlockNumber = header.Number.Uint64()
			logs = append(logs, log)
		}
	}
	return logs
}
*/
