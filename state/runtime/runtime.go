package runtime

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/umbracle/minimal/types"
)

var (
	ErrGasConsumed              = fmt.Errorf("gas has been consumed")
	ErrGasOverflow              = fmt.Errorf("gas overflow")
	ErrStackOverflow            = fmt.Errorf("stack overflow")
	ErrStackUnderflow           = fmt.Errorf("stack underflow")
	ErrJumpDestNotValid         = fmt.Errorf("jump destination is not valid")
	ErrMemoryOverflow           = fmt.Errorf("error memory overflow")
	ErrNotEnoughFunds           = fmt.Errorf("not enough funds")
	ErrMaxCodeSizeExceeded      = errors.New("evm: max code size exceeded")
	ErrContractAddressCollision = errors.New("contract address collision")
	ErrDepth                    = errors.New("max call depth exceeded")
	ErrOpcodeNotFound           = errors.New("opcode not found")
	ErrExecutionReverted        = errors.New("execution was reverted")
	ErrCodeStoreOutOfGas        = fmt.Errorf("code storage out of gas")
)

type CallType int

const (
	Call CallType = iota
	CallCode
	DelegateCall
	StaticCall
)

// Runtime can process contracts
type Runtime interface {
	Run(c *Contract) ([]byte, uint64, error)
}

type Executor interface {
	Call(c *Contract, t CallType) ([]byte, uint64, error)
	Create(c *Contract) ([]byte, uint64, error)
}

// Contract is the instance being called
type Contract struct {
	Code []byte

	CodeAddress types.Address
	Address     types.Address // address of the contract
	Origin      types.Address // origin is where the storage is taken from
	Caller      types.Address // caller is the one calling the contract
	depth       int

	// inputs
	Value *big.Int // value of the tx
	Input []byte   // Input of the tx
	Gas   uint64

	RetOffset uint64
	RetSize   uint64

	Static bool
}

func (c *Contract) Depth() int {
	return c.depth
}

func (c *Contract) ConsumeGas(gas uint64) bool {
	if c.Gas < gas {
		return false
	}

	c.Gas -= gas
	return true
}

func (c *Contract) ConsumeAllGas() {
	c.Gas = 0
}

func NewContract(depth int, origin types.Address, from types.Address, to types.Address, value *big.Int, gas uint64, code []byte) *Contract {
	f := &Contract{
		Caller:      from,
		Origin:      origin,
		CodeAddress: to,
		Address:     to,
		Gas:         gas,
		Value:       value,
		Code:        code,
		depth:       depth,
	}
	return f
}

func NewContractCreation(depth int, origin types.Address, from types.Address, to types.Address, value *big.Int, gas uint64, code []byte) *Contract {
	c := NewContract(depth, origin, from, to, value, gas, code)
	return c
}

func NewContractCall(depth int, origin types.Address, from types.Address, to types.Address, value *big.Int, gas uint64, code []byte, input []byte) *Contract {
	c := NewContract(depth, origin, from, to, value, gas, code)
	c.Input = input
	return c
}

// Env refers to the block information the transactions runs in
// it is shared for all the contracts executed so its in the EVM.
type Env struct {
	Coinbase   types.Address
	Timestamp  uint64
	Number     uint64
	Difficulty *big.Int
	GasLimit   *big.Int
	GasPrice   *big.Int
}

// State is the state interface for the ethereum protocol
type State interface {

	// Balance
	AddBalance(addr types.Address, amount *big.Int)
	SubBalance(addr types.Address, amount *big.Int)
	SetBalance(addr types.Address, amount *big.Int)
	GetBalance(addr types.Address) *big.Int

	// Snapshot
	Snapshot() int
	RevertToSnapshot(int)

	// Logs
	AddLog(log *types.Log)
	Logs() []*types.Log

	// State
	SetState(addr types.Address, key, value types.Hash)
	GetState(addr types.Address, hash types.Hash) types.Hash

	// Nonce
	SetNonce(addr types.Address, nonce uint64)
	GetNonce(addr types.Address) uint64

	// Code
	SetCode(addr types.Address, code []byte)
	GetCode(addr types.Address) []byte
	GetCodeSize(addr types.Address) int
	GetCodeHash(addr types.Address) types.Hash

	// Suicide
	HasSuicided(addr types.Address) bool
	Suicide(addr types.Address) bool

	// Refund
	AddRefund(gas uint64)
	SubRefund(gas uint64)
	GetRefund() uint64
	GetCommittedState(addr types.Address, hash types.Hash) types.Hash

	// Others
	Exist(addr types.Address) bool
	Empty(addr types.Address) bool
	CreateAccount(addr types.Address)
}
