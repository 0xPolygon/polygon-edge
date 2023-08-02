package evm

import (
	"fmt"
)

// OpCode is the EVM operation code
type OpCode int

const (
	// STOP halts execution of the contract
	STOP OpCode = 0x0

	// ADD performs (u)int256 addition modulo 2**256
	ADD = 0x01

	// MUL performs (u)int256 multiplication modulo 2**256
	MUL = 0x02

	// SUB performs (u)int256 subtraction modulo 2**256
	SUB = 0x03

	// DIV performs uint256 division
	DIV = 0x04

	// SDIV performs int256 division
	SDIV = 0x05

	// MOD performs uint256 modulus
	MOD = 0x06

	// SMOD performs int256 modulus
	SMOD = 0x07

	// ADDMOD performs (u)int256 addition modulo N
	ADDMOD = 0x08

	// MULMOD performs (u)int256 multiplication modulo N
	MULMOD = 0x09

	// EXP performs uint256 exponentiation modulo 2**256
	EXP = 0x0A

	// SIGNEXTEND performs sign extends x from (b + 1) * 8 bits to 256 bits.
	SIGNEXTEND = 0x0B

	// LT performs int256 comparison
	LT = 0x10

	// GT performs int256 comparison
	GT = 0x11

	// SLT performs int256 comparison
	SLT = 0x12

	// SGT performs int256 comparison
	SGT = 0x13

	// EQ performs (u)int256 equality
	EQ = 0x14

	// ISZERO checks if (u)int256 is zero
	ISZERO = 0x15

	// AND performs 256-bit bitwise and
	AND = 0x16

	// OR performs 256-bit bitwise or
	OR = 0x17

	// XOR performs 256-bit bitwise xor
	XOR = 0x18

	// NOT performs 256-bit bitwise not
	NOT = 0x19

	// BYTE returns the ith byte of (u)int256 x counting from most significant byte
	BYTE = 0x1A

	// SHL performs a shift left
	SHL = 0x1B

	// SHR performs a logical shift right
	SHR = 0x1C

	// SAR performs an arithmetic shift right
	SAR = 0x1D

	// SHA3 performs the keccak256 hash function
	SHA3 = 0x20

	// ADDRESS returns the address of the executing contract
	ADDRESS = 0x30

	// BALANCE returns the address balance in wei
	BALANCE = 0x31

	// ORIGIN returns the transaction origin address
	ORIGIN = 0x32

	// CALLER returns the message caller address
	CALLER = 0x33

	// CALLVALUE returns the message funds in wei
	CALLVALUE = 0x34

	// CALLDATALOAD reads a (u)int256 from message data
	CALLDATALOAD = 0x35

	// CALLDATASIZE returns the message data length in bytes
	CALLDATASIZE = 0x36

	// CALLDATACOPY copies the message data
	CALLDATACOPY = 0x37

	// CODESIZE returns the length of the executing contract's code in bytes
	CODESIZE = 0x38

	// CODECOPY copies the executing contract bytecode
	CODECOPY = 0x39

	// GASPRICE returns the gas price of the executing transaction, in wei per unit of gas
	GASPRICE = 0x3A

	// EXTCODESIZE returns the length of the contract bytecode at addr
	EXTCODESIZE = 0x3B

	// EXTCODECOPY copies the contract bytecode
	EXTCODECOPY = 0x3C

	// RETURNDATASIZE returns the size of the returned data from the last external call in bytes
	RETURNDATASIZE = 0x3D

	// RETURNDATACOPY copies the returned data
	RETURNDATACOPY = 0x3E

	// EXTCODEHASH returns the hash of the specified contract bytecode
	EXTCODEHASH = 0x3F

	// BLOCKHASH returns the hash of the specific block. Only valid for the last 256 most recent blocks
	BLOCKHASH = 0x40

	// COINBASE returns the address of the current block's miner
	COINBASE = 0x41

	// TIMESTAMP returns the current block's Unix timestamp in seconds
	TIMESTAMP = 0x42

	// NUMBER returns the current block's number
	NUMBER = 0x43

	// DIFFICULTY returns the current block's difficulty
	DIFFICULTY = 0x44

	// GASLIMIT returns the current block's gas limit
	GASLIMIT = 0x45

	// CHAINID returns the id of the chain
	CHAINID = 0x46

	// SELFBALANCE returns the balance of the current account
	SELFBALANCE = 0x47

	// POP pops a (u)int256 off the stack and discards it
	POP = 0x50

	// MLOAD reads a (u)int256 from memory
	MLOAD = 0x51

	// MSTORE writes a (u)int256 to memory
	MSTORE = 0x52

	// MSTORE8 writes a uint8 to memory
	MSTORE8 = 0x53

	// SLOAD reads a (u)int256 from storage
	SLOAD = 0x54

	// SSTORE writes a (u)int256 to storage
	SSTORE = 0x55

	// JUMP performs an unconditional jump
	JUMP = 0x56

	// JUMPI performs a conditional jump if condition is truthy
	JUMPI = 0x57

	// PC returns the program counter
	PC = 0x58

	// MSIZE returns the size of memory for this contract execution, in bytes
	MSIZE = 0x59

	// GAS returns the remaining gas
	GAS = 0x5A

	// JUMPDEST corresponds to a possible jump destination
	JUMPDEST = 0x5B

	// PUSH1 pushes a 1-byte value onto the stack
	PUSH1 = 0x60

	// PUSH32 pushes a 32-byte value onto the stack
	PUSH32 = 0x7F

	// DUP1 clones the last value on the stack
	DUP1 = 0x80

	// DUP16 clones the 16th last value on the stack
	DUP16 = 0x8F

	// SWAP1 swaps the last two values on the stack
	SWAP1 = 0x90

	// SWAP16 swaps the top of the stack with the 17th last element
	SWAP16 = 0x9F

	// LOG0 fires an event without topics
	LOG0 = 0xA0

	// LOG1 fires an event with one topic
	LOG1 = 0xA1

	// LOG2 fires an event with two topics
	LOG2 = 0xA2

	// LOG3 fires an event with three topics
	LOG3 = 0xA3

	// LOG4 fires an event with four topics
	LOG4 = 0xA4

	// CREATE creates a child contract
	CREATE = 0xF0

	// CALL calls a method in another contract
	CALL = 0xF1

	// CALLCODE calls a method in another contract
	CALLCODE = 0xF2

	// RETURN returns from this contract call
	RETURN = 0xF3

	// DELEGATECALL calls a method in another contract using the storage of the current contract
	DELEGATECALL = 0xF4

	// CREATE2 creates a child contract with a salt
	CREATE2 = 0xF5

	// STATICCALL calls a method in another contract
	STATICCALL = 0xFA

	// REVERT reverts with return data
	REVERT = 0xFD

	// SELFDESTRUCT destroys the contract and sends all funds to addr
	SELFDESTRUCT = 0xFF
)

