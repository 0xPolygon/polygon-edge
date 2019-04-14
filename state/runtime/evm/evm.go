package evm

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"

	// "github.com/umbracle/minimal/state/runtime/evm/precompiled"

	"github.com/ethereum/go-ethereum/common"
)

// IMPORTANT. Memory access needs more overflow protection, right now, only calls and returns are protected

var (
	ErrGasConsumed              = fmt.Errorf("gas has been consumed")
	ErrGasOverflow              = fmt.Errorf("gas overflow")
	ErrStackOverflow            = fmt.Errorf("stack overflow")
	ErrStackUnderflow           = fmt.Errorf("stack underflow")
	ErrJumpDestNotValid         = fmt.Errorf("jump destination is not valid")
	ErrMemoryOverflow           = fmt.Errorf("error memory overflow")
	ErrMaxCodeSizeExceeded      = errors.New("evm: max code size exceeded")
	ErrContractAddressCollision = errors.New("contract address collision")
	ErrDepth                    = errors.New("max call depth exceeded")
	ErrOpcodeNotFound           = errors.New("opcode not found")
)

var (
	errReadOnly = fmt.Errorf("it is a static call and the state cannot be changed")
)

var (
	tt255 = math.BigPow(2, 255)
)

const StackSize = 2048
const MaxContracts = 1030

// Instructions is the code of instructions
type Instructions []byte

type CanTransferFunc func(runtime.State, common.Address, *big.Int) bool

type TransferFunc func(state runtime.State, from common.Address, to common.Address, amount *big.Int) error

// Contract is each value from the caller stack
type Contract struct {
	ip   int
	code Instructions

	// memory
	memory *Memory
	stack  []*big.Int
	sp     int

	codeAddress common.Address
	address     common.Address // address of the contract
	origin      common.Address // origin is where the storage is taken from
	caller      common.Address // caller is the one calling the contract

	// remove later
	evm *EVM

	depth int

	// inputs
	value *big.Int // value of the tx
	input []byte   // Input of the tx
	gas   uint64

	// type of contract
	static bool

	retOffset uint64
	retSize   uint64

	bitvec bitvec

	snapshot int

	returnData []byte
}

func (c *Contract) MemoryLen() *big.Int {
	return big.NewInt(int64(len(c.memory.store)))
}

func (c *Contract) validJumpdest(dest *big.Int) bool {
	udest := dest.Uint64()

	if dest.BitLen() >= 63 || udest >= uint64(len(c.code)) {
		return false
	}
	if OpCode(c.code[udest]) != JUMPDEST {
		return false
	}
	return c.bitvec.codeSegment(udest)
}

// Instructions is the code of the contract
func (c *Contract) Instructions() Instructions {
	return c.code
}

func (c *Contract) push(val *big.Int) {
	c.stack[c.sp] = val
	c.sp++
}

func (c *Contract) stackAtLeast(n int) bool {
	return c.sp >= n
}

func (c *Contract) pop() *big.Int {
	if c.sp == 0 {
		return nil
	}
	o := c.stack[c.sp-1]
	c.sp--
	return o
}

func (c *Contract) peek() *big.Int {
	return c.stack[c.sp-1]
}

func (c *Contract) peekAt(n int) *big.Int {
	return c.stack[c.sp-n]
}

func (c *Contract) swap(n int) {
	c.stack[c.sp-1], c.stack[c.sp-n-1] = c.stack[c.sp-n-1], c.stack[c.sp-1]
}

func (c *Contract) consumeGas(gas uint64) bool {
	if c.gas < gas {
		return false
	}

	c.gas -= gas
	return true
}

func (c *Contract) consumeAllGas() {
	c.gas = 0
}

func (c *Contract) showStack() string {
	str := []string{}
	for i := 0; i < c.sp; i++ {
		str = append(str, c.stack[i].String())
	}
	return "Stack: " + strings.Join(str, ",")
}

func newContract(evm *EVM, depth int, origin common.Address, from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	f := &Contract{
		ip:          -1,
		depth:       depth,
		code:        code,
		evm:         evm,
		caller:      from,
		origin:      origin,
		codeAddress: to,
		address:     to,
		value:       value,
		stack:       make([]*big.Int, StackSize),
		sp:          0,
		gas:         gas,
		input:       []byte{},
		bitvec:      codeBitmap(code),
		snapshot:    -1,
	}
	f.memory = newMemory(f)
	return f
}

func newContractCreation(evm *EVM, depth int, origin common.Address, from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	c := newContract(evm, depth, origin, from, to, value, gas, code)
	return c
}

func newContractCall(evm *EVM, depth int, origin common.Address, from common.Address, to common.Address, value *big.Int, gas uint64, code []byte, input []byte) *Contract {
	c := newContract(evm, depth, origin, from, to, value, gas, code)
	c.input = input
	return c
}

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) common.Hash

// EVM is the ethereum virtual machine
type EVM struct {
	created []common.Address

	contracts      []*Contract
	contractsIndex int

	config   chain.ForksInTime
	gasTable chain.GasTable

	state runtime.State
	env   *runtime.Env

	getHash     GetHashByNumber
	CanTransfer CanTransferFunc
	Transfer    TransferFunc

	// returnData []byte

	executor runtime.Executor
	snapshot int
	// precompiled map[common.Address]*precompiled.Precompiled
}

func (e *EVM) Run(c *runtime.Contract) ([]byte, uint64, error) {
	contract := &Contract{
		ip:          -1,
		depth:       c.Depth(),
		code:        c.Code,
		evm:         e,
		caller:      c.Caller,
		origin:      c.Origin,
		codeAddress: c.CodeAddress,
		address:     c.Address,
		value:       c.Value,
		stack:       make([]*big.Int, StackSize),
		sp:          0,
		gas:         c.Gas,
		input:       c.Input,
		bitvec:      codeBitmap(c.Code),
		snapshot:    -1,
		static:      c.Static,
	}

	contract.memory = newMemory(contract)

	ret, err := contract.Run()
	return ret, contract.gas, err
}

// NewEVM creates a new EVM
func NewEVM(executor runtime.Executor, state runtime.State, env *runtime.Env, config chain.ForksInTime, gasTable chain.GasTable, getHash GetHashByNumber) *EVM {
	return &EVM{
		contracts:      make([]*Contract, MaxContracts),
		config:         config,
		gasTable:       gasTable,
		contractsIndex: 0,
		executor:       executor,
		state:          state,
		env:            env,
		getHash:        getHash,
	}
}

