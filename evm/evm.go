package evm

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"

	"github.com/ethereum/go-ethereum/common"
)

var (
	gasConsumed       = fmt.Errorf("gas has been consumed")
	gasOverflow       = fmt.Errorf("gas overflow")
	stackOverflow     = fmt.Errorf("stack overflow")
	stackLimit        = fmt.Errorf("stack higher than 1024")
	stackUnderflow    = fmt.Errorf("stack underflow")
	jumpDestNotValid  = fmt.Errorf("jump destination is not valid")
	errMemoryOverflow = fmt.Errorf("error memory overflow")
)

// Gas costs
const (
	GasQuickStep   uint64 = 2
	GasFastestStep uint64 = 3
	GasFastStep    uint64 = 5
	GasMidStep     uint64 = 8
	GasSlowStep    uint64 = 10
	GasExtStep     uint64 = 20

	GasReturn       uint64 = 0
	GasStop         uint64 = 0
	GasContractByte uint64 = 200

	SstoreSetGas    uint64 = 20000 // Once per SLOAD operation.
	SstoreResetGas  uint64 = 5000  // Once per SSTORE operation if the big.NewInt(0)ness changes from big.NewInt(0).
	SstoreClearGas  uint64 = 5000  // Once per SSTORE operation if the big.NewInt(0)ness doesn't change.
	SstoreRefundGas uint64 = 15000 // Once per SSTORE operation if the big.NewInt(0)ness changes to big.NewInt(0).

	NetSstoreNoopGas  uint64 = 200   // Once per SSTORE operation if the value doesn't change.
	NetSstoreInitGas  uint64 = 20000 // Once per SSTORE operation from clean big.NewInt(0).
	NetSstoreCleanGas uint64 = 5000  // Once per SSTORE operation from clean non-big.NewInt(0).
	NetSstoreDirtyGas uint64 = 200   // Once per SSTORE operation from dirty.

	NetSstoreClearRefund      uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot
	NetSstoreResetRefund      uint64 = 4800  // Once per SSTORE operation for resetting to the original non-big.NewInt(0) value
	NetSstoreResetClearRefund uint64 = 19800 // Once per SSTORE operation for resetting to the original big.NewInt(0) value

	MemoryGas    uint64 = 3
	QuadCoeffDiv uint64 = 512
)

var (
	errReadOnly = fmt.Errorf("it is a static call and the state cannot be changed")
)

var (
	tt255 = math.BigPow(2, 255)
)

var (
//big.NewInt(1)  = big.NewInt(1)
//False = big.NewInt(0)

//One  = big.NewInt(1)
//big.NewInt(0) = big.NewInt(0)
)

const StackSize = 2048
const MaxContracts = 1024

// Instructions is the code of instructions
type Instructions []byte

