package evm

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum/common"
)

var (
	errReadOnly = fmt.Errorf("it is a static call and the state cannot be changed")
)

var (
	tt255 = math.BigPow(2, 255)
)

var (
	True  = big.NewInt(1)
	False = big.NewInt(0)

	One  = big.NewInt(1)
	Zero = big.NewInt(0)
)

const StackSize = 2048
const MaxContracts = 1024

// Instructions is the code of instructions
type Instructions []byte

func (i *Instructions) flatten() []byte {
	j := []byte{}
	for _, y := range *i {
		j = append(j, byte(y))
	}
	return j
}

// BlockInformation refers to the block information the transactions runs in
// it is shared for all the contracts executed so its in the EVM. maybe call it environment?
// TODO, populate this value
type BlockInformation struct {
	BlockHash  common.Hash
	Coinbase   common.Address
	Timestamp  *big.Int
	Number     *big.Int
	Difficulty *big.Int
	GasLimit   *big.Int
}

// Contract is each value from the caller stack
type Contract struct {
	ip   int
	code Instructions

	// memory
	memory *Memory
	stack  []*big.Int
	sp     int

	address common.Address // address of the contract
	origin  common.Address // origin is where the storage is taken from
	caller  common.Address // caller is the one calling the contract

	// inputs
	value    *big.Int // value of the tx
	input    []byte   // Input of the tx
	gasPrice uint64   // gasprice of the tx

	// type of contract
	creation bool
	static   bool

	// return data
	returnData []byte
	retOffset  uint64
	retSize    uint64
}

func (c *Contract) validJumpdest(pos uint64) bool {
	if pos >= uint64(len(c.code)) {
		return false
	}
	return OpCode(c.code[pos]) == JUMPDEST
}

// Instructions is the instructions of the contract
func (c *Contract) Instructions() Instructions {
	return c.code
}

// all this actions need to return an error

func (c *Contract) push(val *big.Int) {
	c.stack[c.sp] = val
	c.sp++
}

