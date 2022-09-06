package hook

import (
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

type ShouldWriteTransactionsFunc func(uint64) bool

type ModifyHeaderFunc func(*types.Header, types.Address) error

type VerifyHeaderFunc func(*types.Header) error

type VerifyBlockFunc func(*types.Block) error

type ProcessHeaderFunc func(*types.Header) error

type PreCommitStateFunc func(*types.Header, *state.Transition) error

type PostInsertBlockFunc func(*types.Block) error

type Hooks struct {
	ShouldWriteTransactionFunc ShouldWriteTransactionsFunc
	ModifyHeaderFunc           ModifyHeaderFunc
	VerifyHeaderFunc           VerifyHeaderFunc
	VerifyBlockFunc            VerifyBlockFunc
	ProcessHeaderFunc          ProcessHeaderFunc
	PreCommitStateFunc         PreCommitStateFunc
	PostInsertBlockFunc        PostInsertBlockFunc
}

func (m *Hooks) ShouldWriteTransactions(height uint64) bool {
	if m.ShouldWriteTransactionFunc != nil {
		return m.ShouldWriteTransactionFunc(height)
	}

	return true
}

func (m *Hooks) ModifyHeader(header *types.Header, proposer types.Address) error {
	if m.ModifyHeaderFunc != nil {
		return m.ModifyHeaderFunc(header, proposer)
	}

	return nil
}

func (m *Hooks) VerifyHeader(header *types.Header) error {
	if m.VerifyHeaderFunc != nil {
		return m.VerifyHeaderFunc(header)
	}

	return nil
}

func (m *Hooks) VerifyBlock(block *types.Block) error {
	if m.VerifyBlockFunc != nil {
		return m.VerifyBlockFunc(block)
	}

	return nil
}

func (m *Hooks) ProcessHeader(header *types.Header) error {
	if m.ProcessHeaderFunc != nil {
		return m.ProcessHeaderFunc(header)
	}

	return nil
}

func (m *Hooks) PreCommitState(header *types.Header, txn *state.Transition) error {
	if m.PreCommitStateFunc != nil {
		return m.PreCommitStateFunc(header, txn)
	}

	return nil
}

func (m *Hooks) PostInsertBlock(block *types.Block) error {
	if m.PostInsertBlockFunc != nil {
		return m.PostInsertBlockFunc(block)
	}

	return nil
}