// Env refers to the block information the transactions runs in
// it is shared for all the contracts executed so its in the EVM. maybe call it environment?
type Env struct {
	BlockHash  common.Hash
	Coinbase   common.Address
	Timestamp  *big.Int
	Number     *big.Int
	Difficulty *big.Int
	GasLimit   *big.Int
	GasPrice   *big.Int
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
	value *big.Int // value of the tx
	input []byte   // Input of the tx
	gas   uint64

	// type of contract
	creation bool
	static   bool

	// return data
	returnData []byte
	retOffset  uint64
	retSize    uint64

	bitvec bitvec

	err error
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

// Instructions is the instructions of the contract
func (c *Contract) Instructions() Instructions {
	return c.code
}

// all this actions need to return an error

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

	fmt.Printf("====================> Consume gas: %d (%d)\n", gas, c.gas)

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

func newContract(from common.Address, to common.Address, value *big.Int, gas uint64, code []byte) *Contract {
	f := &Contract{
		ip:      -1,
		code:    code,
		caller:  from,
		origin:  from,
		address: to,
		value:   value,
		stack:   make([]*big.Int, StackSize),
		sp:      0,
		gas:     gas,
		input:   []byte{},
		bitvec:  codeBitmap(code),
	}
	f.memory = newMemory(f)
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

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) common.Hash

// EVM is the ethereum virtual machine
type EVM struct {
	created []common.Address

	contracts      []*Contract
	contractsIndex int

	config   *params.ChainConfig
	gasTable params.GasTable

	state *state.StateDB
	env   *Env

	getHash    GetHashByNumber
	returnData []byte
}

// NewEVM creates a new EVM
func NewEVM(state *state.StateDB, env *Env, config *params.ChainConfig, gasTable params.GasTable, getHash GetHashByNumber) *EVM {
	return &EVM{
		contracts:      make([]*Contract, MaxContracts),
		config:         config,
		gasTable:       gasTable,
		contractsIndex: 0,
		state:          state,
		env:            env,
		getHash:        getHash,
		returnData:     []byte{},
	}
}

func (e *EVM) Call(caller common.Address, to common.Address, input []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {

	// check transfers
	if value != nil {
		if err := Transfer(e.state, caller, to, value); err != nil {
			return []byte{}, 0, err
		}
	}

	contract := newContractCall(caller, to, value, gas, e.state.GetCode(to), input)
	e.pushContract(contract)

	if err := e.Run(); err != nil {
		return nil, 0, err
	}

	c := e.currentContract()

	fmt.Println("-- gas --")
	fmt.Println(c.gas)

	return c.returnData, c.gas, nil
}

func (e *EVM) Create(caller common.Address, code []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	address := crypto.CreateAddress(caller, e.state.GetNonce(caller))

	fmt.Println("-- address of new contract --")
	fmt.Println(address.String())

	nonce := e.state.GetNonce(address)
	e.state.SetNonce(caller, nonce+1)

	// create the new account
	e.state.CreateAccount(address)
	if e.config.IsEIP158(e.env.Number) {
		e.state.SetNonce(address, 1)
	}
	if err := Transfer(e.state, caller, address, value); err != nil {
		return nil, 0, err
	}

	contract := newContractCreation(caller, address, value, gas, code)
	e.pushContract(contract)

	if err := e.Run(); err != nil {
		return nil, 0, err
	}

	c := e.currentContract()
	return c.returnData, c.gas, nil
}

func (e *EVM) calculateGasUsage(op OpCode) uint64 {
	switch op {
	case ADD, SUB, LT, GT, SLT, SGT, EQ, ISZERO, AND, XOR, OR, NOT, BYTE, CALLDATALOAD, SHL, SHR, SAR:
		return GasFastestStep

	case MUL, DIV, SDIV, MOD, SMOD, SIGNEXTEND:
		return GasFastStep

	case ADDMOD, MULMOD, JUMP:
		return GasMidStep

	case JUMPI:
		return GasSlowStep

	case PC, MSIZE, POP, GASLIMIT, DIFFICULTY, NUMBER, TIMESTAMP, COINBASE, GASPRICE, CODESIZE, CALLDATASIZE, CALLVALUE, CALLER, ORIGIN, ADDRESS, RETURNDATASIZE:
		return GasQuickStep

	case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
		return GasFastestStep

	case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
		return GasFastestStep

	case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
		return GasFastestStep

	case BALANCE:
		return e.gasTable.Balance

	case EXTCODESIZE:
		return e.gasTable.ExtcodeSize

	case SLOAD:
		return e.gasTable.SLoad

	case STOP:
		return 0

	default:
		return 1
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
	fmt.Println("##############")

	for {

	BEGIN:
		for e.currentContract().ip < len(e.currentContract().Instructions())-1 {
			e.currentContract().ip++

			ip := e.currentContract().ip
			ins := e.currentContract().Instructions()
			op := OpCode(ins[ip])

			fmt.Println(op.String())
			// fmt.Println(e.currentContract().showStack())

			// consume gas of constant functions
			if gasUsed := e.calculateGasUsage(op); gasUsed != 1 {
				if !e.currentContract().consumeGas(gasUsed) {
					return gasConsumed
				}
			}

			switch op {
			case ADD, MUL, SUB, DIV, SDIV, MOD, SMOD, EXP: // add the other operations
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

			case SIGNEXTEND:
				val, err := e.executeSignExtension()
				if err != nil {
					return err
				}
				if val != nil {
					e.push(val)
				}

			// --- context ---

			case ADDRESS, BALANCE, ORIGIN, CALLER, CALLVALUE, CALLDATALOAD, CALLDATASIZE, CODESIZE, EXTCODESIZE, GASPRICE, RETURNDATASIZE:
				val, err := e.executeContextOperations(op)
				if err != nil {
					return err
				}
				e.push(val)

			// --- context memory copy ---

			case EXTCODECOPY:
				if err := e.executeExtCodeCopy(); err != nil {
					return err
				}

			case CODECOPY, CALLDATACOPY, RETURNDATACOPY:
				if err := e.executeContextCopyOperations(op); err != nil {
					e.currentContract().err = err
					goto END
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

				var data []byte
				if ip+1+n > len(ins) {
					data = common.RightPadBytes(ins[ip+1:len(ins)], n)
				} else {
					data = ins[ip+1 : ip+1+n]
				}

				// fmt.Printf("PUSH: %s\n", hexutil.Encode(data))

				e.push(big.NewInt(0).SetBytes(data))
				e.currentContract().ip += n

			// Duplicate operations

			case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
				n := int(op - DUP)
				if !e.stackAtLeast(n) {
					return stackUnderflow
				}
				e.push(e.peekAt(n))

			// Swap operations

			case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
				n := int(op - SWAP)
				if !e.stackAtLeast(n + 1) {
					return stackUnderflow
				}
				e.swap(n)

			// Logging operations

			case LOG0, LOG1, LOG2, LOG3, LOG4:
				if e.inStaticCall() {
					e.currentContract().err = errReadOnly
					goto END
				}
				if err := e.executeLogsOperation(op); err != nil {
					return err
				}

			// System operations

			case CREATE:

				e.state.PrintObjects()

				if e.inStaticCall() {
					e.currentContract().err = errReadOnly
					goto END
				}

				if err := e.executeCreateOperation(); err != nil {
					return err
				}

			case CALL, CALLCODE, DELEGATECALL, STATICCALL:
				if err := e.executeCallOperations(op); err != nil {
					e.currentContract().err = err
					goto END
				}

			case REVERT, RETURN:
				done, err := e.revert(op)
				if err != nil {
					if err.Error() == "todo revert" {
						goto END
					}
					return err
				}
				if done {
					return nil
				}

			case SELFDESTRUCT:
				if e.inStaticCall() {
					e.currentContract().err = errReadOnly
					goto END
				}
				if ok := e.selfDestruct(); ok {
					return nil
				}

			case STOP:

				goto END
				// break // this maybe breaks the loop and goes to the end??

				/*
					if e.isLastContract() {
						return nil
					}

					e.popContract()

					// push 1 if it worked and 0 if it failed
					e.push(One)
				*/

			// --- sha3 ---

			case SHA3:
				if err := e.sha3(); err != nil {
					return err
				}

			// --- stack ---

			case POP:
				if n := e.pop(); n == nil {
					return stackUnderflow
				}

			// --- memory ---

			case MLOAD:
				offset := e.pop()

				/*
					fmt.Println("MLOAD: ")
					fmt.Println(offset)
					fmt.Println(e.currentContract().memory)
				*/

				data, gas, err := e.currentContract().memory.Get(offset, big.NewInt(32))
				if err != nil {
					return err
				}
				e.push(big.NewInt(1).SetBytes(data))

				gas, overflow := math.SafeAdd(gas, GasFastestStep)
				if overflow {
					return gasOverflow
				}

				if !e.currentContract().consumeGas(gas) {
					return gasConsumed
				}

			case MSTORE:
				// TODO, try to mix mstore8, mstore and mload
				if !e.stackAtLeast(2) {
					return stackOverflow
				}
				start, val := e.pop(), e.pop()

				//fmt.Println("- ### DOING IT ### --")
				//fmt.Println(e.currentContract().memory)

				//fmt.Println(start)
				//fmt.Println(val)

				gas, err := e.currentContract().memory.Set32(start, val)
				if err != nil {
					return err
				}
				gas, overflow := math.SafeAdd(gas, GasFastestStep)
				if overflow {
					return gasOverflow
				}

				if !e.currentContract().consumeGas(gas) {
					return gasConsumed
				}

				//fmt.Println("- after store 2 --")
				//fmt.Println(e.currentContract().memory)

				//fmt.Println("========> DONE ")

			case MSTORE8:
				if !e.stackAtLeast(2) {
					return stackUnderflow
				}
				offset, val := e.pop(), e.pop().Int64()

				/*
					fmt.Println("- m store -")
					fmt.Println(offset)
					fmt.Println(val)
				*/

				gas, err := e.currentContract().memory.SetByte(offset, val)
				if err != nil {
					return err
				}
				gas, overflow := math.SafeAdd(gas, GasFastestStep)
				if overflow {
					return gasOverflow
				}

				if !e.currentContract().consumeGas(gas) {
					return gasConsumed
				}

			// --- storage ---

			case SLOAD:
				loc := e.pop()
				val := e.state.GetState(e.currentContract().address, common.BigToHash(loc))
				e.push(val.Big())

			case SSTORE:
				if err := e.executeSStoreOperation(); err != nil {
					return err
				}

			// --- flow ---

			case JUMP:
				dest := e.pop()
				if dest == nil {
					return stackOverflow
				}

				//fmt.Println(pos)
				//fmt.Println(OpCode(e.currentContract().Instructions()[pos]))

				if !e.currentContract().validJumpdest(dest) {
					e.currentContract().err = jumpDestNotValid
					goto END
				}
				e.currentContract().ip = int(dest.Uint64() - 1)

			case JUMPI:
				if !e.stackAtLeast(2) {
					return stackOverflow
				}
				dest, cond := e.pop(), e.pop()

				if cond.Sign() != 0 {
					if !e.currentContract().validJumpdest(dest) {
						e.currentContract().err = jumpDestNotValid
						goto END
					}
					e.currentContract().ip = int(dest.Uint64() - 1)
				} else {
					// en geth dicen pc++ pero se supone que esto ya hace pc++
				}

			case JUMPDEST:
				// nothing
				// problem because we use the 1 to let him now there are not values
				// if we fix that we can add jumpdest into the constant pool
				if !e.currentContract().consumeGas(1) {
					return gasConsumed
				}

			case PC:
				e.push(big.NewInt(int64(e.currentContract().ip)))

			// dummy
			case MSIZE:
				e.push(e.currentContract().MemoryLen())

			case GAS:

				// consume gas here because we need the consumed gas
				if !e.currentContract().consumeGas(GasQuickStep) {
					return gasConsumed
				}

				// fmt.Printf("GAS: %d\n", e.currentContract().gas)

				e.push(big.NewInt(int64(e.currentContract().gas)))

			default:
				if strings.Contains(op.String(), "Missing") {
					e.currentContract().err = fmt.Errorf("Error code not found")
					goto END
				}
				return fmt.Errorf("opcode not found: %s", op.String())
			}

			if e.currentContract().sp > 1024 {
				return stackLimit
			}
		}

	END:

		fmt.Println("exit the code")
		fmt.Println(e.currentContract().err)

		c := e.currentContract()

		if e.currentContract().err != nil {
			if e.currentContract().err.Error() == "revert" {
				retOffset, retSize := c.retOffset, c.retSize

				fmt.Println("- antes --")
				fmt.Println(e.contractsIndex)

				e.popContract()

				// 1 as a correct return
				e.push(big.NewInt(0))

				fmt.Println("SIZES")
				fmt.Println(retOffset)
				fmt.Println(retSize)

				// set the return data
				if _, err := e.currentContract().memory.Set(big.NewInt(int64(retOffset)), big.NewInt(int64(retSize)), c.returnData); err != nil {
					panic(err)
				}

				// set the consumed gas
				fmt.Println("-- resto del gas del contrato --")
				fmt.Println(c.gas)

				// refund gas
				e.currentContract().gas += c.gas
				e.returnData = c.returnData

				goto BEGIN

				// el problema es que la gent que arriba aci ya li han fet antes tot lo del popcontract
				// entonces esto es soles para el final (em referisc al de baix del contractindex!=1)
				// altres situaciones no arriben aci

			} else {
				// consume all gas?
				e.currentContract().consumeGas(e.currentContract().gas)
			}
		}

		if e.contractsIndex != 1 {
			// this code is over but we still have, this is a return

			// this only if the contract is creation
			if c := e.currentContract(); c.creation {
				e.popContract()

				e.push(c.address.Big())
			} else {
				err := e.currentContract().err

				e.popContract()

				// it is not a revert, coz the revert does not reach here
				if err != nil {
					e.push(big.NewInt(0))
				} else {
					e.push(big.NewInt(1))
				}
			}

		} else {
			break
		}
	}

	fmt.Println("-- gas resto --")
	fmt.Println(e.currentContract().gas)

	return nil
}

func (e *EVM) isLastContract() bool {
	return e.contractsIndex == 1
}

func (e *EVM) executeSStoreOperation() error {
	if e.inStaticCall() {
		return errReadOnly
	}
	if !e.stackAtLeast(2) {
		return stackUnderflow
	}

	fmt.Println(e.currentContract().showStack())

	address := e.currentContract().address

	loc, val := e.pop(), e.pop()

	var gas uint64

	current := e.state.GetState(address, common.BigToHash(loc))

	fmt.Println("-- current --")
	fmt.Println(current)

	fmt.Println(loc)
	fmt.Println(val)

	// discount gas (constantinople)
	if !e.config.IsConstantinople(e.env.Number) {
		switch {
		case current == (common.Hash{}) && val.Sign() != 0: // 0 => non 0
			fmt.Println("X")
			gas = SstoreSetGas
		case current != (common.Hash{}) && val.Sign() == 0: // non 0 => 0
			fmt.Println("Y")
			e.state.AddRefund(SstoreRefundGas)
			gas = SstoreClearGas
		default: // non 0 => non 0 (or 0 => 0)
			fmt.Println("Z")
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
				gas = NetSstoreCleanGas // write existing slot (2.1.2)
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
		return fmt.Errorf("failed to consume gas")
	}

	e.state.SetState(address, common.BigToHash(loc), common.BigToHash(val))

	e.state.PrintObjects()

	// panic("X")
	return nil
}

func (e *EVM) executeLogsOperation(op OpCode) error {
	size := int(op - LOG)
	topics := make([]common.Hash, size)

	if !e.stackAtLeast(2 + size) {
		return stackUnderflow
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
		return gasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, params.LogGas); overflow {
		return gasOverflow
	}
	if gas, overflow = math.SafeAdd(gas, uint64(size)*params.LogTopicGas); overflow {
		return gasOverflow
	}

	var memorySizeGas uint64
	if memorySizeGas, overflow = math.SafeMul(requestedSize, params.LogDataGas); overflow {
		return gasOverflow
	}
	if gas, overflow = math.SafeAdd(gas, memorySizeGas); overflow {
		return gasOverflow
	}

	if !e.currentContract().consumeGas(gas) {
		return gasConsumed
	}
	return nil
}

func (e *EVM) sha3() error {
	fmt.Println("start sha3")

	offset, size := e.pop(), e.pop()

	data, gas, err := e.currentContract().memory.Get(offset, size)
	if err != nil {
		return err
	}
	hash := crypto.Keccak256Hash(data)
	e.push(hash.Big())

	fmt.Println("SHA3")
	fmt.Println(gas)

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, params.Sha3Gas); overflow {
		return gasOverflow
	}

	fmt.Println(gas)
	fmt.Println(size)

	wordGas, overflow := bigUint64(size)
	if overflow {
		return gasOverflow
	}

	fmt.Println(wordGas)
	fmt.Println(numWords(wordGas))

	if wordGas, overflow = math.SafeMul(numWords(wordGas), params.Sha3WordGas); overflow {
		return gasOverflow
	}

	fmt.Println(wordGas)

	if gas, overflow = math.SafeAdd(gas, wordGas); overflow {
		return gasOverflow
	}

	fmt.Println(gas)

	if !e.currentContract().consumeGas(gas) {
		return gasConsumed
	}
	return nil
}

// system operations

// createContract changes the current contract
func (e *EVM) createContract(code []byte, codeHash common.Hash, gas uint64, value *big.Int, address common.Address) error {

	caller := e.currentContract().address

	//fmt.Println("CALLER")
	//fmt.Println(caller.String())

	nonce := e.state.GetNonce(caller)

	//fmt.Println(nonce)

	//fmt.Printf("%s: %d\n", caller.String(), nonce+1)
	e.state.SetNonce(caller, nonce+1)

	//fmt.Printf("CREATE 1: %s\n", e.state.IntermediateRoot(false).String())

	// create the new account
	e.state.CreateAccount(address)
	if e.config.IsEIP158(e.env.Number) {
		e.state.SetNonce(address, 1)
	}

	//fmt.Printf("CREATE 2: %s\n", e.state.IntermediateRoot(false).String())

	if err := Transfer(e.state, caller, address, value); err != nil {
		return err
	}

	//fmt.Printf("CREATE 3: %s\n", e.state.IntermediateRoot(false).String())

	//fmt.Println("code")
	//fmt.Println(code)

	contract := newContractCreation(caller, address, value, gas, code)
	e.pushContract(contract)

	//fmt.Println("-- set code 2 --")
	//fmt.Println(address.String())
	//fmt.Println(code)

	// you have to use the code from ret that we dont have here,
	// actually, these setcode is handled in the return case
	e.state.SetCode(address, []byte{})

	// we should check before if it works or not
	return nil
}

func (e *EVM) executeCreateOperation() error {

	//e.state.PrintObjects()

	value := e.pop()

	//	e.state.PrintObjects()

	offset, size := e.pop(), e.pop()

	//fmt.Println("@@@ PRE @@@")
	//fmt.Println(&e.currentContract().value)
	//fmt.Println(&offset)
	//fmt.Println(&size)

	//fmt.Println(offset)
	//fmt.Println(size)

	//e.state.PrintObjects()

	// not sure how the cast behaves with the overflow check, if we dont do it, there is an error
	// in the address of this specific addres and the balance in one specific example

	input, gas, err := e.currentContract().memory.Get(offset, size)
	if err != nil {
		return err
	}

	// input := []byte{0}
	// gas := uint64(3)

	//fmt.Println("-- post --")
	e.state.PrintObjects()

	//fmt.Println(input)
	//fmt.Printf("Memory gas: %d\n", gas)

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, params.CreateGas); overflow {
		return gasOverflow
	}

	e.state.PrintObjects()

	fmt.Printf("Total gas: %d\n", gas)

	if !e.currentContract().consumeGas(gas) {
		return gasOverflow
	}

	gasLink := e.currentContract().gas

	fmt.Println("-- CREATE PARAMS --")
	fmt.Println(value)
	fmt.Println(offset)
	fmt.Println(size)
	fmt.Printf("Contract gas: %d\n", gasLink)

	fmt.Println(input)

	if e.config.IsEIP150(e.env.Number) {
		gasLink -= gasLink / 64
	}

	fmt.Printf("GasLink: %d\n", gasLink)

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
	input, _, err := e.currentContract().memory.Get(offset, size)
	if err != nil {
		return err
	}
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
	e.returnData = nil

	if op == CALL {
		if val := e.peekAt(2); val != nil && val.BitLen() > 0 {
			return errReadOnly
		}
	}

	fmt.Println(e.currentContract().showStack())

	// pop the gas value
	one := e.pop().Uint64()
	addr := common.BigToAddress(e.pop())

	var value *big.Int
	if op == CALL || op == CALLCODE {
		value = e.pop()
	}

	inOffset, inSize := e.pop(), e.pop()
	retOffset, retSize := e.pop(), e.pop()

	fmt.Println("SIZES")
	fmt.Println(retOffset)
	fmt.Println(retSize)

	args, memoryGas, err := e.currentContract().memory.Get(inOffset, inSize)
	if err != nil {
		return err
	}

	fmt.Println("SIZES AGAIN")
	fmt.Println(retOffset)
	fmt.Println(retSize)

	fmt.Println("-- memory gas --")
	fmt.Println(memoryGas)

	// --- ## GAS ## ---

	gas := e.gasTable.Calls
	eip158 := e.config.IsEIP158(e.env.Number)
	transfersValue := value != nil && value.Sign() != 0

	if op == CALL {
		if eip158 {
			if transfersValue && e.state.Empty(addr) {
				gas += params.CallNewAccountGas
			}
		} else if !e.state.Exist(addr) {
			gas += params.CallNewAccountGas
		}
	}
	if op == CALL || op == CALLCODE {
		if transfersValue {
			gas += params.CallValueTransferGas
		}
	}

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, memoryGas); overflow {
		return gasOverflow
	}

	fmt.Printf("GAS5: %d\n", gas)

	// TODO, add the memory size component, not sure here because we dont resize here
	// or do we?

	fmt.Println("--- PARAMS ---")
	fmt.Println(e.currentContract().gas)
	fmt.Println(gas)
	fmt.Println(one)

	other, err := callGas(e.gasTable, e.currentContract().gas, gas, big.NewInt(int64(one)))
	if err != nil {
		panic(err)
	}

	fmt.Println("-- other --")
	fmt.Println(other)

	if gas, overflow = math.SafeAdd(gas, other); overflow {
		return gasOverflow
	}

	fmt.Println("FINAL GAS")
	fmt.Println(gas)

	if !e.currentContract().consumeGas(gas) {
		return gasConsumed
	}

	// handle value transfer if necessary

	if op == CALL || op == CALLCODE {
		if transfersValue {
			other += params.CallStipend
		}
	}

	fmt.Println("Check if address exists")
	fmt.Println(addr)

	if precompiled, ok := Contracts[addr]; ok {
		if used := precompiled.Gas(args); other >= used {
			// enough gas for the precompiled
		} else {
			// out of gas in precompiled
			// consume the gas
			// e.currentContract().consumeGas(other)
			e.push(big.NewInt(0)) // coz it fails?
			return nil
		}

	} else if !e.state.Exist(addr) {
		fmt.Println("MEC")
		e.push(big.NewInt(1))
		return nil
		// but return the one of success
	}

	fmt.Println("-- gas consumed in the call --")
	fmt.Println(gas)

	fmt.Println("-- gas for the call --")
	fmt.Println(other)

	contract := newContractCall(e.currentContract().address, addr, value, other, e.state.GetCode(addr), args)
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
	ret, gas, err := e.currentContract().memory.Get(offset, size)
	if err != nil {
		return false, err
	}

	fmt.Println(offset)
	fmt.Println(size)

	if !e.currentContract().consumeGas(gas) {
		return false, fmt.Errorf("failed to consume gas")
	}

	if e.isLastContract() {
		e.currentContract().returnData = ret
		return true, nil
	}

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

			fmt.Println("-- set code 1 --")
			fmt.Println(addr.String())

			e.state.SetCode(addr, ret)

			// if contractsIndex == 1 (last return) return big.NewInt(1) and finish
			return e.contractsIndex == 1, nil
		} else {
			retOffset, retSize := e.currentContract().retOffset, e.currentContract().retSize

			// pop last contract
			e.popContract()

			// fmt.Println(e.contracts[e.contractsIndex])

			e.currentContract().returnData = ret

			// It was a success, push one
			e.push(big.NewInt(1))

			if e.contractsIndex == 1 {
				return true, nil
			}

			// NOT sure!
			e.currentContract().memory.Set(big.NewInt(int64(retOffset)), big.NewInt(int64(retSize)), ret)
			return false, nil
		}

		// not sure now about the pointer for this one

	case REVERT:
		fmt.Println("-- return --")
		fmt.Println(ret)

		// I THINK THIS WAY IS SLIGHTLY DIFFERENT
		// IN THE OTHER CASE WE SET THE REUTNR DATA IN THE PARENT

		e.currentContract().returnData = ret
		e.currentContract().err = fmt.Errorf("revert")

		return false, fmt.Errorf("todo revert")
	}

	return true, fmt.Errorf("todo")
}

