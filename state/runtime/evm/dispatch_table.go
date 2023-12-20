package evm

import "fmt"

type handler struct {
	inst  instruction
	stack int
	gas   uint64
}

var dispatchTable [256]handler

func register(op OpCode, h handler) {
	if dispatchTable[op].inst != nil {
		panic(fmt.Errorf("instruction already exists")) //nolint:gocritic
	}

	dispatchTable[op] = h
}

func registerRange(from, to OpCode, factory func(n int) instruction, gas uint64) {
	c := 1
	for i := from; i <= to; i++ {
		register(i, handler{factory(c), 0, gas})
		c++
	}
}

func init() {
	// unsigned arithmetic operations
	register(STOP, handler{inst: opStop, stack: 0, gas: 0})
	register(ADD, handler{inst: opAdd, stack: 2, gas: 3})
	register(SUB, handler{inst: opSub, stack: 2, gas: 3})
	register(MUL, handler{inst: opMul, stack: 2, gas: 5})
	register(DIV, handler{inst: opDiv, stack: 2, gas: 5})
	register(SDIV, handler{inst: opSDiv, stack: 2, gas: 5})
	register(MOD, handler{inst: opMod, stack: 2, gas: 5})
	register(SMOD, handler{inst: opSMod, stack: 2, gas: 5})
	register(EXP, handler{inst: opExp, stack: 2, gas: 10})

	register(PUSH0, handler{inst: opPush0, stack: 0, gas: 2})
	registerRange(PUSH1, PUSH32, opPush, 3)
	registerRange(DUP1, DUP16, opDup, 3)
	registerRange(SWAP1, SWAP16, opSwap, 3)
	registerRange(LOG0, LOG4, opLog, 375)

	register(ADDMOD, handler{inst: opAddMod, stack: 3, gas: 8})
	register(MULMOD, handler{inst: opMulMod, stack: 3, gas: 8})

	register(AND, handler{inst: opAnd, stack: 2, gas: 3})
	register(OR, handler{inst: opOr, stack: 2, gas: 3})
	register(XOR, handler{inst: opXor, stack: 2, gas: 3})
	register(BYTE, handler{inst: opByte, stack: 2, gas: 3})

	register(NOT, handler{inst: opNot, stack: 1, gas: 3})
	register(ISZERO, handler{inst: opIsZero, stack: 1, gas: 3})

	register(EQ, handler{inst: opEq, stack: 2, gas: 3})
	register(LT, handler{inst: opLt, stack: 2, gas: 3})
	register(GT, handler{inst: opGt, stack: 2, gas: 3})
	register(SLT, handler{inst: opSlt, stack: 2, gas: 3})
	register(SGT, handler{inst: opSgt, stack: 2, gas: 3})

	register(SIGNEXTEND, handler{inst: opSignExtension, stack: 1, gas: 5})

	register(SHL, handler{inst: opShl, stack: 2, gas: 3})
	register(SHR, handler{inst: opShr, stack: 2, gas: 3})
	register(SAR, handler{inst: opSar, stack: 2, gas: 3})

	register(CREATE, handler{inst: opCreate(CREATE), stack: 3, gas: 32000})
	register(CREATE2, handler{inst: opCreate(CREATE2), stack: 4, gas: 32000})

	register(CALL, handler{inst: opCall(CALL), stack: 7, gas: 0})
	register(CALLCODE, handler{inst: opCall(CALLCODE), stack: 7, gas: 0})
	register(DELEGATECALL, handler{inst: opCall(DELEGATECALL), stack: 6, gas: 0})
	register(STATICCALL, handler{inst: opCall(STATICCALL), stack: 6, gas: 0})

	register(REVERT, handler{inst: opHalt(REVERT), stack: 2, gas: 0})
	register(RETURN, handler{inst: opHalt(RETURN), stack: 2, gas: 0})

	// memory
	register(MLOAD, handler{inst: opMload, stack: 1, gas: 3})
	register(MSTORE, handler{inst: opMStore, stack: 2, gas: 3})
	register(MSTORE8, handler{inst: opMStore8, stack: 2, gas: 3})

	// store
	register(SLOAD, handler{inst: opSload, stack: 1, gas: 0})
	register(SSTORE, handler{inst: opSStore, stack: 2, gas: 0})

	register(SHA3, handler{inst: opSha3, stack: 2, gas: 30})

	register(POP, handler{inst: opPop, stack: 1, gas: 2})

	register(EXTCODEHASH, handler{inst: opExtCodeHash, stack: 1, gas: 0})

	// context operations
	register(ADDRESS, handler{inst: opAddress, stack: 0, gas: 2})
	register(BALANCE, handler{inst: opBalance, stack: 1, gas: 0})
	register(SELFBALANCE, handler{inst: opSelfBalance, stack: 0, gas: 5})
	register(ORIGIN, handler{inst: opOrigin, stack: 0, gas: 2})
	register(CALLER, handler{inst: opCaller, stack: 0, gas: 2})
	register(CALLVALUE, handler{inst: opCallValue, stack: 0, gas: 2})
	register(CALLDATALOAD, handler{inst: opCallDataLoad, stack: 1, gas: 3})
	register(CALLDATASIZE, handler{inst: opCallDataSize, stack: 0, gas: 2})
	register(CODESIZE, handler{inst: opCodeSize, stack: 0, gas: 2})
	register(EXTCODESIZE, handler{inst: opExtCodeSize, stack: 1, gas: 0})
	register(GASPRICE, handler{inst: opGasPrice, stack: 0, gas: 2})
	register(RETURNDATASIZE, handler{inst: opReturnDataSize, stack: 0, gas: 2})
	register(CHAINID, handler{inst: opChainID, stack: 0, gas: 2})
	register(PC, handler{inst: opPC, stack: 0, gas: 2})
	register(MSIZE, handler{inst: opMSize, stack: 0, gas: 2})
	register(GAS, handler{inst: opGas, stack: 0, gas: 2})

	register(EXTCODECOPY, handler{inst: opExtCodeCopy, stack: 4, gas: 0})

	register(CALLDATACOPY, handler{inst: opCallDataCopy, stack: 3, gas: 3})
	register(RETURNDATACOPY, handler{inst: opReturnDataCopy, stack: 3, gas: 3})
	register(CODECOPY, handler{inst: opCodeCopy, stack: 3, gas: 3})

	// block information
	register(BLOCKHASH, handler{inst: opBlockHash, stack: 1, gas: 20})
	register(COINBASE, handler{inst: opCoinbase, stack: 0, gas: 2})
	register(TIMESTAMP, handler{inst: opTimestamp, stack: 0, gas: 2})
	register(NUMBER, handler{inst: opNumber, stack: 0, gas: 2})
	register(DIFFICULTY, handler{inst: opDifficulty, stack: 0, gas: 2})
	register(GASLIMIT, handler{inst: opGasLimit, stack: 0, gas: 2})
	register(BASEFEE, handler{inst: opBaseFee, stack: 0, gas: 2})

	register(SELFDESTRUCT, handler{inst: opSelfDestruct, stack: 1, gas: 0})

	// jumps
	register(JUMP, handler{inst: opJump, stack: 1, gas: 8})
	register(JUMPI, handler{inst: opJumpi, stack: 2, gas: 10})
	register(JUMPDEST, handler{inst: opJumpDest, stack: 0, gas: 1})
}
