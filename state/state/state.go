package state

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// State is the state interface for the ethereum protocol
type State interface {

	// Balance
	AddBalance(addr common.Address, amount *big.Int)
	SubBalance(addr common.Address, amount *big.Int)
	SetBalance(addr common.Address, amount *big.Int)
	GetBalance(addr common.Address) *big.Int

	// Snapshot
	Snapshot() int
	RevertToSnapshot(int)

	// Logs
	AddLog(log *types.Log)

	// State
	SetState(addr common.Address, key, value common.Hash)
	GetState(addr common.Address, hash common.Hash) common.Hash

	// Nonce
	SetNonce(addr common.Address, nonce uint64)
	GetNonce(addr common.Address) uint64

	// Code
	SetCode(addr common.Address, code []byte)
	GetCode(addr common.Address) []byte
	GetCodeSize(addr common.Address) int
	GetCodeHash(addr common.Address) common.Hash

	// Suicide
	HasSuicided(addr common.Address) bool
	Suicide(addr common.Address) bool

	// Refund
	AddRefund(gas uint64)
	SubRefund(gas uint64)
	GetCommittedState(addr common.Address, hash common.Hash) common.Hash

	// Others
	Exist(addr common.Address) bool
	Empty(addr common.Address) bool
	CreateAccount(addr common.Address)
}