func (c *Contract) pop() *big.Int {
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

func newContract(from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	f := &Contract{
		ip:       -1,
		code:     code,
		caller:   from,
		address:  to,
		origin:   to,
		value:    value,
		memory:   newMemory(),
		stack:    make([]*big.Int, StackSize),
		sp:       0,
		gasPrice: gas,
		input:    []byte{},
	}
	return f
}

func newContractCreation(from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	c := newContract(from, to, value, gas, code)
	c.creation = true
	return c
}

func newContractCall(from common.Address, to common.Address, value *big.Int, gas uint64, code []byte, input []byte) *Contract {
	c := newContract(from, to, value, gas, code)
	c.input = input
	return c
}

// EVM is the ethereum virtual machine
type EVM struct {
	created []common.Address

	contracts      []*Contract
	contractsIndex int

	state        *state.StateDB
	blockContext *BlockInformation
}

// NewEVM creates a new EVM
func NewEVM(state *state.StateDB) *EVM {
	return &EVM{
		contracts:      make([]*Contract, MaxContracts),
		contractsIndex: 1,
		state:          state,
	}
}

func (e *EVM) Call(caller common.Address, to common.Address, input []byte, value *big.Int) error {
	contract := newContractCall(caller, to, value, 10000, e.state.GetCode(to), input)
	e.pushContract(contract)
	return e.Run()
}

func (e *EVM) Create(caller common.Address, code []byte, value *big.Int) error {
	address := crypto.CreateAddress(caller, 0)

	nonce := e.state.GetNonce(address)
	e.state.SetNonce(address, nonce+1)

	// create the new account
	e.state.CreateAccount(address)

	contract := newContractCreation(caller, address, value, 10000, code)
	e.pushContract(contract)
	return e.Run()
}

// Run executes the virtual machine
func (e *EVM) Run() error {
	for e.currentContract().ip < len(e.currentContract().Instructions())-1 {
		e.currentContract().ip++

		ip := e.currentContract().ip
		ins := e.currentContract().Instructions()
		op := OpCode(ins[ip])

		switch op {
		case ADD, MUL, SUB, DIV, MOD, SMOD, EXP: // add the other operations
			val, err := e.executeUnsignedArithmeticOperations(op)
			if err != nil {
				return err
			}
			e.push(val)

		case ADDMOD, MULMOD:
			val, err := e.executeModularOperations(op)
			if err != nil {
				return err
			}
			e.push(val)

		case NOT, ISZERO:
			val, err := e.executeBitWiseOperations1(op)
			if err != nil {
				return err
			}
			e.push(val)

		case AND, OR, XOR, BYTE:
			val, err := e.executeBitWiseOperations2(op)
			if err != nil {
				return err
			}
			e.push(val)

		case EQ, GT, LT, SLT, SGT:
			val, err := e.executeComparison(op)
			if err != nil {
				return err
			}
			e.push(val)

		case SHL, SHR, SAR:
			val, err := e.executeShiftOperations(op)
			if err != nil {
				return err
			}
			e.push(val)

		// --- context ---

		case ADDRESS, BALANCE, ORIGIN, CALLER, CALLVALUE, CALLDATALOAD, CALLDATASIZE, CODESIZE, EXTCODESIZE, GASPRICE, RETURNDATASIZE:
			val, err := e.executeContextOperations(op)
			if err != nil {
				return err
			}
			e.push(val)

		// --- context memory copy ---

		case CODECOPY, CALLDATACOPY, EXTCODECOPY, RETURNDATACOPY:
			if err := e.executeContextCopyOperations(op); err != nil {
				return err
			}

		// --- block information ---

		case BLOCKHASH, COINBASE, TIMESTAMP, NUMBER, DIFFICULTY, GASLIMIT:
			val, err := e.executeBlockInformation(op)
			if err != nil {
				return err
			}
			e.push(val)

		// Push operations

		case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
			n := int(op - PUSH) // i.e. PUSH6, n = 6
			data := ins[ip+1 : ip+1+n]
			e.push(big.NewInt(0).SetBytes(data))
			e.currentContract().ip += n

		// Duplicate operations

		case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
			n := int(op - DUP)
			e.push(e.peekAt(n))

		// Swap operations

		case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
			n := int(op - SWAP)
			e.swap(n)

		// Logging operations

		case LOG0, LOG1, LOG2, LOG3, LOG4:
			if e.inStaticCall() {
				return errReadOnly
			}
			// todo

		// System operations

		case CREATE:
			if e.inStaticCall() {
				return errReadOnly
			}

			if err := e.create(); err != nil {
				return err
			}

		case CALL, CALLCODE, DELEGATECALL, STATICCALL:
			if err := e.executeCallOperations(op); err != nil {
				return err
			}

		case REVERT, RETURN:
			done, err := e.revert(op)
			if err != nil {
				return err
			}
			if done {
				return nil
			}

		case SELFDESTRUCT:
			if e.inStaticCall() {
				return errReadOnly
			}

			if err := e.selfDestruct(); err != nil {
				return err
			}

		case STOP:

			e.popContract()

			// push 1 if it worked and 0 if it failed
			e.push(One)

		// --- sha3 ---

		case SHA3:
			// TODO

		// --- stack ---

		case POP:
			e.pop()

		// --- memory ---

		case MLOAD:
			offset := e.pop()
			data := e.currentContract().memory.Get(offset.Int64(), 32)
			e.push(big.NewInt(1).SetBytes(data))

		case MSTORE:
			start, val := e.pop(), e.pop()
			e.currentContract().memory.Set32(start.Uint64(), val)

		case MSTORE8:
			offset, val := e.pop(), e.pop()
			e.currentContract().memory.SetByte(offset.Uint64(), val)

		// --- storage ---

		case SLOAD:
			loc := e.pop()
			val := e.state.GetState(e.currentContract().address, common.BigToHash(loc))
			e.push(val.Big())

		case SSTORE:
			if e.inStaticCall() {
				return errReadOnly
			}
			loc, val := e.pop(), e.pop()
			e.state.SetState(e.currentContract().address, common.BigToHash(loc), common.BigToHash(val))

		// --- flow ---

		case JUMP:
			pos := e.pop().Uint64()
			if !e.currentContract().validJumpdest(pos) {
				panic("not valid") // todo, handle
			}
			e.currentContract().ip = int(pos)

		case JUMPI:
			pos, cond := e.pop().Uint64(), e.pop()
			if cond.Sign() != 0 {
				if !e.currentContract().validJumpdest(pos) {
					panic("not valid") // todo, handle
				}
				e.currentContract().ip = int(pos)
			} else {
				// en geth dicen pc++ pero se supone que esto ya hace pc++
			}

		case JUMPDEST:
			// nothing

		case PC:
			e.push(big.NewInt(int64(e.currentContract().ip)))

		// dummy
		case MSIZE:
			e.push(big.NewInt(10000))

		case GAS:
			e.push(big.NewInt(10000))

		default:
			if strings.Contains(op.String(), "Missing") {
				continue
			}
			return fmt.Errorf("opcode not found: %s", op.String())
		}

	}

	return nil
}

// system operations

// createContract changes the current contract
func (e *EVM) createContract(code []byte, codeHash common.Hash, gas uint64, value *big.Int, address common.Address) error {

	nonce := e.state.GetNonce(e.currentContract().origin)
	e.state.SetNonce(e.currentContract().origin, nonce+1)

	// create the new account
	e.state.CreateAccount(address)

	contract := newContractCreation(e.currentContract().origin, address, value, 10000, code)
	e.pushContract(contract)

	return nil
}

func (e *EVM) create() error {
	value := e.pop()
	offset, size := e.pop(), e.pop()

	input := e.currentContract().memory.Get(offset.Int64(), size.Int64())
	gas := 100 // TODO

	// new address of the contract
	address := crypto.CreateAddress(e.currentContract().address, e.state.GetNonce(e.currentContract().address))

	if err := e.createContract(input, crypto.Keccak256Hash(input), uint64(gas), value, address); err != nil {
		return err
	}

	return nil
}

func (e *EVM) create2() error {
	// TODO, merge with create
	value := e.pop()
	offset, size := e.pop(), e.pop()
	salt := e.pop()
	input := e.currentContract().memory.Get(offset.Int64(), size.Int64())
	gas := 0

	// EIP-150
	gas -= gas / 64

	// address of the new contract
	inputHash := crypto.Keccak256Hash(input)

	// EIPXXX
	address := crypto.CreateAddress2(e.currentContract().address, common.BigToHash(salt), inputHash.Bytes())

	if err := e.createContract(input, inputHash, uint64(gas), value, address); err != nil {
		return err
	}

	return nil
}

func (e *EVM) executeCallOperations(op OpCode) error {

	if op == CALL && e.currentContract().static {
		// check if its a value transfer (some value in the stack is set to true)
	}

	// pop the gas value
	e.pop()

	addr := common.BigToAddress(e.pop())
	value := e.pop()

	inOffset, inSize := e.pop(), e.pop()
	retOffset, retSize := e.pop(), e.pop()

	args := e.currentContract().memory.Get(inOffset.Int64(), inSize.Int64())

	// handle value transfer if necessary

	contract := newContractCall(e.currentContract().address, addr, value, 10000, e.state.GetCode(addr), args)
	contract.retOffset = retOffset.Uint64() // this are only used here
	contract.retSize = retSize.Uint64()

	switch op {
	case STATICCALL:
		contract.static = true
	case DELEGATECALL, CALLCODE:
		contract.origin = e.currentContract().caller // set the storage
		if op == DELEGATECALL {
			contract.caller = e.currentContract().caller
		}
	}

	e.pushContract(contract)
	return nil
}

func (e *EVM) revert(op OpCode) (bool, error) {
	offset, size := e.pop(), e.pop()
	ret := e.currentContract().memory.Get(offset.Int64(), size.Int64())

	switch op {
	case RETURN:

		if e.currentContract().creation {
			addr := e.currentContract().address

			// not sure what to do here!
			contract := e.popContract()
			contract.returnData = ret

			// put on the stack the address of the contract creted
			e.push(addr.Big())

			// save now the contract
			e.created = append(e.created, addr)
			e.state.SetCode(addr, ret)

			// if contractsIndex == 1 (last return) return true and finish
			return e.contractsIndex == 1, nil
		} else {
			retOffset, retSize := e.currentContract().retOffset, e.currentContract().retSize

			// pop last contract
			e.popContract()
			e.currentContract().returnData = ret

			// It was a success, push one
			e.push(One)

			if e.contractsIndex == 1 {
				return true, nil
			}

			if err := e.currentContract().memory.Set(int64(retOffset), int64(retSize), ret); err != nil {
				return false, err
			}

			return false, nil
		}

		// not sure now about the pointer for this one

	case REVERT:
		return false, fmt.Errorf("todo revert")
	}

	return true, fmt.Errorf("todo")
}

func (e *EVM) selfDestruct() error {
	return fmt.Errorf("todo")
}

// copy values to memory
func (e *EVM) executeContextCopyOperations(op OpCode) error {
	memOffset, dataOffset, length := e.pop(), e.pop(), e.pop()

	switch op {
	case CALLDATACOPY:
		return fmt.Errorf("todo")

	case RETURNDATACOPY:
		return fmt.Errorf("todo")

	case CODECOPY:
		return e.currentContract().memory.Set(memOffset.Int64(), length.Int64(), getSlice(e.currentContract().code, dataOffset, length))

	case EXTCODECOPY:
		return fmt.Errorf("todo")

	default:
		return fmt.Errorf("copy bad opcode found: %s", op.String())
	}
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
		return e.state.GetBalance(common.BigToAddress(addr)), nil

	case ORIGIN:
		return e.currentContract().origin.Big(), nil

	case CALLER:
		return e.currentContract().caller.Big(), nil

	case CALLVALUE:
		return e.currentContract().value, nil

	case CALLDATALOAD:
		offset := e.pop()
		return big.NewInt(1).SetBytes(getSlice(e.currentContract().input, offset, big.NewInt(32))), nil

	case CALLDATASIZE:
		return big.NewInt(int64(len(e.currentContract().input))), nil

	case CODESIZE:
		return big.NewInt(int64(len(e.currentContract().code))), nil

	case EXTCODESIZE:
		addr := e.pop()
		return big.NewInt(int64(e.state.GetCodeSize(common.BigToAddress(addr)))), nil

	case GASPRICE:
		return big.NewInt(int64(e.currentContract().gasPrice)), nil

	case RETURNDATASIZE:
		return big.NewInt(int64(len(e.currentContract().returnData))), nil

	default:
		return nil, fmt.Errorf("context bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeBlockInformation(op OpCode) (*big.Int, error) {
	switch op {
	case BLOCKHASH:
		return e.blockContext.BlockHash.Big(), nil

	case COINBASE:
		return e.blockContext.Coinbase.Big(), nil

	case TIMESTAMP:
		return e.blockContext.Timestamp, nil

	case NUMBER:
		return e.blockContext.Number, nil

	case DIFFICULTY:
		return e.blockContext.Difficulty, nil

	case GASLIMIT:
		return e.blockContext.GasLimit, nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

// do it there but add the helper functions
func (e *EVM) executeUnsignedArithmeticOperations(op OpCode) (*big.Int, error) {
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
			return Zero, nil
		}
		return math.U256(big.NewInt(0).Div(x, y)), nil

	case MOD:
		if y.Sign() == 0 {
			return Zero, nil
		}
		return math.U256(big.NewInt(0).Mod(x, y)), nil

	case SMOD:
		if y.Sign() == 0 {
			return Zero, nil
		}
		z := big.NewInt(1).Mod(big.NewInt(0).Abs(x), big.NewInt(0).Abs(y))
		if x.Sign() < 0 {
			return z.Neg(z), nil
		}
		return z, nil

	case EXP:
		base, exponent := x, y
		cmp := base.Cmp(exponent)
		if cmp < 0 {
			return One, nil
		} else if base.Sign() == 0 {
			return Zero, nil
		} else if cmp == 0 {
			return base, nil
		}
		return math.Exp(base, exponent), nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeSignExtension() (*big.Int, error) {
	// TODO
	return nil, nil
}

func (e *EVM) executeModularOperations(op OpCode) (*big.Int, error) {
	x, y, z := e.pop(), e.pop(), e.pop()

	switch op {
	case ADDMOD:
		if z.Cmp(Zero) > 0 {
			x.Add(x, y)
			x.Mod(x, z)
			return math.U256(x), nil
		}
		return Zero, nil

	case MULMOD:
		if z.Cmp(Zero) > 0 {
			x.Mul(x, y)
			x.Mod(x, z)
			return math.U256(x), nil
		}
		return Zero, nil

	default:
		return nil, fmt.Errorf("modular bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeBitWiseOperations1(op OpCode) (*big.Int, error) {
	x := e.pop()

	switch op {
	case ISZERO:
		if x.Sign() > 0 {
			return Zero, nil
		}
		return One, nil

	case NOT:
		return x.Not(x), nil

	default:
		return nil, fmt.Errorf("bitwise1 bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeBitWiseOperations2(op OpCode) (*big.Int, error) {
	x, y := e.pop(), e.pop()

	switch op {
	case AND:
		return x.And(x, y), nil

	case OR:
		return x.Or(x, y), nil

	case XOR:
		return x.Xor(x, y), nil

	case BYTE:
		if x.Cmp(common.Big32) < 0 {
			return big.NewInt(int64(math.Byte(y, 32, int(x.Int64())))), nil
		}
		return Zero, nil

	default:
		return nil, fmt.Errorf("bitwise2 bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeShiftOperations(op OpCode) (*big.Int, error) {
	x, y := e.pop(), e.pop()

	shift := math.U256(x)

	switch op {
	case SHL:
		value := math.U256(y)
		if shift.Cmp(common.Big256) >= 0 {
			return Zero, nil
		}
		return math.U256(value.Lsh(value, uint(shift.Uint64()))), nil

	case SHR:
		value := math.U256(y)
		if shift.Cmp(common.Big256) >= 0 {
			return Zero, nil
		}
		return math.U256(value.Rsh(value, uint(shift.Uint64()))), nil

	case SAR:
		value := math.S256(y)
		if shift.Cmp(common.Big256) >= 0 {
			if value.Sign() >= 0 {
				return math.U256(Zero), nil
			}
			return math.U256(big.NewInt(-1)), nil
		}
		return math.U256(value.Rsh(value, uint(shift.Uint64()))), nil

	default:
		return nil, fmt.Errorf("shift bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeComparison(op OpCode) (*big.Int, error) {
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
		return True, nil
	}
	return False, nil
}

func sltComparison(x, y *big.Int) *big.Int {
	xSign := x.Cmp(tt255)
	ySign := y.Cmp(tt255)

	if xSign >= 0 && ySign < 0 {
		return One
	} else if xSign < 0 && ySign >= 0 {
		return Zero
	}

	if x.Cmp(y) < 0 {
		return One
	}
	return Zero
}

func sgtComparison(x, y *big.Int) *big.Int {
	xSign := x.Cmp(tt255)
	ySign := y.Cmp(tt255)

	if xSign >= 0 && ySign < 0 {
		return Zero
	} else if xSign < 0 && ySign >= 0 {
		return One
	}

	if x.Cmp(y) > 0 {
		return One
	}
	return Zero
}

func (e *EVM) inStaticCall() bool {
	return e.currentContract().static
}

// -- evm ---

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

// Memory (based on geth)

type Memory struct {
	store []byte
}

func newMemory() *Memory {
	return &Memory{store: []byte{}}
}

func (m *Memory) Resize(size uint64) {
	if uint64(len(m.store)) < size {
		m.store = append(m.store, make([]byte, size-uint64(len(m.store)))...)
	}
}

func (m *Memory) SetByte(offset uint64, val *big.Int) {
	copy(m.store[offset:offset+32], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	// Fill in relevant bits
	math.ReadBits(val, m.store[offset:offset+32])
}

func (m *Memory) Set32(offset uint64, val *big.Int) {
	m.Resize(uint64(offset + 32))
	// Zero the memory area
	copy(m.store[offset:offset+32], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	// Fill in relevant bits
	math.ReadBits(val, m.store[offset:offset+32])
}

func (m *Memory) Set(offset int64, length int64, data []byte) error {
	m.Resize(uint64(offset + length))
	copy(m.store[offset:offset+length], data)
	return nil
}

func (m *Memory) Get(offset int64, length int64) []byte {
	cpy := make([]byte, length)
	copy(cpy, m.store[offset:offset+length])
	return cpy
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