var opCodeToString = map[OpCode]string{
	STOP:           "STOP",
	ADD:            "ADD",
	MUL:            "MUL",
	SUB:            "SUB",
	DIV:            "DIV",
	SDIV:           "SDIV",
	MOD:            "MOD",
	SMOD:           "SMOD",
	EXP:            "EXP",
	NOT:            "NOT",
	LT:             "LT",
	GT:             "GT",
	SLT:            "SLT",
	SGT:            "SGT",
	EQ:             "EQ",
	ISZERO:         "ISZERO",
	SIGNEXTEND:     "SIGNEXTEND",
	AND:            "AND",
	OR:             "OR",
	XOR:            "XOR",
	BYTE:           "BYTE",
	SHL:            "SHL",
	SHR:            "SHR",
	SAR:            "SAR",
	ADDMOD:         "ADDMOD",
	MULMOD:         "MULMOD",
	SHA3:           "SHA3",
	ADDRESS:        "ADDRESS",
	BALANCE:        "BALANCE",
	ORIGIN:         "ORIGIN",
	CALLER:         "CALLER",
	CALLVALUE:      "CALLVALUE",
	CALLDATALOAD:   "CALLDATALOAD",
	CALLDATASIZE:   "CALLDATASIZE",
	CALLDATACOPY:   "CALLDATACOPY",
	CODESIZE:       "CODESIZE",
	CODECOPY:       "CODECOPY",
	GASPRICE:       "GASPRICE",
	EXTCODESIZE:    "EXTCODESIZE",
	EXTCODECOPY:    "EXTCODECOPY",
	RETURNDATASIZE: "RETURNDATASIZE",
	RETURNDATACOPY: "RETURNDATACOPY",
	EXTCODEHASH:    "EXTCODEHASH",
	BLOCKHASH:      "BLOCKHASH",
	COINBASE:       "COINBASE",
	TIMESTAMP:      "TIMESTAMP",
	NUMBER:         "NUMBER",
	DIFFICULTY:     "DIFFICULTY",
	GASLIMIT:       "GASLIMIT",
	POP:            "POP",
	MLOAD:          "MLOAD",
	MSTORE:         "MSTORE",
	MSTORE8:        "MSTORE8",
	SLOAD:          "SLOAD",
	SSTORE:         "SSTORE",
	JUMP:           "JUMP",
	JUMPI:          "JUMPI",
	PC:             "PC",
	MSIZE:          "MSIZE",
	GAS:            "GAS",
	JUMPDEST:       "JUMPDEST",
	CREATE:         "CREATE",
	CALL:           "CALL",
	RETURN:         "RETURN",
	CALLCODE:       "CALLCODE",
	DELEGATECALL:   "DELEGATECALL",
	CREATE2:        "CREATE2",
	STATICCALL:     "STATICCALL",
	REVERT:         "REVERT",
	SELFDESTRUCT:   "SELFDESTRUCT",
	CHAINID:        "CHAINID",
	SELFBALANCE:    "SELFBALANCE",
}

func opCodesToString(from, to OpCode, str string) {
	c := 1
	if from == LOG0 {
		c = 0
	}

	for i := from; i <= to; i++ {
		opCodeToString[i] = fmt.Sprintf("%s%d", str, c)
		c++
	}
}

func init() {
	// write push
	opCodesToString(PUSH1, PUSH32, "PUSH")
	// write dup
	opCodesToString(DUP1, DUP16, "DUP")
	// write log
	opCodesToString(LOG0, LOG4, "LOG")
	// write swap
	opCodesToString(SWAP1, SWAP16, "SWAP")
}

func (op OpCode) String() string {
	return opCodeToString[op]
}
