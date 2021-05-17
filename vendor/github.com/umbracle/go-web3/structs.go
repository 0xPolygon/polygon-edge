package web3

import (
	"encoding/hex"
	"fmt"
	"math/big"
)

// Address is an Ethereum address
type Address [20]byte

// HexToAddress converts an hex string value to an address object
func HexToAddress(str string) Address {
	a := Address{}
	a.UnmarshalText([]byte(str))
	return a
}

// UnmarshalText implements the unmarshal interface
func (a *Address) UnmarshalText(b []byte) error {
	return unmarshalTextByte(a[:], b, 20)
}

// MarshalText implements the marshal interface
func (a Address) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a Address) String() string {
	return "0x" + hex.EncodeToString(a[:])
}

// Hash is an Ethereum hash
type Hash [32]byte

// HexToHash converts an hex string value to a hash object
func HexToHash(str string) Hash {
	h := Hash{}
	h.UnmarshalText([]byte(str))
	return h
}

// UnmarshalText implements the unmarshal interface
func (h *Hash) UnmarshalText(b []byte) error {
	return unmarshalTextByte(h[:], b, 32)
}

// MarshalText implements the marshal interface
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (h Hash) String() string {
	return "0x" + hex.EncodeToString(h[:])
}

type Block struct {
	Number             uint64
	Hash               Hash
	ParentHash         Hash
	Sha3Uncles         Hash
	TransactionsRoot   Hash
	StateRoot          Hash
	ReceiptsRoot       Hash
	Miner              Address
	Difficulty         *big.Int
	ExtraData          []byte
	GasLimit           uint64
	GasUsed            uint64
	Timestamp          uint64
	Transactions       []*Transaction
	TransactionsHashes []Hash
	Uncles             []Hash
}

type Transaction struct {
	Hash        Hash
	From        Address
	To          *Address
	Input       []byte
	GasPrice    uint64
	Gas         uint64
	Value       *big.Int
	Nonce       uint64
	V           []byte
	R           []byte
	S           []byte
	BlockHash   Hash
	BlockNumber uint64
	TxnIndex    uint64
}

type CallMsg struct {
	From     Address
	To       Address
	Data     []byte
	GasPrice uint64
	Value    *big.Int
}

type LogFilter struct {
	Address   []Address
	Topics    []*Hash
	BlockHash *Hash
	From      *BlockNumber
	To        *BlockNumber
}

func (l *LogFilter) SetFromUint64(num uint64) {
	b := BlockNumber(num)
	l.From = &b
}

func (l *LogFilter) SetToUint64(num uint64) {
	b := BlockNumber(num)
	l.To = &b
}

func (l *LogFilter) SetTo(b BlockNumber) {
	l.To = &b
}

type Receipt struct {
	TransactionHash   Hash
	TransactionIndex  uint64
	ContractAddress   Address
	BlockHash         Hash
	From              Address
	BlockNumber       uint64
	GasUsed           uint64
	CumulativeGasUsed uint64
	LogsBloom         []byte
	Logs              []*Log
}

type Log struct {
	Removed          bool
	LogIndex         uint64
	TransactionIndex uint64
	TransactionHash  Hash
	BlockHash        Hash
	BlockNumber      uint64
	Address          Address
	Topics           []Hash
	Data             []byte
}

type BlockNumber int

const (
	Latest   BlockNumber = -1
	Earliest             = -2
	Pending              = -3
)

func (b BlockNumber) String() string {
	switch b {
	case Latest:
		return "latest"
	case Earliest:
		return "earliest"
	case Pending:
		return "pending"
	}
	if b < 0 {
		panic("internal. blocknumber is negative")
	}
	return fmt.Sprintf("0x%x", uint64(b))
}

func EncodeBlock(block ...BlockNumber) BlockNumber {
	if len(block) != 1 {
		return Latest
	}
	return block[0]
}
