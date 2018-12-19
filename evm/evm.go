package evm

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/chain"

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
	ErrNotEnoughFunds           = fmt.Errorf("not enough funds")
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

// Env refers to the block information the transactions runs in
// it is shared for all the contracts executed so its in the EVM.
type Env struct {
	BlockHash  common.Hash
	Coinbase   common.Address
	Timestamp  *big.Int
	Number     *big.Int
	Difficulty *big.Int
	GasLimit   *big.Int
	GasPrice   *big.Int
}

type CanTransferFunc func(*state.StateDB, common.Address, *big.Int) bool

type TransferFunc func(state *state.StateDB, from common.Address, to common.Address, amount *big.Int) error

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

	// inputs
	value *big.Int // value of the tx
	input []byte   // Input of the tx
	gas   uint64

	// type of contract
	creation bool
	static   bool

	retOffset uint64
	retSize   uint64

	bitvec bitvec

	snapshot int
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

func (c *Contract) showStack() string {
	str := []string{}
	for i := 0; i < c.sp; i++ {
		str = append(str, c.stack[i].String())
	}
	return "Stack: " + strings.Join(str, ",")
}

func newContract(origin common.Address, from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	f := &Contract{
		ip:          -1,
		code:        code,
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

func newContractCreation(origin common.Address, from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	c := newContract(origin, from, to, value, gas, code)
	c.creation = true
	return c
}

func newContractCall(origin common.Address, from common.Address, to common.Address, value *big.Int, gas uint64, code []byte, input []byte) *Contract {
	c := newContract(origin, from, to, value, gas, code)
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

	state *state.StateDB
	env   *Env

	getHash     GetHashByNumber
	CanTransfer CanTransferFunc
	Transfer    TransferFunc

	returnData []byte

	snapshot int
}

// NewEVM creates a new EVM
func NewEVM(state *state.StateDB, env *Env, config chain.ForksInTime, gasTable chain.GasTable, getHash GetHashByNumber) *EVM {
	return &EVM{
		contracts:      make([]*Contract, MaxContracts),
		config:         config,
		gasTable:       gasTable,
		contractsIndex: 0,
		state:          state,
		env:            env,
		getHash:        getHash,
		returnData:     []byte{},
		CanTransfer:    CanTransfer,
		Transfer:       Transfer,
	}
}

// Call calls a specific contract
func (e *EVM) Call(caller common.Address, to common.Address, input []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	contract := newContractCall(caller, caller, to, value, gas, e.state.GetCode(to), input)

	if err := e.call(contract, CALL); err != nil {
		if contract.snapshot != -1 {
			e.state.RevertToSnapshot(contract.snapshot)
		}
		return nil, 0, err
	}

	if err := e.Run(); err != nil {
		return nil, 0, err
	}

	c := e.currentContract()
	return e.returnData, c.gas, nil
}

var emptyCodeHash = crypto.Keccak256Hash(nil)

// Create creates a new contract
func (e *EVM) Create(caller common.Address, code []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	address := crypto.CreateAddress(caller, e.state.GetNonce(caller))
	contract := newContractCreation(caller, caller, address, value, gas, code)

	if err := e.create(contract); err != nil {
		if contract.snapshot != -1 {
			e.state.RevertToSnapshot(contract.snapshot)
		}
		return nil, 0, err
	}

	if err := e.Run(); err != nil {
		return nil, 0, err
	}

	c := e.currentContract()
	return e.returnData, c.gas, nil
}

func (e *EVM) calculateFixedGasUsage(op OpCode) uint64 {
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

	case PC, GAS, MSIZE, POP, GASLIMIT, DIFFICULTY, NUMBER, TIMESTAMP, COINBASE, GASPRICE, CODESIZE, CALLDATASIZE, CALLVALUE, CALLER, ORIGIN, ADDRESS, RETURNDATASIZE:
		return GasQuickStep

	case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
		return GasFastestStep

	case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
		return GasFastestStep

	case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
		return GasFastestStep

	case BALANCE:
		return e.gasTable.Balance

	case EXTCODEHASH:
		return e.gasTable.ExtcodeHash

	case EXTCODESIZE:
		return e.gasTable.ExtcodeSize

	case SLOAD:
		return e.gasTable.SLoad

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
func (e *EVM) Run() error {

	var op OpCode

	for {
		var vmerr error

		for e.currentContract().ip < len(e.currentContract().Instructions())-1 {
			e.currentContract().ip++

			ip := e.currentContract().ip
			ins := e.currentContract().Instructions()
			op = OpCode(ins[ip])

			// consume gas for those opcodes with fixed gas
			if gasUsed := e.calculateFixedGasUsage(op); gasUsed != 0 {
				if !e.currentContract().consumeGas(gasUsed) {
					vmerr = ErrGasConsumed
					goto END
				}
			}

			switch op {
			case ADD, MUL, SUB, DIV, SDIV, MOD, SMOD, EXP: // add the other operations
				val, err := e.executeUnsignedArithmeticOperations(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			case ADDMOD, MULMOD:
				val, err := e.executeModularOperations(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			case NOT, ISZERO:
				val, err := e.executeBitWiseOperations1(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			case AND, OR, XOR, BYTE:
				val, err := e.executeBitWiseOperations2(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			case EQ, GT, LT, SLT, SGT:
				val, err := e.executeComparison(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			case SHL, SHR, SAR:
				if !e.config.Constantinople {
					vmerr = ErrOpcodeNotFound
					goto END
				}

				val, err := e.executeShiftOperations(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			case SIGNEXTEND:
				val, err := e.executeSignExtension()
				if err != nil {
					vmerr = err
					goto END
				}
				if val != nil {
					e.push(val)
				}

			// --- context ---

			case ADDRESS, BALANCE, ORIGIN, CALLER, CALLVALUE, CALLDATALOAD, CALLDATASIZE, CODESIZE, EXTCODESIZE, GASPRICE, RETURNDATASIZE:
				val, err := e.executeContextOperations(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			// --- context memory copy ---

			case EXTCODECOPY:
				vmerr = e.executeExtCodeCopy()

			case CODECOPY, CALLDATACOPY, RETURNDATACOPY:
				vmerr = e.executeContextCopyOperations(op)

			// --- block information ---

			case BLOCKHASH, COINBASE, TIMESTAMP, NUMBER, DIFFICULTY, GASLIMIT:
				val, err := e.executeBlockInformation(op)
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(val)

			// Push operations

			case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
				n := int(op - PUSH) // i.e. PUSH6, n = 6

				var data []byte
				if ip+1+n > len(ins) {
					data = common.RightPadBytes(ins[ip+1:len(ins)], n)
				} else {
					data = ins[ip+1 : ip+1+n]
				}

				e.push(big.NewInt(0).SetBytes(data))
				e.currentContract().ip += n

			// Duplicate operations

			case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
				n := int(op - DUP)
				if !e.stackAtLeast(n) {
					vmerr = ErrStackUnderflow
					goto END
				}
				e.push(e.peekAt(n))

			// Swap operations

			case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
				n := int(op - SWAP)
				if !e.stackAtLeast(n + 1) {
					vmerr = ErrStackUnderflow
					goto END
				}
				e.swap(n)

			// Logging operations

			case LOG0, LOG1, LOG2, LOG3, LOG4:
				vmerr = e.executeLogsOperation(op)

			// System operations

			case EXTCODEHASH:
				vmerr = e.executeExtCodeHashOperation()

			case CREATE, CREATE2:
				vmerr = e.executeCreateOperation(op)

			case CALL, CALLCODE, DELEGATECALL, STATICCALL:
				vmerr = e.executeCallOperation(op)

			case REVERT, RETURN:
				err := e.executeHaltOperations(op)
				vmerr = err
				goto END

			case SELFDESTRUCT:
				if e.inStaticCall() {
					vmerr = errReadOnly
					goto END
				}
				vmerr = e.selfDestruct()
				goto END

			case STOP:
				goto END

			// --- sha3 ---

			case SHA3:
				vmerr = e.sha3()

			// --- stack ---

			case POP:
				if n := e.pop(); n == nil {
					vmerr = ErrStackUnderflow
				}

			// --- memory ---

			case MLOAD:
				offset := e.pop()
				if offset == nil {
					vmerr = ErrStackUnderflow
					goto END
				}

				data, gas, err := e.currentContract().memory.Get(offset, big.NewInt(32))
				if err != nil {
					vmerr = err
					goto END
				}
				e.push(big.NewInt(1).SetBytes(data))

				gas, overflow := math.SafeAdd(gas, GasFastestStep)
				if overflow {
					vmerr = ErrGasOverflow
					goto END
				}

				if !e.currentContract().consumeGas(gas) {
					vmerr = ErrGasConsumed
					goto END
				}

			case MSTORE:
				// TODO, try to mix mstore8, mstore and mload
				if !e.stackAtLeast(2) {
					vmerr = ErrStackUnderflow
					goto END
				}
				start, val := e.pop(), e.pop()

				gas, err := e.currentContract().memory.Set32(start, val)
				if err != nil {
					vmerr = err
					goto END
				}

				gas, overflow := math.SafeAdd(gas, GasFastestStep)
				if overflow {
					vmerr = ErrGasConsumed
					goto END
				}

				if !e.currentContract().consumeGas(gas) {
					vmerr = ErrGasConsumed
					goto END
				}

			case MSTORE8:
				if !e.stackAtLeast(2) {
					vmerr = ErrStackUnderflow
					goto END
				}

				offset, val := e.pop(), e.pop().Int64()

				gas, err := e.currentContract().memory.SetByte(offset, val)
				if err != nil {
					vmerr = err
					goto END
				}
				gas, overflow := math.SafeAdd(gas, GasFastestStep)
				if overflow {
					vmerr = ErrGasOverflow
					goto END
				}

				if !e.currentContract().consumeGas(gas) {
					vmerr = ErrGasConsumed
					goto END
				}

			// --- storage ---

			case SLOAD:
				loc := e.pop()
				if loc == nil {
					vmerr = ErrStackUnderflow
					goto END
				}
				val := e.state.GetState(e.currentContract().address, common.BigToHash(loc))
				e.push(val.Big())

			case SSTORE:
				vmerr = e.executeSStoreOperation()

			// --- flow ---

			case JUMP:
				dest := e.pop()
				if dest == nil {
					vmerr = ErrStackUnderflow
					goto END
				}

				if !e.currentContract().validJumpdest(dest) {
					vmerr = ErrJumpDestNotValid
					goto END
				}
				e.currentContract().ip = int(dest.Uint64() - 1)

			case JUMPI:
				if !e.stackAtLeast(2) {
					vmerr = ErrStackUnderflow
					goto END
				}
				dest, cond := e.pop(), e.pop()

				if cond.Sign() != 0 {
					if !e.currentContract().validJumpdest(dest) {
						vmerr = ErrJumpDestNotValid
						goto END
					}
					e.currentContract().ip = int(dest.Uint64() - 1)
				}

			case JUMPDEST:
				// Nothing

			case PC:
				e.push(big.NewInt(int64(e.currentContract().ip)))

			case MSIZE:
				e.push(e.currentContract().MemoryLen())

			case GAS:
				e.push(big.NewInt(int64(e.currentContract().gas)))

			default:
				if strings.Contains(op.String(), "Missing") {
					vmerr = ErrOpcodeNotFound
					goto END
				}
				return fmt.Errorf("opcode not found: %s", op.String())
			}

			if e.currentContract().sp > 1024 {
				vmerr = ErrStackOverflow
				goto END
			}

			if vmerr != nil {
				break
			}
		}

	END:

		// need to handle first the error to consume the gas at least
		c := e.currentContract()

		// consume all the gas of the current contract
		if vmerr != nil {
			// only if its a smart contract error,
			if vmerr != ErrNotEnoughFunds && vmerr != ErrDepth {
				c.consumeGas(c.gas)
			}
		}

		// If its the last contract, just stop the loop
		if e.contractsIndex == 1 {
			// revert the state
			if vmerr != nil || op == REVERT {
				e.state.RevertToSnapshot(e.currentContract().snapshot)
			}

			return vmerr
		}

		// Otherwise, pop the last contract and fill the return fields
		e.popContract()

		// Set return codes
		if vmerr != nil || op == REVERT {
			e.push(big.NewInt(0))
		} else {
			if c.creation {
				e.push(c.address.Big())
			} else {
				e.push(big.NewInt(1))
			}
		}

		// Set the state on memory for the contract calls
		if !c.creation && vmerr == nil && len(e.returnData) != 0 {
			// return offset values are stored in the child contract
			retOffset, retSize := c.retOffset, c.retSize

			if _, err := e.currentContract().memory.Set(big.NewInt(int64(retOffset)), big.NewInt(int64(retSize)), e.returnData); err != nil {
				panic(fmt.Errorf("This memory error should not happen: %v", err))
			}
		}

		// Remove return data if there is an error
		if vmerr != nil {
			e.returnData = []byte{}
		}

		// Return the gas
		e.currentContract().gas += c.gas

		// revert the state
		if vmerr != nil || op == REVERT {
			if c.snapshot == -1 {
				if vmerr != ErrNotEnoughFunds && vmerr != ErrContractAddressCollision && vmerr != ErrDepth {
					panic("there should be a snapshot")
				}
			} else {
				e.state.RevertToSnapshot(c.snapshot)
			}
		}
	}
}

func (e *EVM) isLastContract() bool {
	return e.contractsIndex == 1
}

func (e *EVM) executeSStoreOperation() error {
	if e.inStaticCall() {
		return errReadOnly
	}
	if !e.stackAtLeast(2) {
		return ErrStackUnderflow
	}

	address := e.currentContract().address

	loc, val := e.pop(), e.pop()

	var gas uint64

	current := e.state.GetState(address, common.BigToHash(loc))

	// discount gas (constantinople)
	if !e.config.Constantinople {
		switch {
		case current == (common.Hash{}) && val.Sign() != 0: // 0 => non 0
			gas = SstoreSetGas
		case current != (common.Hash{}) && val.Sign() == 0: // non 0 => 0
			e.state.AddRefund(SstoreRefundGas)
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
			original := e.state.GetCommittedState(address, common.BigToHash(loc))
			if original == current {
				if original == (common.Hash{}) { // create slot (2.1.1)
					return NetSstoreInitGas
				}
				if value == (common.Hash{}) { // delete slot (2.1.2b)
					e.state.AddRefund(NetSstoreClearRefund)
				}
				return NetSstoreCleanGas // write existing slot (2.1.2)
			}
			if original != (common.Hash{}) {
				if current == (common.Hash{}) { // recreate slot (2.2.1.1)
					e.state.SubRefund(NetSstoreClearRefund)
				} else if value == (common.Hash{}) { // delete slot (2.2.1.2)
					e.state.AddRefund(NetSstoreClearRefund)
				}
			}
			if original == value {
				if original == (common.Hash{}) { // reset to original inexistent slot (2.2.2.1)
					e.state.AddRefund(NetSstoreResetClearRefund)
				} else { // reset to original existing slot (2.2.2.2)
					e.state.AddRefund(NetSstoreResetRefund)
				}
			}
			return NetSstoreDirtyGas
		}

		gas = getGas()
	}

	if !e.currentContract().consumeGas(gas) {
		return ErrGasOverflow
	}

	e.state.SetState(address, common.BigToHash(loc), common.BigToHash(val))
	return nil
}

func (e *EVM) executeLogsOperation(op OpCode) error {
	if e.inStaticCall() {
		return errReadOnly
	}

	size := int(op - LOG)
	topics := make([]common.Hash, size)

	if !e.stackAtLeast(2 + size) {
		return ErrStackUnderflow
	}

	mStart, mSize := e.pop(), e.pop()
	for i := 0; i < size; i++ {
		topics[i] = common.BigToHash(e.pop())
	}

	data, gas, err := e.currentContract().memory.Get(mStart, mSize)
	if err != nil {
		return err
	}
	e.state.AddLog(&types.Log{
		Address:     e.currentContract().address,
		Topics:      topics,
		Data:        data,
		BlockNumber: e.env.Number.Uint64(),
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

	if !e.currentContract().consumeGas(gas) {
		return ErrGasConsumed
	}
	return nil
}

func (e *EVM) sha3() error {

	if !e.stackAtLeast(2) {
		return ErrStackUnderflow
	}

	offset, size := e.pop(), e.pop()

	data, gas, err := e.currentContract().memory.Get(offset, size)
	if err != nil {

		return err
	}

	hash := crypto.Keccak256Hash(data)
	e.push(hash.Big())

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

	if !e.currentContract().consumeGas(gas) {
		return ErrGasConsumed
	}
	return nil
}

func (e *EVM) create(contract *Contract) error {
	e.pushContract(contract)

	// Check if its too deep
	if e.Depth() > int(CallCreateDepth)+1 {
		return ErrDepth
	}

	caller, address, value := contract.caller, contract.address, contract.value

	// Check if the values can be transfered
	if !e.CanTransfer(e.state, caller, value) {
		return ErrNotEnoughFunds
	}

	// Increase the nonce of the caller
	nonce := e.state.GetNonce(caller)
	e.state.SetNonce(caller, nonce+1)

	// Check for address collisions
	contractHash := e.state.GetCodeHash(address)
	if e.state.GetNonce(address) != 0 || (contractHash != (common.Hash{}) && contractHash != emptyCodeHash) {
		return ErrContractAddressCollision
	}

	// Take snapshot of the current state
	contract.snapshot = e.state.Snapshot()

	// Create the new account for the contract
	e.state.CreateAccount(address)
	if e.config.EIP158 {
		e.state.SetNonce(address, 1)
	}

	// Transfer the value
	if value != nil {
		if err := e.Transfer(e.state, caller, address, value); err != nil {
			return ErrNotEnoughFunds
		}
	}

	return nil
}

func (e *EVM) buildCreateContract(op OpCode) (*Contract, error) {
	var expected int
	if op == CREATE {
		expected = 3
	} else if op == CREATE2 {
		expected = 4
	} else {
		panic(fmt.Errorf("Only CREATE or CREATE2 expected: Found %s", op.String()))
	}

	if !e.stackAtLeast(expected) {
		return nil, ErrStackUnderflow
	}

	e.returnData = nil

	// Pop input arguments
	value := e.pop()
	offset, size := e.pop(), e.pop()

	var salt *big.Int
	if op == CREATE2 {
		salt = e.pop()
	}

	// Calculate and consume gas cost

	var overflow bool
	var gasCost uint64

	// Both CREATE and CREATE2 use memory
	input, gasCost, err := e.currentContract().memory.Get(offset, size)
	if err != nil {
		return nil, err
	}

	gasParam := CreateGas
	if op == CREATE2 {
		// Need to add the sha3 gas cost
		wordGas, overflow := bigUint64(size)
		if overflow {
			return nil, ErrGasOverflow
		}
		if wordGas, overflow = math.SafeMul(numWords(wordGas), Sha3WordGas); overflow {
			return nil, ErrGasOverflow
		}
		if gasCost, overflow = math.SafeAdd(gasCost, wordGas); overflow {
			return nil, ErrGasOverflow
		}

		gasParam = Create2Gas
	}

	if gasCost, overflow = math.SafeAdd(gasCost, gasParam); overflow {
		return nil, ErrGasOverflow
	}

	if !e.currentContract().consumeGas(gasCost) {
		return nil, ErrGasOverflow
	}

	// Calculate and consume gas for the call
	gas := e.currentContract().gas

	// CREATE2 uses by default EIP150
	if e.config.EIP150 || op == CREATE2 {
		gas -= gas / 64
	}

	if !e.currentContract().consumeGas(gas) {
		return nil, ErrGasOverflow
	}

	// Calculate address
	var address common.Address
	if op == CREATE {
		address = crypto.CreateAddress(e.currentContract().address, e.state.GetNonce(e.currentContract().address))
	} else {
		address = crypto.CreateAddress2(e.currentContract().address, common.BigToHash(salt), crypto.Keccak256Hash(input).Bytes())
	}

	contract := newContractCreation(e.currentContract().origin, e.currentContract().address, address, value, gas, input)
	return contract, nil
}

func (e *EVM) executeCreateOperation(op OpCode) error {
	if e.inStaticCall() {
		return errReadOnly
	}

	if op == CREATE2 {
		if !e.config.Constantinople {
			return ErrOpcodeNotFound
		}
	}

	contract, err := e.buildCreateContract(op)
	if err != nil {
		return err
	}

	return e.create(contract)
}

func (e *EVM) executeCallOperation(op OpCode) error {
	if op == CALL && e.inStaticCall() {
		if val := e.peekAt(3); val != nil && val.BitLen() > 0 {
			return errReadOnly
		}
	}

	if op == DELEGATECALL && !e.config.Homestead {
		return ErrOpcodeNotFound
	}
	if op == STATICCALL && !e.config.Byzantium {
		return ErrOpcodeNotFound
	}

	contract, err := e.buildCallContract(op)
	if err != nil {
		return err
	}

	return e.call(contract, op)
}

func (e *EVM) call(contract *Contract, op OpCode) error {
	e.pushContract(contract)

	// Check if its too deep
	if e.Depth() > int(CallCreateDepth)+1 {
		return ErrDepth
	}

	// Check if there is enough balance
	if op == CALL || op == CALLCODE {
		if !e.CanTransfer(e.state, contract.caller, contract.value) {
			return ErrNotEnoughFunds
		}
	}

	contract.snapshot = e.state.Snapshot()

	// check first if its precompiled
	precompiledContracts := ContractsHomestead
	if e.config.Byzantium {
		precompiledContracts = ContractsByzantium
	}

	precompiled, isPrecompiled := precompiledContracts[contract.codeAddress]

	if op == CALL {
		if !e.state.Exist(contract.address) {
			if !isPrecompiled && e.config.EIP158 && contract.value.Sign() == 0 {
				// calling an unexisting account
				return nil
			}

			// Not sure why but the address has to be created for the precompiled contracts
			e.state.CreateAccount(contract.address)
		}

		// Try to transfer
		if err := e.Transfer(e.state, contract.caller, contract.address, contract.value); err != nil {
			return err
		}
	}

	if isPrecompiled {
		if !contract.consumeGas(precompiled.Gas(contract.input)) {
			return ErrGasOverflow
		}

		output, err := precompiled.Call(contract.input)
		if err != nil {
			return err
		}
		e.returnData = output
	}

	return nil
}

func (e *EVM) buildCallContract(op OpCode) (*Contract, error) {
	var expected int
	if op == CALL || op == CALLCODE {
		expected = 7
	} else {
		expected = 6
	}

	if !e.stackAtLeast(expected) {
		return nil, ErrStackUnderflow
	}

	e.returnData = nil

	// Pop input arguments
	initialGas := e.pop()
	addr := common.BigToAddress(e.pop())

	var value *big.Int
	if op == CALL || op == CALLCODE {
		value = e.pop()
	}

	inOffset, inSize := e.pop(), e.pop()
	retOffset, retSize := e.pop(), e.pop()

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

	memoryGas, err := e.currentContract().memory.Resize(max.Uint64())
	if err != nil {
		return nil, err
	}

	args, _, err := e.currentContract().memory.Get(inOffset, inSize)
	if err != nil {
		return nil, err
	}

	gasCost := e.gasTable.Calls
	eip158 := e.config.EIP158
	transfersValue := value != nil && value.Sign() != 0

	if op == CALL {
		if eip158 {
			if transfersValue && e.state.Empty(addr) {
				gasCost += CallNewAccountGas
			}
		} else if !e.state.Exist(addr) {
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

	gas, err := callGas(e.gasTable, e.currentContract().gas, gasCost, initialGas)
	if err != nil {
		return nil, err
	}

	if gasCost, overflow = math.SafeAdd(gasCost, gas); overflow {
		return nil, ErrGasOverflow
	}

	// Consume gas cost
	if !e.currentContract().consumeGas(gasCost) {
		return nil, ErrGasConsumed
	}

	if op == CALL || op == CALLCODE {
		if transfersValue {
			gas += CallStipend
		}
	}

	parent := e.currentContract()

	contract := newContractCall(parent.origin, parent.address, addr, value, gas, e.state.GetCode(addr), args)
	contract.retOffset = retOffset.Uint64()
	contract.retSize = retSize.Uint64()

	if op == STATICCALL || parent.static {
		contract.static = true
	}
	if op == CALLCODE || op == DELEGATECALL {
		contract.address = parent.address
		if op == DELEGATECALL {
			contract.value = parent.value
			contract.caller = parent.caller
		}
	}

	return contract, nil
}

func (e *EVM) Depth() int {
	return e.contractsIndex
}

func (e *EVM) executeExtCodeHashOperation() error {
	if !e.config.Constantinople {
		return ErrOpcodeNotFound
	}

	addr := e.pop()
	if addr == nil {
		return ErrStackUnderflow
	}

	address := common.BigToAddress(addr)
	if e.state.Empty(address) {
		e.push(big.NewInt(0))
	} else {
		e.push(big.NewInt(0).SetBytes(e.state.GetCodeHash(address).Bytes()))
	}

	return nil
}

func (e *EVM) executeHaltOperations(op OpCode) error {
	if op == REVERT && !e.config.Byzantium {
		return ErrOpcodeNotFound
	}

	e.returnData = nil

	if !e.stackAtLeast(2) {
		return ErrStackUnderflow
	}

	offset, size := e.pop(), e.pop()

	if _, overflow := bigUint64(size); overflow {
		return ErrGasOverflow
	}

	ret, gas, err := e.currentContract().memory.Get(offset, size)
	if err != nil {
		return err
	}

	if !e.currentContract().consumeGas(gas) {
		return ErrGasConsumed
	}

	// Return only allowed in calls
	if !e.currentContract().creation {
		e.returnData = ret
	}

	// Return if reverted inside a contract creation
	if e.currentContract().creation && op == REVERT {
		e.returnData = ret
	}

	if op == RETURN {
		if e.currentContract().creation {
			maxCodeSizeExceeded := e.config.EIP158 && len(ret) > MaxCodeSize
			if maxCodeSizeExceeded {
				return ErrMaxCodeSizeExceeded
			}

			createDataGas := uint64(len(ret)) * CreateDataGas
			if !e.currentContract().consumeGas(createDataGas) {
				return ErrGasConsumed
			}
			e.state.SetCode(e.currentContract().address, ret)
		}
	}

	return nil
}

func (e *EVM) selfDestruct() error {

	addr := e.pop()
	if addr == nil {
		return ErrStackUnderflow
	}

	address := common.BigToAddress(addr)

	// try to remove the gas first
	var gas uint64

	// EIP150 homestead gas reprice fork:
	if e.config.EIP150 {
		gas = e.gasTable.Suicide

		eip158 := e.config.EIP158

		if eip158 {
			// if empty and transfers value
			if e.state.Empty(address) && e.state.GetBalance(e.currentContract().address).Sign() != 0 {
				gas += e.gasTable.CreateBySuicide
			}
		} else if !e.state.Exist(address) {
			gas += e.gasTable.CreateBySuicide
		}
	}

	if !e.state.HasSuicided(e.currentContract().address) {
		e.state.AddRefund(SuicideRefundGas)
	}

	if !e.currentContract().consumeGas(gas) {
		return ErrGasConsumed
	}

	balance := e.state.GetBalance(e.currentContract().address)
	e.state.AddBalance(address, balance)
	e.state.Suicide(e.currentContract().address)

	return nil
}

func bigUint64(v *big.Int) (uint64, bool) {
	return v.Uint64(), v.BitLen() > 64
}

func (e *EVM) executeExtCodeCopy() error {
	if !e.stackAtLeast(4) {
		return ErrStackUnderflow
	}

	address, memOffset, codeOffset, length := e.pop(), e.pop(), e.pop(), e.pop()

	codeCopy := getSlice(e.state.GetCode(common.BigToAddress(address)), codeOffset, length)

	gas, err := e.currentContract().memory.Set(memOffset, length, codeCopy)
	if err != nil {
		return err
	}

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, e.gasTable.ExtcodeCopy); overflow {
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

	if !e.currentContract().consumeGas(gas) {
		return ErrGasConsumed
	}
	return nil
}

// copy values to memory
func (e *EVM) executeContextCopyOperations(op OpCode) error {
	if !e.stackAtLeast(3) {
		return ErrStackUnderflow
	}

	memOffset, dataOffset, length := e.pop(), e.pop(), e.pop()

	var gas uint64
	var err error

	switch op {
	case CALLDATACOPY:
		gas, err = e.currentContract().memory.Set(memOffset, length, getSlice(e.currentContract().input, dataOffset, length))

	case RETURNDATACOPY:
		if !e.config.Byzantium {
			return ErrOpcodeNotFound
		}

		end := big.NewInt(1).Add(dataOffset, length)
		if end.BitLen() > 64 || uint64(len(e.returnData)) < end.Uint64() {
			return fmt.Errorf("out of bounds")
		}

		gas, err = e.currentContract().memory.Set(memOffset, length, e.returnData[dataOffset.Uint64():end.Uint64()])

	case CODECOPY:
		gas, err = e.currentContract().memory.Set(memOffset, length, getSlice(e.currentContract().code, dataOffset, length))

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

	if !e.currentContract().consumeGas(gas) {
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

func (e *EVM) executeContextOperations(op OpCode) (*big.Int, error) {
	switch op {
	case ADDRESS:
		return e.currentContract().address.Big(), nil

	case BALANCE:
		addr := e.pop()
		if addr == nil {
			return nil, ErrStackUnderflow
		}

		return e.state.GetBalance(common.BigToAddress(addr)), nil

	case ORIGIN:
		return e.currentContract().origin.Big(), nil

	case CALLER:
		return e.currentContract().caller.Big(), nil

	case CALLVALUE:
		value := e.currentContract().value
		if value == nil {
			return big.NewInt(0), nil
		} else {
			return value, nil
		}

	case CALLDATALOAD:
		offset := e.pop()
		if offset == nil {
			return nil, ErrStackUnderflow
		}

		return big.NewInt(1).SetBytes(getSlice(e.currentContract().input, offset, big.NewInt(32))), nil

	case CALLDATASIZE:
		return big.NewInt(int64(len(e.currentContract().input))), nil

	case CODESIZE:
		return big.NewInt(int64(len(e.currentContract().code))), nil

	case EXTCODESIZE:
		addr := e.pop()
		if addr == nil {
			return nil, ErrStackUnderflow
		}

		return big.NewInt(int64(e.state.GetCodeSize(common.BigToAddress(addr)))), nil

	case GASPRICE:
		return e.env.GasPrice, nil

	case RETURNDATASIZE:
		if !e.config.Byzantium {
			return nil, ErrOpcodeNotFound
		}

		return big.NewInt(int64(len(e.returnData))), nil

	default:
		return nil, fmt.Errorf("context bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeBlockInformation(op OpCode) (*big.Int, error) {
	switch op {
	case BLOCKHASH:
		n := e.pop()
		if n == nil {
			return nil, ErrStackUnderflow
		}
		return e.getHash(n.Uint64()).Big(), nil

	case COINBASE:
		return e.env.Coinbase.Big(), nil

	case TIMESTAMP:
		return math.U256(e.env.Timestamp), nil

	case NUMBER:
		return math.U256(e.env.Number), nil

	case DIFFICULTY:
		return math.U256(e.env.Difficulty), nil

	case GASLIMIT:
		return math.U256(e.env.GasLimit), nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

// do it there but add the helper functions
func (e *EVM) executeUnsignedArithmeticOperations(op OpCode) (*big.Int, error) {
	if !e.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := e.pop(), e.pop()

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

		gas := expByteLen * e.gasTable.ExpByte
		overflow := false

		if gas, overflow = math.SafeAdd(gas, GasSlowStep); overflow {
			return nil, ErrGasOverflow
		}
		if !e.currentContract().consumeGas(gas) {
			return nil, ErrGasConsumed
		}

		return res, nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeSignExtension() (*big.Int, error) {
	back := e.pop()
	if back == nil {
		return nil, ErrStackUnderflow
	}

	if back.Cmp(big.NewInt(31)) < 0 {
		bit := uint(back.Uint64()*8 + 7)
		num := e.pop()
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

func (e *EVM) executeModularOperations(op OpCode) (*big.Int, error) {
	if !e.stackAtLeast(3) {
		return nil, ErrStackUnderflow
	}

	x, y, z := e.pop(), e.pop(), e.pop()

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

func (e *EVM) executeBitWiseOperations1(op OpCode) (*big.Int, error) {
	x := e.pop()
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

func (e *EVM) executeBitWiseOperations2(op OpCode) (*big.Int, error) {
	if !e.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := e.pop(), e.pop()

	switch op {
	case AND:
		return big.NewInt(0).And(x, y), nil

	case OR:
		return big.NewInt(0).Or(x, y), nil

	case XOR:
		return big.NewInt(0).Xor(x, y), nil

	case BYTE:
		if x.Cmp(common.Big32) < 0 {
			return big.NewInt(int64(math.Byte(y, 32, int(x.Int64())))), nil
		}
		return big.NewInt(0), nil

	default:
		return nil, fmt.Errorf("bitwise2 bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeShiftOperations(op OpCode) (*big.Int, error) {
	if !e.config.Constantinople {
		return nil, ErrOpcodeNotFound
	}

	if !e.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := e.pop(), e.pop()

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

func (e *EVM) executeComparison(op OpCode) (*big.Int, error) {
	if !e.stackAtLeast(2) {
		return nil, ErrStackUnderflow
	}

	x, y := e.pop(), e.pop()

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

func (e *EVM) inStaticCall() bool {
	return e.currentContract().static
}

// -- evm ---

func (e *EVM) stackAtLeast(n int) bool {
	return e.currentContract().stackAtLeast(n)
}

func (e *EVM) push(val *big.Int) {
	e.currentContract().push(val)
}

func (e *EVM) pop() *big.Int {
	return e.currentContract().pop()
}

func (e *EVM) peek() *big.Int {
	return e.currentContract().peek()
}

func (e *EVM) peekAt(n int) *big.Int {
	return e.currentContract().peekAt(n)
}

func (e *EVM) swap(n int) {
	e.currentContract().swap(n)
}

func (e *EVM) currentContract() *Contract {
	return e.contracts[e.contractsIndex-1]
}

func (e *EVM) pushContract(c *Contract) {
	e.contracts[e.contractsIndex] = c
	e.contractsIndex++
}

func (e *EVM) popContract() *Contract {
	e.contractsIndex--
	return e.contracts[e.contractsIndex]
}

func CanTransfer(state *state.StateDB, from common.Address, amount *big.Int) bool {
	return state.GetBalance(from).Cmp(amount) >= 0
}

func Transfer(state *state.StateDB, from common.Address, to common.Address, amount *big.Int) error {
	if balance := state.GetBalance(from); balance.Cmp(amount) < 0 {
		return ErrNotEnoughFunds
	}

	x := big.NewInt(amount.Int64())

	state.SubBalance(from, x)
	state.AddBalance(to, x)
	return nil
}

// Memory (based on geth)

type Memory struct {
	c           *Contract
	store       []byte
	lastGasCost uint64
}

func newMemory(c *Contract) *Memory {
	return &Memory{c: c, store: []byte{}, lastGasCost: 0}
}

func (m *Memory) Len() int {
	return len(m.store)
}

func roundUpToWord(n uint64) uint64 {
	return (n + 31) / 32 * 32
}

func numWords(n uint64) uint64 {
	if n > math.MaxUint64-31 {
		return math.MaxUint64/32 + 1
	}

	return (n + 31) / 32
}

func (m *Memory) Resize(size uint64) (uint64, error) {
	fee := uint64(0)

	if uint64(len(m.store)) < size {

		// expand in slots of 32 bytes
		words := numWords(size)
		size = roundUpToWord(size)

		// measure gas
		square := words * words
		linCoef := words * MemoryGas
		quadCoef := square / QuadCoeffDiv
		newTotalFee := linCoef + quadCoef

		fee = newTotalFee - m.lastGasCost

		if fee > m.c.gas {
			return 0, ErrGasConsumed
		}

		m.lastGasCost = newTotalFee

		m.store = append(m.store, make([]byte, size-uint64(len(m.store)))...)
	}

	return fee, nil
}

// calculates the memory size required for a step
func calcMemSize(off, l *big.Int) *big.Int {
	if l.Sign() == 0 {
		return common.Big0
	}

	return new(big.Int).Add(off, l)
}

func (m *Memory) SetByte(o *big.Int, val int64) (uint64, error) {
	offset := o.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, big.NewInt(1)))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}
	m.store[offset] = byte(val & 0xff)
	return gas, nil
}

func (m *Memory) Set32(o *big.Int, val *big.Int) (uint64, error) {
	offset := o.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, big.NewInt(32)))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}

	copy(m.store[offset:offset+32], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	math.ReadBits(val, m.store[offset:offset+32])
	return gas, nil
}

func (m *Memory) Set(o *big.Int, l *big.Int, data []byte) (uint64, error) {
	offset := o.Uint64()
	length := l.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}

	copy(m.store[offset:offset+length], data)
	return gas, nil
}

func (m *Memory) Get(o *big.Int, l *big.Int) ([]byte, uint64, error) {
	offset := o.Uint64()
	length := l.Uint64()

	if length == 0 {
		return []byte{}, 0, nil
	}

	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return nil, 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return nil, 0, err
	}

	cpy := make([]byte, length)
	copy(cpy, m.store[offset:offset+length])
	return cpy, gas, nil
}

func (m *Memory) Resize2(o *big.Int, l *big.Int) (uint64, error) {
	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	return m.Resize(size)
}

func (m *Memory) Show() string {
	str := []string{}
	for i := 0; i < len(m.store); i += 16 {
		j := i + 16
		if j > len(m.store) {
			j = len(m.store)
		}

		str = append(str, hexutil.Encode(m.store[i:j]))
	}
	return strings.Join(str, "\n")
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
