package state

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/allowlist"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/state/runtime/precompiled"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	SpuriousDragonMaxCodeSize = 24576
	TxPoolMaxInitCodeSize     = 2 * SpuriousDragonMaxCodeSize

	TxGas                 uint64 = 21000 // Per transaction not creating a contract
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract
)

var emptyCodeHashTwo = types.BytesToHash(crypto.Keccak256(nil))

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) types.Hash

type GetHashByNumberHelper = func(*types.Header) GetHashByNumber

// Executor is the main entity
type Executor struct {
	logger  hclog.Logger
	config  *chain.Params
	state   State
	GetHash GetHashByNumberHelper

	PostHook        func(txn *Transition)
	GenesisPostHook func(*Transition) error
}

// NewExecutor creates a new executor
func NewExecutor(config *chain.Params, s State, logger hclog.Logger) *Executor {
	return &Executor{
		logger: logger,
		config: config,
		state:  s,
	}
}

func (e *Executor) WriteGenesis(
	alloc map[types.Address]*chain.GenesisAccount,
	initialStateRoot types.Hash) (types.Hash, error) {
	var (
		snap Snapshot
		err  error
	)

	if initialStateRoot == types.ZeroHash {
		snap = e.state.NewSnapshot()
	} else {
		snap, err = e.state.NewSnapshotAt(initialStateRoot)
	}

	if err != nil {
		return types.Hash{}, err
	}

	txn := NewTxn(snap)
	config := e.config.Forks.At(0)

	env := runtime.TxContext{
		ChainID: e.config.ChainID,
	}

	transition := &Transition{
		ctx:         env,
		state:       txn,
		gasPool:     uint64(env.GasLimit),
		config:      config,
		precompiles: precompiled.NewPrecompiled(),
	}

	for addr, account := range alloc {
		if account.Balance != nil {
			txn.AddBalance(addr, account.Balance)
		}

		if account.Nonce != 0 {
			txn.SetNonce(addr, account.Nonce)
		}

		if len(account.Code) != 0 {
			txn.SetCode(addr, account.Code)
		}

		for key, value := range account.Storage {
			txn.SetState(addr, key, value)
		}
	}

	if e.GenesisPostHook != nil {
		if err := e.GenesisPostHook(transition); err != nil {
			return types.Hash{}, fmt.Errorf("Error writing genesis block: %w", err)
		}
	}

	objs := txn.Commit(false)
	_, root := snap.Commit(objs)

	return types.BytesToHash(root), nil
}

type BlockResult struct {
	Root     types.Hash
	Receipts []*types.Receipt
	TotalGas uint64
}

// ProcessBlock already does all the handling of the whole process
func (e *Executor) ProcessBlock(
	parentRoot types.Hash,
	block *types.Block,
	blockCreator types.Address,
) (*Transition1, error) {
	txn, err := e.BeginTxn(parentRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	for _, t := range block.Transactions {
		if err := txn.Write(t); err != nil {
			return nil, err
		}
	}

	return txn, nil
}

// StateAt returns snapshot at given root
func (e *Executor) State() State {
	return e.state
}

// StateAt returns snapshot at given root
func (e *Executor) StateAt(root types.Hash) (Snapshot, error) {
	return e.state.NewSnapshotAt(root)
}

// GetForksInTime returns the active forks at the given block height
func (e *Executor) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return e.config.Forks.At(blockNumber)
}

func (e *Executor) BeginTxn(
	parentRoot types.Hash,
	header *types.Header,
	coinbaseReceiver types.Address,
) (*Transition1, error) {
	forkConfig := e.config.Forks.At(header.Number)

	auxSnap2, err := e.state.NewSnapshotAt(parentRoot)
	if err != nil {
		return nil, err
	}

	newTxn := NewTxn(auxSnap2)

	txCtx := runtime.TxContext{
		Coinbase:   coinbaseReceiver,
		Timestamp:  int64(header.Timestamp),
		Number:     int64(header.Number),
		Difficulty: types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		GasLimit:   int64(header.GasLimit),
		ChainID:    e.config.ChainID,
	}

	txn := &Transition{
		ctx:         txCtx,
		state:       newTxn,
		snap:        auxSnap2,
		getHash:     e.GetHash(header),
		config:      forkConfig,
		gasPool:     uint64(txCtx.GasLimit),
		totalGas:    0,
		evm:         evm.NewEVM(),
		precompiles: precompiled.NewPrecompiled(),
		PostHook:    e.PostHook,
	}

	// enable contract deployment allow list (if any)
	if e.config.ContractDeployerAllowList != nil {
		txn.deploymentAllowlist = allowlist.NewAllowList(txn, contracts.AllowListContractsAddr)
	}

	// enable transactions allow list (if any)
	if e.config.TransactionsAllowList != nil {
		txn.txnAllowList = allowlist.NewAllowList(txn, contracts.AllowListTransactionsAddr)
	}

	return &Transition1{state: auxSnap2, tt: txn}, nil
}

type Transition1 struct {
	state    Snapshot
	tt       *Transition
	receipts []*types.Receipt
}

func (t *Transition1) Receipts() []*types.Receipt {
	return t.receipts
}

func (t *Transition1) TotalGas() uint64 {
	return t.tt.TotalGas()
}

func (t *Transition1) Write(txn *types.Transaction) error {
	receipt, err := t.tt.Write(txn)
	if err != nil {
		return err
	}
	t.receipts = append(t.receipts, receipt)
	return nil
}

// Commit commits the final result
func (t *Transition1) Commit() (Snapshot, types.Hash) {
	s2, root := t.state.Commit(t.tt.Commit())

	return s2, types.BytesToHash(root)
}

func (t *Transition1) Transition() *Transition {
	return t.tt
}
