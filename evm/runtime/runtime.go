package runtime

import (
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
)

// TxContext is the context of the transaction
type TxContext struct {
	GasPrice   types.Hash
	Origin     types.Address
	Coinbase   types.Address
	Number     int64
	Timestamp  int64
	GasLimit   int64
	ChainID    int64
	Difficulty types.Hash
}

// StorageStatus is the status of the storage access
type StorageStatus int

const (
	// StorageUnchanged if the data has not changed
	StorageUnchanged StorageStatus = iota
	// StorageModified if the value has been modified
	StorageModified
	// StorageModifiedAgain if the value has been modified before in the txn
	StorageModifiedAgain
	// StorageAdded if this is a new entry in the storage
	StorageAdded
	// StorageDeleted if the storage was deleted
	StorageDeleted
)

func (s StorageStatus) String() string {
	switch s {
	case StorageUnchanged:
		return "StorageUnchanged"
	case StorageModified:
		return "StorageModified"
	case StorageModifiedAgain:
		return "StorageModifiedAgain"
	case StorageAdded:
		return "StorageAdded"
	case StorageDeleted:
		return "StorageDeleted"
	default:
		panic("BUG: storage status not found")
	}
}

// Host is the execution host
type Host interface {
	AccountExists(addr types.Address) bool
	GetStorage(addr types.Address, key types.Hash) types.Hash
	SetStorage(addr types.Address, key types.Hash, value types.Hash, config *chain.ForksInTime) StorageStatus
	GetBalance(addr types.Address) *big.Int
	GetCodeSize(addr types.Address) int
	GetCodeHash(addr types.Address) types.Hash
	GetCode(addr types.Address) []byte
	Selfdestruct(addr types.Address, beneficiary types.Address)
	GetTxContext() TxContext
	GetBlockHash(number int64) types.Hash
	EmitLog(addr types.Address, topics []types.Hash, data []byte)
	Callx(*Contract, Host) *ExecutionResult
	Empty(addr types.Address) bool
	GetNonce(addr types.Address) uint64
}

// ExecutionResult includes all output after executing given evm
// message no matter the execution itself is successful or not.
type ExecutionResult struct {
	ReturnValue []byte // Returned data from the runtime (function result or data supplied with revert opcode)
	GasLeft     uint64 // Total gas left as result of execution
	GasUsed     uint64 // Total gas used as result of execution
	Err         error  // Any error encountered during the execution, listed below
}

func (r *ExecutionResult) Succeeded() bool { return r.Err == nil }
func (r *ExecutionResult) Failed() bool    { return r.Err != nil }
func (r *ExecutionResult) Reverted() bool  { return errors.Is(r.Err, ErrExecutionReverted) }

func (r *ExecutionResult) UpdateGasUsed(gasLimit uint64, refund uint64) {
	r.GasUsed = gasLimit - r.GasLeft

	// Refund can go up to half the gas used
	if maxRefund := r.GasUsed / 2; refund > maxRefund {
		refund = maxRefund
	}

	r.GasLeft += refund
	r.GasUsed -= refund
}

var (
	ErrOutOfGas                 = errors.New("out of gas")
	ErrStackOverflow            = errors.New("stack overflow")
	ErrStackUnderflow           = errors.New("stack underflow")
	ErrNotEnoughFunds           = errors.New("not enough funds")
	ErrInsufficientBalance      = errors.New("insufficient balance for transfer")
	ErrMaxCodeSizeExceeded      = errors.New("evm: max code size exceeded")
	ErrContractAddressCollision = errors.New("contract address collision")
	ErrDepth                    = errors.New("max call depth exceeded")
	ErrExecutionReverted        = errors.New("execution was reverted")
	ErrCodeStoreOutOfGas        = errors.New("contract creation code storage out of gas")
)

type CallType int

const (
	Call CallType = iota
	CallCode
	DelegateCall
	StaticCall
	Create
	Create2
)

// Runtime can process contracts
type Runtime interface {
	Run(c *Contract, host Host, config *chain.ForksInTime) *ExecutionResult
	CanRun(c *Contract, host Host, config *chain.ForksInTime) bool
	Name() string
}

// Contract is the instance being called
type Contract struct {
	Code        []byte
	Type        CallType
	CodeAddress types.Address
	Address     types.Address
	Origin      types.Address
	Caller      types.Address
	Depth       int
	Value       *big.Int
	Input       []byte
	Gas         uint64
	Static      bool
}

func NewContract(
	depth int,
	origin types.Address,
	from types.Address,
	to types.Address,
	value *big.Int,
	gas uint64,
	code []byte,
) *Contract {
	f := &Contract{
		Caller:      from,
		Origin:      origin,
		CodeAddress: to,
		Address:     to,
		Gas:         gas,
		Value:       value,
		Code:        code,
		Depth:       depth,
	}

	return f
}

func NewContractCreation(
	depth int,
	origin types.Address,
	from types.Address,
	to types.Address,
	value *big.Int,
	gas uint64,
	code []byte,
) *Contract {
	c := NewContract(depth, origin, from, to, value, gas, code)

	return c
}

func NewContractCall(
	depth int,
	origin types.Address,
	from types.Address,
	to types.Address,
	value *big.Int,
	gas uint64,
	code []byte,
	input []byte,
) *Contract {
	c := NewContract(depth, origin, from, to, value, gas, code)
	c.Input = input

	return c
}