func (c *Contract) calculateFixedGasUsage(op OpCode) uint64 {
	if isPush(op) || isSwap(op) || isDup(op) {
		return GasFastestStep
	}

	switch op {
	case ADD, SUB, LT, GT, SLT, SGT, EQ, ISZERO, AND, XOR, OR, NOT, BYTE, CALLDATALOAD, SHL, SHR, SAR:
		return GasFastestStep

	case MUL, DIV, SDIV, MOD, SMOD, SIGNEXTEND:
		return GasFastStep

	case ADDMOD, MULMOD, JUMP:
		return GasMidStep

	case JUMPI:
		return GasSlowStep

	case BLOCKHASH:
		return GasExtStep

	case PC, GAS, MSIZE, POP, GASLIMIT, DIFFICULTY, NUMBER, TIMESTAMP, COINBASE, GASPRICE, CODESIZE, CALLDATASIZE, CALLVALUE, CALLER, ORIGIN, ADDRESS, RETURNDATASIZE:
		return GasQuickStep

	case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
		return GasFastestStep

	case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
		return GasFastestStep

	case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
		return GasFastestStep

	case BALANCE:
		return c.evm.gasTable.Balance

	case EXTCODEHASH:
		return c.evm.gasTable.ExtcodeHash

	case EXTCODESIZE:
		return c.evm.gasTable.ExtcodeSize

	case SLOAD:
		return c.evm.gasTable.SLoad

	case JUMPDEST:
		return 1

	case STOP:
		return 0

	default:
		return 0
	}
}

func isPush(op OpCode) bool {
	return op >= PUSH1 && op <= PUSH32
}

func isSwap(op OpCode) bool {
	return op >= SWAP1 && op <= SWAP16
}

func isDup(op OpCode) bool {
	return op >= DUP1 && op <= DUP16
}

