package hook

import (
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

// register multiple

type ShouldWriteTransactionsFunc func(uint64) bool

type ModifyHeaderFunc func(*types.Header, types.Address) error

type VerifyHeaderFunc func(*types.Header) error

type VerifyBlockFunc func(*types.Block) error

type ProcessHeaderFunc func(*types.Header) error

type PreCommitStateFunc func(*types.Header, *state.Transition) error

type Hooks interface {
	ShouldWriteTransactions(uint64) bool
	ModifyHeader(*types.Header, types.Address) error
	VerifyHeader(*types.Header) error
	VerifyBlock(*types.Block) error
	ProcessHeader(*types.Header) error
	PreCommitState(*types.Header, *state.Transition) error
}

type HookManager struct {
	ShouldWriteTransactionFunc ShouldWriteTransactionsFunc
	ModifyHeaderFunc           ModifyHeaderFunc
	VerifyHeaderFunc           VerifyHeaderFunc
	VerifyBlockFunc            VerifyBlockFunc
	ProcessHeaderFunc          ProcessHeaderFunc
	PreCommitStateFunc         PreCommitStateFunc
}

func (m *HookManager) Clear() {
	m.ShouldWriteTransactionFunc = nil
	m.ModifyHeaderFunc = nil
	m.VerifyHeaderFunc = nil
	m.VerifyBlockFunc = nil
	m.ProcessHeaderFunc = nil
	m.PreCommitStateFunc = nil
}

func (m *HookManager) ShouldWriteTransactions(height uint64) bool {
	if m.ShouldWriteTransactionFunc != nil {
		return m.ShouldWriteTransactionFunc(height)
	}

	return true
}

func (m *HookManager) ModifyHeader(header *types.Header, proposer types.Address) error {
	if m.ModifyHeaderFunc != nil {
		return m.ModifyHeaderFunc(header, proposer)
	}

	return nil
}

func (m *HookManager) VerifyHeader(header *types.Header) error {
	if m.VerifyHeaderFunc != nil {
		return m.VerifyHeaderFunc(header)
	}

	return nil
}

func (m *HookManager) VerifyBlock(block *types.Block) error {
	if m.VerifyBlockFunc != nil {
		return m.VerifyBlockFunc(block)
	}

	return nil
}

func (m *HookManager) ProcessHeader(header *types.Header) error {
	if m.ProcessHeaderFunc != nil {
		return m.ProcessHeaderFunc(header)
	}

	return nil
}

func (m *HookManager) PreCommitState(header *types.Header, txn *state.Transition) error {
	if m.PreCommitStateFunc != nil {
		return m.PreCommitStateFunc(header, txn)
	}

	return nil
}