func (e *EVM) selfDestruct() bool {
	e.state.PrintObjects()

	address := e.pop()

	balance := e.state.GetBalance(e.currentContract().address)
	e.state.AddBalance(common.BigToAddress(address), balance)
	e.state.Suicide(e.currentContract().address)

	// halt
	if e.contractsIndex == 1 {
		return true
	}

	e.popContract()
	e.push(big.NewInt(1))

	return false
}

func bigUint64(v *big.Int) (uint64, bool) {
	return v.Uint64(), v.BitLen() > 64
}

func (e *EVM) executeExtCodeCopy() error {
	if !e.stackAtLeast(4) {
		return stackUnderflow
	}

	address, memOffset, codeOffset, length := e.pop(), e.pop(), e.pop(), e.pop()

	codeCopy := getSlice(e.state.GetCode(common.BigToAddress(address)), codeOffset, length)

	fmt.Println("-- codecopy --")
	fmt.Println(codeCopy)

	gas, err := e.currentContract().memory.Set(memOffset, length, codeCopy)
	if err != nil {
		return err
	}

	// SAME AS DOWN, PROBLEM IS THE NUMBER OF PARAMS; MAYBE WE CAN CHECK THE NUMBER OF PARAMS SEPARATELY
	// THIS IS, IF ITS EXTODECOPY CHECK FOR 4, OTHERWISE CHECK FOR 3 (AND THEN DO THE SWITCH), TODO

	var overflow bool
	if gas, overflow = math.SafeAdd(gas, e.gasTable.ExtcodeCopy); overflow {
		return gasOverflow
	}

	words, overflow := bigUint64(length)
	if overflow {
		return gasOverflow
	}

	if words, overflow = math.SafeMul(numWords(words), params.CopyGas); overflow {
		return gasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, words); overflow {
		return gasOverflow
	}

	fmt.Println("-- cosume gas in code copa")
	fmt.Println(gas)

	if !e.currentContract().consumeGas(gas) {
		return gasConsumed
	}
	return nil
}