// Run executes the virtual machine
func (c *Contract) Run() ([]byte, error) {
	var op OpCode

	var vmerr error
	var returnData []byte

	for c.ip < len(c.Instructions())-1 {
		c.ip++

		ip := c.ip
		ins := c.Instructions()
		op = OpCode(ins[ip])

		// fmt.Printf("OP [%d]: %s (%d)\n", c.depth, op.String(), c.gas)

		// consume gas for those opcodes with fixed gas
		if gasUsed := c.calculateFixedGasUsage(op); gasUsed != 0 {
			if !c.consumeGas(gasUsed) {
				vmerr = ErrGasConsumed
				goto END
			}
		}

		switch op {
		case ADD, MUL, SUB, DIV, SDIV, MOD, SMOD, EXP: // add the other operations
			val, err := c.executeUnsignedArithmeticOperations(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		case ADDMOD, MULMOD:
			val, err := c.executeModularOperations(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		case NOT, ISZERO:
			val, err := c.executeBitWiseOperations1(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		case AND, OR, XOR, BYTE:
			val, err := c.executeBitWiseOperations2(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		case EQ, GT, LT, SLT, SGT:
			val, err := c.executeComparison(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		case SHL, SHR, SAR:
			if !c.evm.config.Constantinople {
				vmerr = ErrOpcodeNotFound
			} else {
				val, err := c.executeShiftOperations(op)
				if err != nil {
					vmerr = err
				} else {
					c.push(val)
				}
			}

		case SIGNEXTEND:
			val, err := c.executeSignExtension()
			if err != nil {
				vmerr = err
			} else if val != nil {
				c.push(val)
			}

		// --- context ---

		case ADDRESS, BALANCE, ORIGIN, CALLER, CALLVALUE, CALLDATALOAD, CALLDATASIZE, CODESIZE, EXTCODESIZE, GASPRICE, RETURNDATASIZE:
			val, err := c.executeContextOperations(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		// --- context memory copy ---

		case EXTCODECOPY:
			vmerr = c.executeExtCodeCopy()

		case CODECOPY, CALLDATACOPY, RETURNDATACOPY:
			vmerr = c.executeContextCopyOperations(op)

		// --- block information ---

		case BLOCKHASH, COINBASE, TIMESTAMP, NUMBER, DIFFICULTY, GASLIMIT:
			val, err := c.executeBlockInformation(op)
			if err != nil {
				vmerr = err
			} else {
				c.push(val)
			}

		// Push operations

		case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
			n := int(op - PUSH) // i.e. PUSH6, n = 6

			var data []byte
			if ip+1+n > len(ins) {
				data = common.RightPadBytes(ins[ip+1:len(ins)], n)
			} else {
				data = ins[ip+1 : ip+1+n]
			}

			c.push(big.NewInt(0).SetBytes(data))
			c.ip += n

		// Duplicate operations

		case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
			n := int(op - DUP)
			if !c.stackAtLeast(n) {
				vmerr = ErrStackUnderflow
				goto END
			}
			c.push(c.peekAt(n))

		// Swap operations

		case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
			n := int(op - SWAP)
			if !c.stackAtLeast(n + 1) {
				vmerr = ErrStackUnderflow
				goto END
			}
			c.swap(n)

		// Logging operations

		case LOG0, LOG1, LOG2, LOG3, LOG4:
			vmerr = c.executeLogsOperation(op)

		// System operations

		case EXTCODEHASH:
			vmerr = c.executeExtCodeHashOperation()

		case CREATE, CREATE2:
			c.returnData, vmerr = c.executeCreateOperation(op)

		case CALL, CALLCODE, DELEGATECALL, STATICCALL:
			c.returnData, vmerr = c.executeCallOperation(op)

		case REVERT, RETURN:
			return c.executeHaltOperations(op)

		case SELFDESTRUCT:
			if c.inStaticCall() {
				vmerr = errReadOnly
				goto END
			}
			vmerr = c.selfDestruct()
			goto END

		case STOP:
			goto END

		// --- sha3 ---

		case SHA3:
			vmerr = c.sha3()

		// --- stack ---

		case POP:
			if n := c.pop(); n == nil {
				vmerr = ErrStackUnderflow
			}

		// --- memory ---

		case MLOAD:
			offset := c.pop()
			if offset == nil {
				vmerr = ErrStackUnderflow
				goto END
			}

			data, gas, err := c.memory.Get(offset, big.NewInt(32))
			if err != nil {
				vmerr = err
				goto END
			}
			c.push(big.NewInt(1).SetBytes(data))

			gas, overflow := math.SafeAdd(gas, GasFastestStep)
			if overflow {
				vmerr = ErrGasOverflow
				goto END
			}

			if !c.consumeGas(gas) {
				vmerr = ErrGasConsumed
				goto END
			}

		case MSTORE:
			// TODO, try to mix mstore8, mstore and mload
			if !c.stackAtLeast(2) {
				vmerr = ErrStackUnderflow
				goto END
			}
			start, val := c.pop(), c.pop()

			gas, err := c.memory.Set32(start, val)
			if err != nil {
				vmerr = err
				goto END
			}

			gas, overflow := math.SafeAdd(gas, GasFastestStep)
			if overflow {
				vmerr = ErrGasConsumed
				goto END
			}

			if !c.consumeGas(gas) {
				vmerr = ErrGasConsumed
				goto END
			}

		case MSTORE8:
			if !c.stackAtLeast(2) {
				vmerr = ErrStackUnderflow
				goto END
			}

			offset, val := c.pop(), c.pop().Int64()

			gas, err := c.memory.SetByte(offset, val)
			if err != nil {
				vmerr = err
				goto END
			}
			gas, overflow := math.SafeAdd(gas, GasFastestStep)
			if overflow {
				vmerr = ErrGasOverflow
				goto END
			}

			if !c.consumeGas(gas) {
				vmerr = ErrGasConsumed
				goto END
			}

		// --- storage ---

		case SLOAD:
			loc := c.pop()
			if loc == nil {
				vmerr = ErrStackUnderflow
				goto END
			}
			val := c.evm.state.GetState(c.address, common.BigToHash(loc))
			c.push(val.Big())

		case SSTORE:
			vmerr = c.executeSStoreOperation()

		// --- flow ---

		case JUMP:
			dest := c.pop()
			if dest == nil {
				vmerr = ErrStackUnderflow
				goto END
			}

			if !c.validJumpdest(dest) {
				vmerr = ErrJumpDestNotValid
				goto END
			}
			c.ip = int(dest.Uint64() - 1)

		case JUMPI:
			if !c.stackAtLeast(2) {
				vmerr = ErrStackUnderflow
				goto END
			}
			dest, cond := c.pop(), c.pop()

			if cond.Sign() != 0 {
				if !c.validJumpdest(dest) {
					vmerr = ErrJumpDestNotValid
					goto END
				}
				c.ip = int(dest.Uint64() - 1)
			}

		case JUMPDEST:
			// Nothing

		case PC:
			c.push(big.NewInt(int64(c.ip)))

		case MSIZE:
			c.push(c.MemoryLen())

		case GAS:
			c.push(big.NewInt(int64(c.gas)))

		default:
			if strings.Contains(op.String(), "Missing") {
				vmerr = ErrOpcodeNotFound
				goto END
			}
			return nil, fmt.Errorf("opcode not found: %s", op.String())
		}

		if c.sp > 1024 {
			vmerr = ErrStackOverflow
		}
		if vmerr != nil {
			break
		}
	}

END:
	return returnData, vmerr
}

func (c *Contract) executeSStoreOperation() error {
	if c.inStaticCall() {
		return errReadOnly
	}
	if !c.stackAtLeast(2) {
		return ErrStackUnderflow
	}

	address := c.address

	loc, val := c.pop(), c.pop()

	var gas uint64

	current := c.evm.state.GetState(address, common.BigToHash(loc))

	// discount gas (constantinople)
	if !c.evm.config.Constantinople {
		switch {
		case current == (common.Hash{}) && val.Sign() != 0: // 0 => non 0
			gas = SstoreSetGas
		case current != (common.Hash{}) && val.Sign() == 0: // non 0 => 0
			c.evm.state.AddRefund(SstoreRefundGas)
			gas = SstoreClearGas
		default: // non 0 => non 0 (or 0 => 0)
			gas = SstoreResetGas
		}
	} else {

		getGas := func() uint64 {
			// non constantinople gas
			value := common.BigToHash(val)
			if current == value { // noop (1)
				return NetSstoreNoopGas
			}
			original := c.evm.state.GetCommittedState(address, common.BigToHash(loc))
			if original == current {
				if original == (common.Hash{}) { // create slot (2.1.1)
					return NetSstoreInitGas
				}
				if value == (common.Hash{}) { // delete slot (2.1.2b)
					c.evm.state.AddRefund(NetSstoreClearRefund)
				}
				return NetSstoreCleanGas // write existing slot (2.1.2)
			}
			if original != (common.Hash{}) {
				if current == (common.Hash{}) { // recreate slot (2.2.1.1)
					c.evm.state.SubRefund(NetSstoreClearRefund)
				} else if value == (common.Hash{}) { // delete slot (2.2.1.2)
					c.evm.state.AddRefund(NetSstoreClearRefund)
				}
			}
			if original == value {
				if original == (common.Hash{}) { // reset to original inexistent slot (2.2.2.1)
					c.evm.state.AddRefund(NetSstoreResetClearRefund)
				} else { // reset to original existing slot (2.2.2.2)
					c.evm.state.AddRefund(NetSstoreResetRefund)
				}
			}
			return NetSstoreDirtyGas
		}
		gas = getGas()
	}

	if !c.consumeGas(gas) {
		return ErrGasOverflow
	}

	c.evm.state.SetState(address, common.BigToHash(loc), common.BigToHash(val))
	return nil
}

func (c *Contract) executeLogsOperation(op OpCode) error {
	if c.inStaticCall() {
		return errReadOnly
	}

	size := int(op - LOG)
	topics := make([]common.Hash, size)

	if !c.stackAtLeast(2 + size) {
		return ErrStackUnderflow
	}

	mStart, mSize := c.pop(), c.pop()
	for i := 0; i < size; i++ {
		topics[i] = common.BigToHash(c.pop())
	}

	data, gas, err := c.memory.Get(mStart, mSize)
	if err != nil {
		return err
	}
	c.evm.state.AddLog(&types.Log{
		Address:     c.address,
		Topics:      topics,
		Data:        data,
		BlockNumber: c.evm.env.Number.Uint64(),
	})

	requestedSize, overflow := bigUint64(mSize)
	if overflow {
		return ErrGasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, LogGas); overflow {
		return ErrGasOverflow
	}
	if gas, overflow = math.SafeAdd(gas, uint64(size)*LogTopicGas); overflow {
		return ErrGasOverflow
	}

	var memorySizeGas uint64
	if memorySizeGas, overflow = math.SafeMul(requestedSize, LogDataGas); overflow {
		return ErrGasOverflow
	}
	if gas, overflow = math.SafeAdd(gas, memorySizeGas); overflow {
		return ErrGasOverflow
	}

	if !c.consumeGas(gas) {
		return ErrGasConsumed
	}
	return nil
}

func (c *Contract) sha3() error {
	if !c.stackAtLeast(2) {
		return ErrStackUnderflow
	}

	offset, size := c.pop(), c.pop()

	data, gas, err := c.memory.Get(offset, size)
	if err != nil {

		return err
	}

	hash := crypto.Keccak256Hash(data)
	c.push(hash.Big())

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, Sha3Gas); overflow {
		return ErrGasOverflow
	}

	wordGas, overflow := bigUint64(size)
	if overflow {
		return ErrGasOverflow
	}

	if wordGas, overflow = math.SafeMul(numWords(wordGas), Sha3WordGas); overflow {
		return ErrGasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, wordGas); overflow {
		return ErrGasOverflow
	}

	if !c.consumeGas(gas) {
		return ErrGasConsumed
	}
	return nil
}

/*
func (c *Contract) create(contract *Contract) ([]byte, error) {
	// Check if its too deep
	if c.Depth() > int(CallCreateDepth) {
		return nil, ErrDepth
	}

	caller, address, value := contract.caller, contract.address, contract.value

	// Check if the values can be transfered
	if !c.evm.CanTransfer(c.evm.state, caller, value) {
		return nil, ErrNotEnoughFunds
	}

	// Increase the nonce of the caller
	nonce := c.evm.state.GetNonce(caller)
	c.evm.state.SetNonce(caller, nonce+1)

	// Check for address collisions
	contractHash := c.evm.state.GetCodeHash(address)
	if c.evm.state.GetNonce(address) != 0 || (contractHash != (common.Hash{}) && contractHash != emptyCodeHash) {
		contract.consumeAllGas()
		return nil, ErrContractAddressCollision
	}

	// Take snapshot of the current state
	contract.snapshot = c.evm.state.Snapshot()

	// Create the new account for the contract
	c.evm.state.CreateAccount(address)
	if c.evm.config.EIP158 {
		c.evm.state.SetNonce(address, 1)
	}

	// Transfer the value
	if value != nil {
		if err := c.evm.Transfer(c.evm.state, caller, address, value); err != nil {
			return nil, ErrNotEnoughFunds
		}
	}

	// run the code here
	ret, err := contract.Run()

	maxCodeSizeExceeded := c.evm.config.EIP158 && len(ret) > MaxCodeSize

	if err == nil && !maxCodeSizeExceeded {
		createDataGas := uint64(len(ret)) * params.CreateDataGas
		if contract.consumeGas(createDataGas) {
			c.evm.state.SetCode(address, ret)
		} else {
			err = vm.ErrCodeStoreOutOfGas
		}
	}

	if maxCodeSizeExceeded || (err != nil && (c.evm.config.Homestead || err != vm.ErrCodeStoreOutOfGas)) {
		c.evm.state.RevertToSnapshot(contract.snapshot)
		if err != ErrExecutionReverted {
			contract.consumeAllGas()
		}
	}

	// Assign err if contract code size exceeds the max while the err is still empty.
	if maxCodeSizeExceeded && err == nil {
		err = ErrMaxCodeSizeExceeded
	}

	return ret, err
}
*/

func (c *Contract) buildCreateContract(op OpCode) (*runtime.Contract, common.Address, error) {
	var expected int
	if op == CREATE {
		expected = 3
	} else if op == CREATE2 {
		expected = 4
	} else {
		panic(fmt.Errorf("Only CREATE or CREATE2 expected: Found %s", op.String()))
	}

	if !c.stackAtLeast(expected) {
		return nil, common.Address{}, ErrStackUnderflow
	}

	// Pop input arguments
	value := c.pop()
	offset, size := c.pop(), c.pop()

	var salt *big.Int
	if op == CREATE2 {
		salt = c.pop()
	}

	// Calculate and consume gas cost

	var overflow bool
	var gasCost uint64

	// Both CREATE and CREATE2 use memory
	input, gasCost, err := c.memory.Get(offset, size)
	if err != nil {
		return nil, common.Address{}, err
	}

	gasParam := CreateGas
	if op == CREATE2 {
		// Need to add the sha3 gas cost
		wordGas, overflow := bigUint64(size)
		if overflow {
			return nil, common.Address{}, ErrGasOverflow
		}
		if wordGas, overflow = math.SafeMul(numWords(wordGas), Sha3WordGas); overflow {
			return nil, common.Address{}, ErrGasOverflow
		}
		if gasCost, overflow = math.SafeAdd(gasCost, wordGas); overflow {
			return nil, common.Address{}, ErrGasOverflow
		}

		gasParam = Create2Gas
	}

	if gasCost, overflow = math.SafeAdd(gasCost, gasParam); overflow {
		return nil, common.Address{}, ErrGasOverflow
	}

	if !c.consumeGas(gasCost) {
		return nil, common.Address{}, ErrGasOverflow
	}

	// Calculate and consume gas for the call
	gas := c.gas

	// CREATE2 uses by default EIP150
	if c.evm.config.EIP150 || op == CREATE2 {
		gas -= gas / 64
	}

	if !c.consumeGas(gas) {
		return nil, common.Address{}, ErrGasOverflow
	}

	// Calculate address
	var address common.Address
	if op == CREATE {
		address = crypto.CreateAddress(c.address, c.evm.state.GetNonce(c.address))
	} else {
		address = crypto.CreateAddress2(c.address, common.BigToHash(salt), crypto.Keccak256Hash(input).Bytes())
	}

	contract := runtime.NewContractCreation(c.depth+1, c.origin, c.address, address, value, gas, input)
	return contract, address, nil
}

func (c *Contract) executeCreateOperation(op OpCode) ([]byte, error) {
	if c.inStaticCall() {
		return nil, errReadOnly
	}

	if op == CREATE2 {
		if !c.evm.config.Constantinople {
			return nil, ErrOpcodeNotFound
		}
	}

	contract, addr, err := c.buildCreateContract(op)
	if err != nil {
		return nil, err
	}

	ret, gas, err := c.evm.executor.Create(contract)

	if op == CREATE && c.evm.config.Homestead && err == vm.ErrCodeStoreOutOfGas {
		c.push(big.NewInt(0))
	} else if err != nil && err != vm.ErrCodeStoreOutOfGas {
		c.push(big.NewInt(0))
	} else {
		c.push(addr.Big())
	}

	c.gas += gas

	if err == runtime.ErrExecutionReverted {
		return ret, nil
	}
	return nil, nil
}

func (c *Contract) executeCallOperation(op OpCode) ([]byte, error) {
	if op == CALL && c.inStaticCall() {
		if val := c.peekAt(3); val != nil && val.BitLen() > 0 {
			return nil, errReadOnly
		}
	}

	if op == DELEGATECALL && !c.evm.config.Homestead {
		return nil, ErrOpcodeNotFound
	}
	if op == STATICCALL && !c.evm.config.Byzantium {
		return nil, ErrOpcodeNotFound
	}

	var callType runtime.CallType
	switch op {
	case CALL:
		callType = runtime.Call

	case CALLCODE:
		callType = runtime.CallCode

	case DELEGATECALL:
		callType = runtime.DelegateCall

	case STATICCALL:
		callType = runtime.StaticCall

	default:
		panic("not expected")
	}

	contract, err := c.buildCallContract(op)
	if err != nil {
		return nil, err
	}

	ret, gas, err := c.evm.executor.Call(contract, callType)

	if err != nil {
		c.push(big.NewInt(0))
	} else {
		c.push(big.NewInt(1))
	}

	if err == nil || err == runtime.ErrExecutionReverted {
		// TODO, change retOffset
		c.memory.Set(big.NewInt(int64(contract.RetOffset)), big.NewInt(int64(contract.RetSize)), ret)
	}

	c.gas += gas
	return ret, nil
}

func (c *Contract) Depth() int {
	return c.depth
}

/*
// SetPrecompiled sets the precompiled contracts
func (e *EVM) SetPrecompiled(precompiled map[common.Address]*precompiled.Precompiled) {
	e.precompiled = precompiled
}

func (e *EVM) getPrecompiled(addr common.Address) (precompiled.Backend, bool) {
	p, ok := e.precompiled[addr]
	if !ok {
		return nil, false
	}
	if p.ActiveAt > e.env.Number.Uint64() {
		return nil, false
	}
	return p.Backend, true
}
*/

/*
func (c *Contract) call(contract *Contract, op OpCode) ([]byte, error) {
	// Check if its too deep
	if c.Depth() > int(CallCreateDepth) {
		//fmt.Println("TOO DEEP")
		return nil, ErrDepth
	}

	// Check if there is enough balance
	if op == CALL || op == CALLCODE {
		if !c.evm.CanTransfer(c.evm.state, contract.caller, contract.value) {
			return nil, ErrNotEnoughFunds
		}
	}

	contract.snapshot = c.evm.state.Snapshot()

	precompiled, isPrecompiled := c.evm.getPrecompiled(contract.codeAddress)

	if op == CALL {
		if !c.evm.state.Exist(contract.address) {
			if !isPrecompiled && c.evm.config.EIP158 && contract.value.Sign() == 0 {
				// calling an unexisting account
				return nil, nil
			}

			// Not sure why but the address has to be created for the precompiled contracts
			c.evm.state.CreateAccount(contract.address)
		}

		// Try to transfer
		if err := c.evm.Transfer(c.evm.state, contract.caller, contract.address, contract.value); err != nil {
			return nil, err
		}
	}

	var ret []byte
	var err error

	if isPrecompiled {
		if !contract.consumeGas(precompiled.Gas(contract.input)) {
			c.evm.state.RevertToSnapshot(contract.snapshot) // SKETCHY
			contract.consumeAllGas()

			return nil, ErrGasOverflow
		}

		ret, err = precompiled.Call(contract.input)
	} else {
		ret, err = contract.Run()
	}

	if err != nil {
		c.evm.state.RevertToSnapshot(contract.snapshot)
		if err != ErrExecutionReverted {
			contract.consumeAllGas()
		}
	}

	return ret, err
}
*/

func (c *Contract) buildCallContract(op OpCode) (*runtime.Contract, error) {
	var expected int
	if op == CALL || op == CALLCODE {
		expected = 7
	} else {
		expected = 6
	}

	if !c.stackAtLeast(expected) {
		return nil, ErrStackUnderflow
	}

	// Pop input arguments
	initialGas := c.pop()
	addr := common.BigToAddress(c.pop())

	var value *big.Int
	if op == CALL || op == CALLCODE {
		value = c.pop()
	}

	inOffset, inSize := c.pop(), c.pop()
	retOffset, retSize := c.pop(), c.pop()

	// Calculate and consume gas cost

	// Memory cost needs to consider both input and output resizes (HACK)
	in := calcMemSize(inOffset, inSize)
	ret := calcMemSize(retOffset, retSize)

	max := math.BigMax(in, ret)

	memSize, overflow := bigUint64(max)
	if overflow {
		return nil, ErrGasOverflow
	}

	if _, overflow := math.SafeMul(numWords(memSize), 32); overflow {
		return nil, ErrGasOverflow
	}

	memoryGas, err := c.memory.Resize(max.Uint64())
	if err != nil {
		return nil, err
	}

	args, _, err := c.memory.Get(inOffset, inSize)
	if err != nil {
		return nil, err
	}

	gasCost := c.evm.gasTable.Calls
	eip158 := c.evm.config.EIP158
	transfersValue := value != nil && value.Sign() != 0

	if op == CALL {
		if eip158 {
			if transfersValue && c.evm.state.Empty(addr) {
				gasCost += CallNewAccountGas
			}
		} else if !c.evm.state.Exist(addr) {
			gasCost += CallNewAccountGas
		}
	}
	if op == CALL || op == CALLCODE {
		if transfersValue {
			gasCost += CallValueTransferGas
		}
	}

	if gasCost, overflow = math.SafeAdd(gasCost, memoryGas); overflow {
		return nil, ErrGasOverflow
	}

	gas, err := callGas(c.evm.gasTable, c.gas, gasCost, initialGas)
	if err != nil {
		return nil, err
	}

	if gasCost, overflow = math.SafeAdd(gasCost, gas); overflow {
		return nil, ErrGasOverflow
	}

	// Consume gas cost
	if !c.consumeGas(gasCost) {
		return nil, ErrGasConsumed
	}

	if op == CALL || op == CALLCODE {
		if transfersValue {
			gas += CallStipend
		}
	}

	parent := c

	contract := runtime.NewContractCall(c.depth+1, parent.origin, parent.address, addr, value, gas, c.evm.state.GetCode(addr), args)

	contract.RetOffset = retOffset.Uint64()
	contract.RetSize = retSize.Uint64()

	if op == STATICCALL || parent.static {
		contract.Static = true
	}
	if op == CALLCODE || op == DELEGATECALL {
		contract.Address = parent.address
		if op == DELEGATECALL {
			contract.Value = parent.value
			contract.Caller = parent.caller
		}
	}

	return contract, nil
}

func (c *Contract) executeExtCodeHashOperation() error {
	if !c.evm.config.Constantinople {
		return ErrOpcodeNotFound
	}

	addr := c.pop()
	if addr == nil {
		return ErrStackUnderflow
	}

	address := common.BigToAddress(addr)
	if c.evm.state.Empty(address) {
		c.push(big.NewInt(0))
	} else {
		c.push(big.NewInt(0).SetBytes(c.evm.state.GetCodeHash(address).Bytes()))
	}

	return nil
}

func (c *Contract) executeHaltOperations(op OpCode) ([]byte, error) {
	if op == REVERT && !c.evm.config.Byzantium {
		return nil, ErrOpcodeNotFound
	}

	var rett []byte

	if !c.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	offset, size := c.pop(), c.pop()

	if _, overflow := bigUint64(size); overflow {
		return nil, ErrGasOverflow
	}

	ret, gas, err := c.memory.Get(offset, size)
	if err != nil {
		return nil, err
	}

	if !c.consumeGas(gas) {
		return nil, ErrGasConsumed
	}

	rett = ret

	if op == REVERT {
		return rett, runtime.ErrExecutionReverted
	}
	return rett, nil
}

func (c *Contract) selfDestruct() error {

	addr := c.pop()
	if addr == nil {
		return ErrStackUnderflow
	}

	address := common.BigToAddress(addr)

	// try to remove the gas first
	var gas uint64

	// EIP150 homestead gas reprice fork:
	if c.evm.config.EIP150 {
		gas = c.evm.gasTable.Suicide

		eip158 := c.evm.config.EIP158

		if eip158 {
			// if empty and transfers value
			if c.evm.state.Empty(address) && c.evm.state.GetBalance(c.address).Sign() != 0 {
				gas += c.evm.gasTable.CreateBySuicide
			}
		} else if !c.evm.state.Exist(address) {
			gas += c.evm.gasTable.CreateBySuicide
		}
	}

	if !c.evm.state.HasSuicided(c.address) {
		c.evm.state.AddRefund(SuicideRefundGas)
	}

	if !c.consumeGas(gas) {
		return ErrGasConsumed
	}

	balance := c.evm.state.GetBalance(c.address)
	c.evm.state.AddBalance(address, balance)
	c.evm.state.Suicide(c.address)

	return nil
}

func bigUint64(v *big.Int) (uint64, bool) {
	return v.Uint64(), v.BitLen() > 64
}

func (c *Contract) executeExtCodeCopy() error {
	if !c.stackAtLeast(4) {
		return ErrStackUnderflow
	}

	address, memOffset, codeOffset, length := c.pop(), c.pop(), c.pop(), c.pop()

	codeCopy := getSlice(c.evm.state.GetCode(common.BigToAddress(address)), codeOffset, length)

	gas, err := c.memory.Set(memOffset, length, codeCopy)
	if err != nil {
		return err
	}

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, c.evm.gasTable.ExtcodeCopy); overflow {
		return ErrGasOverflow
	}

	words, overflow := bigUint64(length)
	if overflow {
		return ErrGasOverflow
	}

	if words, overflow = math.SafeMul(numWords(words), CopyGas); overflow {
		return ErrGasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, words); overflow {
		return ErrGasOverflow
	}

	if !c.consumeGas(gas) {
		return ErrGasConsumed
	}
	return nil
}

// copy values to memory
func (c *Contract) executeContextCopyOperations(op OpCode) error {
	if !c.stackAtLeast(3) {
		return ErrStackUnderflow
	}

	memOffset, dataOffset, length := c.pop(), c.pop(), c.pop()

	var gas uint64
	var err error

	switch op {
	case CALLDATACOPY:
		gas, err = c.memory.Set(memOffset, length, getSlice(c.input, dataOffset, length))

	case RETURNDATACOPY:
		if !c.evm.config.Byzantium {
			return ErrOpcodeNotFound
		}

		end := big.NewInt(1).Add(dataOffset, length)
		if end.BitLen() > 64 || uint64(len(c.returnData)) < end.Uint64() {
			return fmt.Errorf("out of bounds")
		}
		gas, err = c.memory.Set(memOffset, length, c.returnData[dataOffset.Uint64():end.Uint64()])

	case CODECOPY:
		gas, err = c.memory.Set(memOffset, length, getSlice(c.code, dataOffset, length))

	default:
		return fmt.Errorf("copy bad opcode found: %s", op.String())
	}

	if err != nil {
		return err
	}

	// calculate gas

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, GasFastestStep); overflow {
		return ErrGasOverflow
	}

	words, overflow := bigUint64(length)
	if overflow {
		return ErrGasOverflow
	}

	if words, overflow = math.SafeMul(numWords(words), CopyGas); overflow {
		return ErrGasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, words); overflow {
		return ErrGasOverflow
	}

	if !c.consumeGas(gas) {
		return ErrGasOverflow
	}
	return nil
}

func getData(data []byte, start uint64, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return common.RightPadBytes(data[start:end], int(size))
}

func getSlice(data []byte, start *big.Int, size *big.Int) []byte {
	dlen := big.NewInt(int64(len(data)))

	s := math.BigMin(start, dlen)
	e := math.BigMin(new(big.Int).Add(s, size), dlen)
	return common.RightPadBytes(data[s.Uint64():e.Uint64()], int(size.Uint64()))
}

func (c *Contract) executeContextOperations(op OpCode) (*big.Int, error) {
	switch op {
	case ADDRESS:
		return c.address.Big(), nil

	case BALANCE:
		addr := c.pop()
		if addr == nil {
			return nil, ErrStackUnderflow
		}
		return c.evm.state.GetBalance(common.BigToAddress(addr)), nil

	case ORIGIN:
		return c.origin.Big(), nil

	case CALLER:
		return c.caller.Big(), nil

	case CALLVALUE:
		value := c.value
		if value == nil {
			return big.NewInt(0), nil
		} else {
			return value, nil
		}

	case CALLDATALOAD:
		offset := c.pop()
		if offset == nil {
			return nil, ErrStackUnderflow
		}

		return big.NewInt(1).SetBytes(getSlice(c.input, offset, big.NewInt(32))), nil

	case CALLDATASIZE:
		return big.NewInt(int64(len(c.input))), nil

	case CODESIZE:
		return big.NewInt(int64(len(c.code))), nil

	case EXTCODESIZE:
		addr := c.pop()
		if addr == nil {
			return nil, ErrStackUnderflow
		}
		return big.NewInt(int64(c.evm.state.GetCodeSize(common.BigToAddress(addr)))), nil

	case GASPRICE:
		return c.evm.env.GasPrice, nil

	case RETURNDATASIZE:
		if !c.evm.config.Byzantium {
			return nil, ErrOpcodeNotFound
		}
		return big.NewInt(int64(len(c.returnData))), nil

	default:
		return nil, fmt.Errorf("context bad opcode found: %s", op.String())
	}
}

func (c *Contract) executeBlockInformation(op OpCode) (*big.Int, error) {
	switch op {
	case BLOCKHASH:
		num := c.pop()
		if num == nil {
			return nil, ErrStackUnderflow
		}
		n := big.NewInt(1).Sub(c.evm.env.Number, common.Big257)
		if num.Cmp(n) > 0 && num.Cmp(c.evm.env.Number) < 0 {
			return c.evm.getHash(num.Uint64()).Big(), nil
		}
		return big.NewInt(0), nil

	case COINBASE:
		return c.evm.env.Coinbase.Big(), nil

	case TIMESTAMP:
		return math.U256(c.evm.env.Timestamp), nil

	case NUMBER:
		return math.U256(c.evm.env.Number), nil

	case DIFFICULTY:
		return math.U256(c.evm.env.Difficulty), nil

	case GASLIMIT:
		return math.U256(c.evm.env.GasLimit), nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

// do it there but add the helper functions
func (c *Contract) executeUnsignedArithmeticOperations(op OpCode) (*big.Int, error) {
	if !c.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := c.pop(), c.pop()

	switch op {
	case ADD:
		return math.U256(big.NewInt(0).Add(x, y)), nil

	case MUL:
		return math.U256(big.NewInt(0).Mul(x, y)), nil

	case SUB:
		return math.U256(big.NewInt(0).Sub(x, y)), nil

	case DIV:
		if y.Sign() == 0 {
			return big.NewInt(0), nil
		}
		return math.U256(big.NewInt(0).Div(x, y)), nil

	case SDIV:
		x, y = math.S256(x), math.S256(y)
		res := big.NewInt(0)

		if y.Sign() == 0 || x.Sign() == 0 {
			return big.NewInt(0), nil
		} else if x.Sign() != y.Sign() {
			res.Div(x.Abs(x), y.Abs(y))
			res.Neg(res)
		} else {
			res.Div(x.Abs(x), y.Abs(y))
		}
		return math.U256(res), nil

	case MOD:
		if y.Sign() == 0 {
			return big.NewInt(0), nil
		}
		return math.U256(big.NewInt(0).Mod(x, y)), nil

	case SMOD:
		x, y = math.S256(x), math.S256(y)
		res := big.NewInt(0)

		if y.Sign() == 0 {
			return res, nil
		}
		if x.Sign() < 0 {
			res.Mod(x.Abs(x), y.Abs(y))
			res.Neg(res)
		} else {
			res.Mod(x.Abs(x), y.Abs(y))
		}

		return math.U256(res), nil

	case EXP:
		base, exponent := x, y

		expByteLen := uint64((exponent.BitLen() + 7) / 8)
		cmpToOne := exponent.Cmp(big.NewInt(1))

		var res *big.Int
		if cmpToOne < 0 {
			res = big.NewInt(1)
		} else if base.Sign() == 0 {
			res = big.NewInt(0)
		} else if cmpToOne == 0 {
			res = base
		} else {
			res = math.Exp(base, exponent)
		}

		gas := expByteLen * c.evm.gasTable.ExpByte
		overflow := false

		if gas, overflow = math.SafeAdd(gas, GasSlowStep); overflow {
			return nil, ErrGasOverflow
		}
		if !c.consumeGas(gas) {
			return nil, ErrGasConsumed
		}

		return res, nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

func (c *Contract) executeSignExtension() (*big.Int, error) {
	back := c.pop()
	if back == nil {
		return nil, ErrStackUnderflow
	}

	if back.Cmp(big.NewInt(31)) < 0 {
		bit := uint(back.Uint64()*8 + 7)
		num := c.pop()
		if num == nil {
			return nil, ErrStackUnderflow
		}

		mask := big.NewInt(1).Lsh(common.Big1, bit)
		mask = big.NewInt(1).Sub(mask, common.Big1)

		res := big.NewInt(1)
		if num.Bit(int(bit)) > 0 {
			res.Or(num, big.NewInt(1).Not(mask))
		} else {
			res.And(num, mask)
		}
		return math.U256(res), nil
	}

	return nil, nil
}

func (c *Contract) executeModularOperations(op OpCode) (*big.Int, error) {
	if !c.stackAtLeast(3) {
		return nil, ErrStackUnderflow
	}

	x, y, z := c.pop(), c.pop(), c.pop()

	res := big.NewInt(0)
	switch op {
	case ADDMOD:
		if z.Cmp(big.NewInt(0)) > 0 {
			res.Add(x, y)
			res.Mod(res, z)
			return math.U256(res), nil
		}
		return big.NewInt(0), nil

	case MULMOD:
		if z.Cmp(big.NewInt(0)) > 0 {
			res.Mul(x, y)
			res.Mod(res, z)
			return math.U256(res), nil
		}
		return big.NewInt(0), nil

	default:
		return nil, fmt.Errorf("modular bad opcode found: %s", op.String())
	}
}

func (c *Contract) executeBitWiseOperations1(op OpCode) (*big.Int, error) {
	x := c.pop()
	if x == nil {
		return nil, ErrStackUnderflow
	}

	switch op {
	case ISZERO:
		if x.Sign() > 0 {
			return big.NewInt(0), nil
		}
		return big.NewInt(1), nil

	case NOT:
		return math.U256(big.NewInt(1).Not(x)), nil

	default:
		return nil, fmt.Errorf("bitwise1 bad opcode found: %s", op.String())
	}
}

func (c *Contract) executeBitWiseOperations2(op OpCode) (*big.Int, error) {
	if !c.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := c.pop(), c.pop()

	switch op {
	case AND:
		return big.NewInt(0).And(x, y), nil

	case OR:
		return big.NewInt(0).Or(x, y), nil

	case XOR:
		return big.NewInt(0).Xor(x, y), nil

	case BYTE:
		if x.Cmp(common.Big32) < 0 {
			return big.NewInt(1).SetUint64(uint64(math.Byte(y, 32, int(x.Int64())))), nil
		}
		return big.NewInt(0), nil

	default:
		return nil, fmt.Errorf("bitwise2 bad opcode found: %s", op.String())
	}
}

func (c *Contract) executeShiftOperations(op OpCode) (*big.Int, error) {
	if !c.evm.config.Constantinople {
		return nil, ErrOpcodeNotFound
	}

	if !c.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := c.pop(), c.pop()

	shift := math.U256(x)

	switch op {
	case SHL:
		value := math.U256(y)
		if shift.Cmp(common.Big256) >= 0 {
			return big.NewInt(0), nil
		}
		return math.U256(value.Lsh(value, uint(shift.Uint64()))), nil

	case SHR:
		value := math.U256(y)
		if shift.Cmp(common.Big256) >= 0 {
			return big.NewInt(0), nil
		}
		return math.U256(value.Rsh(value, uint(shift.Uint64()))), nil

	case SAR:
		value := math.S256(y)
		if shift.Cmp(common.Big256) >= 0 {
			if value.Sign() >= 0 {
				return math.U256(big.NewInt(0)), nil
			}
			return math.U256(big.NewInt(-1)), nil
		}
		return math.U256(value.Rsh(value, uint(shift.Uint64()))), nil

	default:
		return nil, fmt.Errorf("shift bad opcode found: %s", op.String())
	}
}

func (c *Contract) executeComparison(op OpCode) (*big.Int, error) {
	if !c.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := c.pop(), c.pop()

	var res bool
	switch op {
	case EQ:
		res = x.Cmp(y) == 0

	case LT:
		res = x.Cmp(y) < 0

	case GT:
		res = x.Cmp(y) > 0

	case SLT:
		return sltComparison(x, y), nil

	case SGT:
		return sgtComparison(x, y), nil

	default:
		return nil, fmt.Errorf("comparison bad opcode found: %s", op.String())
	}

	if res {
		return big.NewInt(1), nil
	}
	return big.NewInt(0), nil
}

func sltComparison(x, y *big.Int) *big.Int {
	xSign := x.Cmp(tt255)
	ySign := y.Cmp(tt255)

	if xSign >= 0 && ySign < 0 {
		return big.NewInt(1)
	} else if xSign < 0 && ySign >= 0 {
		return big.NewInt(0)
	}

	if x.Cmp(y) < 0 {
		return big.NewInt(1)
	}
	return big.NewInt(0)
}

func sgtComparison(x, y *big.Int) *big.Int {
	xSign := x.Cmp(tt255)
	ySign := y.Cmp(tt255)

	if xSign >= 0 && ySign < 0 {
		return big.NewInt(0)
	} else if xSign < 0 && ySign >= 0 {
		return big.NewInt(1)
	}

	if x.Cmp(y) > 0 {
		return big.NewInt(1)
	}
	return big.NewInt(0)
}

func (c *Contract) inStaticCall() bool {
	return c.static
}

// bitmap

type bitvec []byte

func (bits *bitvec) set(pos uint64) {
	(*bits)[pos/8] |= 0x80 >> (pos % 8)
}
func (bits *bitvec) set8(pos uint64) {
	(*bits)[pos/8] |= 0xFF >> (pos % 8)
	(*bits)[pos/8+1] |= ^(0xFF >> (pos % 8))
}

// codeSegment checks if the position is in a code segment.
func (bits *bitvec) codeSegment(pos uint64) bool {
	return ((*bits)[pos/8] & (0x80 >> (pos % 8))) == 0
}

// codeBitmap collects data locations in code.
func codeBitmap(code []byte) bitvec {
	// The bitmap is 4 bytes longer than necessary, in case the code
	// ends with a PUSH32, the algorithm will push big.NewInt(0)es onto the
	// bitvector outside the bounds of the actual code.
	bits := make(bitvec, len(code)/8+1+4)
	for pc := uint64(0); pc < uint64(len(code)); {
		op := OpCode(code[pc])

		if op >= PUSH1 && op <= PUSH32 {
			numbits := op - PUSH1 + 1
			pc++
			for ; numbits >= 8; numbits -= 8 {
				bits.set8(pc) // 8
				pc += 8
			}
			for ; numbits > 0; numbits-- {
				bits.set(pc)
				pc++
			}
		} else {
			pc++
		}
	}
	return bits
}

func callGas(gasTable chain.GasTable, availableGas, base uint64, callCost *big.Int) (uint64, error) {
	if gasTable.CreateBySuicide > 0 {
		availableGas = availableGas - base
		gas := availableGas - availableGas/64
		// If the bit length exceeds 64 bit we know that the newly calculated "gas" for EIP150
		// is smaller than the requested amount. Therefor we return the new gas instead
		// of returning an error.
		if callCost.BitLen() > 64 || gas < callCost.Uint64() {
			return gas, nil
		}
	}
	if callCost.BitLen() > 64 {
		return 0, ErrGasOverflow
	}

	return callCost.Uint64(), nil
}
