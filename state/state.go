package state

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/evm"
	"github.com/0xPolygon/polygon-edge/evm/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

type State = evm.State

type Snapshot = evm.Snapshot

type Account = evm.Account

type GetHashByNumber = evm.GetHashByNumber

// Executor is the main entity
type Executor struct {
	logger  hclog.Logger
	config  *chain.Params
	state   evm.State
	GetHash evm.GetHashByNumberHelper

	PostHook PostHook
}

type PostHook func(txn *Transition)

// NewExecutor creates a new executor
func NewExecutor(config *chain.Params, s evm.State, logger hclog.Logger) *Executor {
	return &Executor{
		logger: logger,
		config: config,
		state:  s,
	}
}

func (e *Executor) SetRuntime(r runtime.Runtime) {

}

func (e *Executor) WriteGenesis(alloc map[types.Address]*chain.GenesisAccount) types.Hash {
	snap := e.state.NewSnapshot()
	txn := evm.NewTxn(e.state, snap)

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

	objs := txn.Commit(false)
	_, root := snap.Commit(objs)

	return types.BytesToHash(root)
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
) (*Transition, error) {
	txn, err := e.BeginTxn(parentRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	for _, t := range block.Transactions {
		var receipt *types.Receipt

		if t.ExceedsBlockGasLimit(block.Header.GasLimit) {
			receipt, err = txn.WriteFailedReceipt(t)
			if err != nil {
				return nil, err
			}
		} else {
			receipt, err = txn.Write(t)
			if err != nil {
				return nil, err
			}
		}

		txn.receipts = append(txn.receipts, receipt)
	}

	return txn, nil
}

// GetForksInTime returns the active forks at the given block height
func (e *Executor) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return e.config.Forks.At(blockNumber)
}

func (e *Executor) BeginTxn(
	parentRoot types.Hash,
	header *types.Header,
	coinbaseReceiver types.Address,
) (*Transition, error) {
	config := e.config.Forks.At(header.Number)

	auxSnap2, err := e.state.NewSnapshotAt(parentRoot)
	if err != nil {
		return nil, err
	}

	newTxn := evm.NewTxn(e.state, auxSnap2)

	env2 := runtime.TxContext{
		Coinbase:   coinbaseReceiver,
		Timestamp:  int64(header.Timestamp),
		Number:     int64(header.Number),
		Difficulty: types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		GasLimit:   int64(header.GasLimit),
		ChainID:    int64(e.config.ChainID),
	}

	txn := evm.NewTransition1()
	txn.Ctx = env2
	txn.Config = config
	txn.State = newTxn
	txn.GetHash = e.GetHash(header)
	txn.GasPool = uint64(env2.GasLimit)

	return &Transition{receipts: []*types.Receipt{}, writeSnapshot: auxSnap2, hook: e.PostHook, Transition1: txn}, nil
}

type Transition struct {
	*evm.Transition1

	writeSnapshot evm.Snapshot

	hook PostHook

	receipts []*types.Receipt
}

// Apply applies a new transaction
func (t *Transition) Apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	result, err := t.Transition1.Apply(msg)

	if t.hook != nil {
		t.hook(t)
	}

	return result, err
}

func (t *Transition) Commit() (evm.Snapshot, types.Hash) {
	s2, root := t.writeSnapshot.Commit(t.Transition1.Commit2())
	return s2, types.BytesToHash(root)
}

func (t *Transition) Receipts() []*types.Receipt {
	return t.receipts
}