// copy values to memory
func (e *EVM) executeContextCopyOperations(op OpCode) error {
	if !e.stackAtLeast(3) {
		return stackUnderflow
	}

	memOffset, dataOffset, length := e.pop(), e.pop(), e.pop()

	var gas uint64
	var err error

	// TODO, simplify even more just returning the []byte for each case in the switch
	switch op {
	case CALLDATACOPY:
		// fmt.Println(getSlice(e.currentContract().input, dataOffset, length))
		gas, err = e.currentContract().memory.Set(memOffset, length, getSlice(e.currentContract().input, dataOffset, length))

	case RETURNDATACOPY:

		fmt.Println("#################")
		fmt.Println(memOffset)
		fmt.Println(dataOffset)
		fmt.Println(length)
		fmt.Println(e.returnData)

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
		return gasOverflow
	}

	words, overflow := bigUint64(length)
	if overflow {
		return gasOverflow
	}

	if words, overflow = math.SafeMul(numWords(words), params.CopyGas); overflow {
		return gasOverflow
	}

	if gas, overflow = math.SafeAdd(gas, words); overflow {
		return gasOverflow
	}

	if !e.currentContract().consumeGas(gas) {
		return gasOverflow
	}
	return nil
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

		//fmt.Println(common.BigToAddress(addr).String())

		return e.state.GetBalance(common.BigToAddress(addr)), nil

	case ORIGIN:
		return e.currentContract().origin.Big(), nil

	case CALLER:
		return e.currentContract().caller.Big(), nil

	case CALLVALUE:
		panic("X")
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
		return e.env.GasPrice, nil

	case RETURNDATASIZE:
		return big.NewInt(int64(len(e.currentContract().returnData))), nil

	default:
		return nil, fmt.Errorf("context bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeBlockInformation(op OpCode) (*big.Int, error) {
	switch op {
	case BLOCKHASH:
		n := e.pop()
		if n == nil {
			return nil, stackUnderflow
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
	if !e.stackAtLeast(2) { // maybe change the semantics to do it without the negation
		return nil, stackOverflow
	}

	x, y := e.pop(), e.pop()

	switch op {
	case ADD:

		fmt.Println("ADD")
		fmt.Println(x)
		fmt.Println(y)

		z := math.U256(big.NewInt(0).Add(x, y))

		fmt.Println(z)

		return z, nil

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

		// do the consume gas
		expByteLen := uint64((exponent.BitLen() + 7) / 8)

		// some shortcuts
		cmpToOne := exponent.Cmp(big.NewInt(1))

		var res *big.Int
		if cmpToOne < 0 { // Exponent is big.NewInt(0)
			res = big.NewInt(1)
		} else if base.Sign() == 0 {
			res = big.NewInt(0)
		} else if cmpToOne == 0 { // Exponent is one
			res = base
		} else {
			res = math.Exp(base, exponent)
		}

		fmt.Println("EXP PARAMS")
		fmt.Println(base)
		fmt.Println(exponent)

		fmt.Println(expByteLen)

		gas := expByteLen * e.gasTable.ExpByte
		overflow := false

		if gas, overflow = math.SafeAdd(gas, GasSlowStep); overflow {
			panic("overflow")
		}

		fmt.Println("GAS USED")
		fmt.Println(gas)

		if !e.currentContract().consumeGas(gas) {
			return nil, gasConsumed
		}

		fmt.Println("-- res --")
		fmt.Println(res)

		return res, nil

	default:
		return nil, fmt.Errorf("arithmetic bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeSignExtension() (*big.Int, error) {
	back := e.pop()

	if back.Cmp(big.NewInt(31)) < 0 {
		bit := uint(back.Uint64()*8 + 7)
		num := e.pop()
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
	x, y, z := e.pop(), e.pop(), e.pop()

	switch op {
	case ADDMOD:
		if z.Cmp(big.NewInt(0)) > 0 {
			x.Add(x, y)
			x.Mod(x, z)
			return math.U256(x), nil
		}
		return big.NewInt(0), nil

	case MULMOD:
		if z.Cmp(big.NewInt(0)) > 0 {
			x.Mul(x, y)
			x.Mod(x, z)
			return math.U256(x), nil
		}
		return big.NewInt(0), nil

	default:
		return nil, fmt.Errorf("modular bad opcode found: %s", op.String())
	}
}

func (e *EVM) executeBitWiseOperations1(op OpCode) (*big.Int, error) {
	x := e.pop()

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
		return big.NewInt(0), nil

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

func Transfer(state *state.StateDB, from common.Address, to common.Address, amount *big.Int) error {
	if balance := state.GetBalance(from); balance.Cmp(amount) < 0 {
		return fmt.Errorf("not enough balance %d for the transfer %d", balance.Uint64(), amount.Uint64())
	}

	x := big.NewInt(amount.Int64())

	//fmt.Println("################################################# --")
	//fmt.Println(&x)

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
	return (n + 31) / 32
}

func (m *Memory) Resize(size uint64) (uint64, error) {
	// fmt.Println("size")
	// fmt.Println(size)

	fee := uint64(0)

	if uint64(len(m.store)) < size {

		// expand in slots of 32 bytes
		words := numWords(size)
		size = roundUpToWord(size)

		// fmt.Printf("Resize: %d, New Size: %d, Words %d\n", len(m.store), size, words)

		// measure gas
		square := words * words
		linCoef := words * MemoryGas
		quadCoef := square / QuadCoeffDiv
		newTotalFee := linCoef + quadCoef

		fee = newTotalFee - m.lastGasCost

		// fmt.Printf("New total fee: %d. Fee: %d\n", newTotalFee, fee)

		if fee > m.c.gas {
			return 0, gasConsumed
		}

		m.lastGasCost = newTotalFee

		m.store = append(m.store, make([]byte, size-uint64(len(m.store)))...)
	}

	return fee, nil
}

func (m *Memory) SetByte(o *big.Int, val int64) (uint64, error) {
	offset := o.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, big.NewInt(1)))
	if overflow {
		return 0, errMemoryOverflow
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
		return 0, errMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}

	// big.NewInt(0) the memory area
	copy(m.store[offset:offset+32], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	// Fill in relevant bits
	math.ReadBits(val, m.store[offset:offset+32])
	return gas, nil
}

func (m *Memory) Set(o *big.Int, l *big.Int, data []byte) (uint64, error) {
	offset := o.Uint64()
	length := l.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return 0, errMemoryOverflow
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

	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return nil, 0, errMemoryOverflow
	}

	fmt.Println(offset)
	fmt.Println(length)

	if length == 0 {
		return []byte{}, 0, nil
	}
	// not sure if we have to increase the gas cost here too.
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
		return 0, errMemoryOverflow
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

func callGas(gasTable params.GasTable, availableGas, base uint64, callCost *big.Int) (uint64, error) {
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
		panic("some error unhandled")
	}

	return callCost.Uint64(), nil
}
