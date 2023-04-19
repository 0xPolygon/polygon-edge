package state

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	statetransition "github.com/0xPolygon/polygon-edge/state_transition"
	"github.com/0xPolygon/polygon-edge/state_transition/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

var emptyCodeHashTwo = types.BytesToHash(crypto.Keccak256(nil))

// Executor is the main entity
type Executor struct {
	logger  hclog.Logger
	config  *chain.Params
	state   State
	GetHash statetransition.GetHashByNumberHelper

	PostHook        func(txn *statetransition.Transition)
	GenesisPostHook func(*statetransition.Transition) error
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

	config := e.config.Forks.At(0)

	env := runtime.TxContext{
		ChainID: e.config.ChainID,
	}

	transition := statetransition.NewTransition(config, snap, env, nil)
	txn := transition.Txn()

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

	objs := transition.Commit()
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

	txCtx := runtime.TxContext{
		Coinbase:   coinbaseReceiver,
		Timestamp:  int64(header.Timestamp),
		Number:     int64(header.Number),
		Difficulty: types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		GasLimit:   int64(header.GasLimit),
		ChainID:    e.config.ChainID,
	}

	txn := statetransition.NewTransition(forkConfig, auxSnap2, txCtx, e.GetHash(header))

	if e.PostHook != nil {
		txn.WithPostHook(e.PostHook)
	}

	// enable contract deployment allow list (if any)
	if e.config.ContractDeployerAllowList != nil {
		txn.WithDeploymentAllowList(contracts.AllowListContractsAddr)
	}

	// enable transactions allow list (if any)
	if e.config.TransactionsAllowList != nil {
		txn.WithTxnAllowList(contracts.AllowListTransactionsAddr)
	}

	return &Transition1{state: auxSnap2, tt: txn}, nil
}

type Transition1 struct {
	state    Snapshot
	tt       *statetransition.Transition
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

func (t *Transition1) Transition() *statetransition.Transition {
	return t.tt
}
