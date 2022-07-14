package types

import (
	"database/sql/driver"
	"encoding/binary"
	goHex "encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/hex"
)

// Header represents a block header in the Ethereum blockchain.
type Header struct {
	ParentHash   Hash    `json:"parentHash"`
	Sha3Uncles   Hash    `json:"sha3Uncles"`
	Miner        Address `json:"miner"`
	StateRoot    Hash    `json:"stateRoot"`
	TxRoot       Hash    `json:"transactionsRoot"`
	ReceiptsRoot Hash    `json:"receiptsRoot"`
	LogsBloom    Bloom   `json:"logsBloom"`
	Difficulty   uint64  `json:"difficulty"`
	Number       uint64  `json:"number"`
	GasLimit     uint64  `json:"gasLimit"`
	GasUsed      uint64  `json:"gasUsed"`
	Timestamp    uint64  `json:"timestamp"`
	ExtraData    []byte  `json:"extraData"`
	MixHash      Hash    `json:"mixHash"`
	Nonce        Nonce   `json:"nonce"`
	Hash         Hash    `json:"hash"`
}

// HeaderJson represents a block header used for json calls
type HeaderJSON struct {
	ParentHash   Hash     `json:"parentHash"`
	Sha3Uncles   Hash     `json:"sha3Uncles"`
	Miner        Address  `json:"miner"`
	StateRoot    Hash     `json:"stateRoot"`
	TxRoot       Hash     `json:"transactionsRoot"`
	ReceiptsRoot Hash     `json:"receiptsRoot"`
	LogsBloom    Bloom    `json:"logsBloom"`
	Difficulty   *big.Int `json:"difficulty"`
	Number       *big.Int `json:"number"`
	GasLimit     uint64   `json:"gasLimit"`
	GasUsed      uint64   `json:"gasUsed"`
	Timestamp    uint64   `json:"timestamp"`
	ExtraData    []byte   `json:"extraData"`
	MixHash      Hash     `json:"mixHash"`
	Nonce        Nonce    `json:"nonce"`
	Hash         Hash     `json:"hash"`
}

func (h *Header) MarshalJSON() ([]byte, error) {
	var header HeaderJSON

	header.ParentHash = h.ParentHash
	header.Sha3Uncles = h.Sha3Uncles
	header.Miner = h.Miner
	header.StateRoot = h.StateRoot
	header.TxRoot = h.TxRoot
	header.ReceiptsRoot = h.ReceiptsRoot
	header.LogsBloom = h.LogsBloom
	header.GasLimit = h.GasLimit
	header.GasUsed = h.GasUsed
	header.Timestamp = h.Timestamp
	header.ExtraData = h.ExtraData
	header.MixHash = h.MixHash
	header.Nonce = h.Nonce
	header.Hash = h.Hash

	header.Difficulty = new(big.Int).SetUint64(h.Difficulty)
	header.Number = new(big.Int).SetUint64(h.Number)

	return json.Marshal(&header)
}

func (h *Header) UnmarshalJSON(input []byte) error {
	var header HeaderJSON
	if err := json.Unmarshal(input, &header); err != nil {
		return err
	}

	h.ParentHash = header.ParentHash
	h.Sha3Uncles = header.Sha3Uncles
	h.Miner = header.Miner
	h.StateRoot = header.StateRoot
	h.TxRoot = header.TxRoot
	h.ReceiptsRoot = header.ReceiptsRoot
	h.LogsBloom = header.LogsBloom
	h.GasLimit = header.GasLimit
	h.GasUsed = header.GasUsed
	h.Timestamp = header.Timestamp
	h.ExtraData = header.ExtraData
	h.MixHash = header.MixHash
	h.Nonce = header.Nonce
	h.Hash = header.Hash

	h.Difficulty = header.Difficulty.Uint64()
	h.Number = header.Number.Uint64()

	return nil
}

func (h *Header) Equal(hh *Header) bool {
	return h.Hash == hh.Hash
}

func (h *Header) HasBody() bool {
	return h.TxRoot != EmptyRootHash || h.Sha3Uncles != EmptyUncleHash
}

func (h *Header) HasReceipts() bool {
	return h.ReceiptsRoot != EmptyRootHash
}

func (h *Header) SetNonce(i uint64) {
	binary.BigEndian.PutUint64(h.Nonce[:], i)
}

type Nonce [8]byte

func (n Nonce) String() string {
	return hex.EncodeToHex(n[:])
}

func (n Nonce) Value() (driver.Value, error) {
	return n.String(), nil
}

func (n *Nonce) Scan(src interface{}) error {
	stringVal, ok := src.([]byte)
	if !ok {
		return errors.New("invalid type assert")
	}

	nn, decodeErr := hex.DecodeHex(string(stringVal))
	if decodeErr != nil {
		return fmt.Errorf("unable to decode value, %w", decodeErr)
	}

	copy(n[:], nn[:])

	return nil
}

// MarshalText implements encoding.TextMarshaler
func (n Nonce) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (n *Nonce) UnmarshalText(input []byte) error {
	input = input[2:]
	if _, err := goHex.Decode(n[:], input); err != nil {
		return err
	}

	return nil
}

func (h *Header) Copy() *Header {
	hh := new(Header)
	*hh = *h

	hh.ExtraData = make([]byte, len(h.ExtraData))
	copy(hh.ExtraData[:], h.ExtraData[:])

	return hh
}

type Body struct {
	Transactions []*Transaction
	Uncles       []*Header
}

type Block struct {
	Header       *Header
	Transactions []*Transaction
	Uncles       []*Header

	// Cache
	size atomic.Value // *uint64
}

func (b *Block) Hash() Hash {
	return b.Header.Hash
}

func (b *Block) Number() uint64 {
	return b.Header.Number
}

func (b *Block) ParentHash() Hash {
	return b.Header.ParentHash
}

func (b *Block) Body() *Body {
	return &Body{
		Transactions: b.Transactions,
		Uncles:       b.Uncles,
	}
}

func (b *Block) Size() uint64 {
	sizePtr := b.size.Load()
	if sizePtr == nil {
		bytes := b.MarshalRLP()
		size := uint64(len(bytes))
		b.size.Store(&size)

		return size
	}

	sizeVal, ok := sizePtr.(*uint64)
	if !ok {
		return 0
	}

	return *sizeVal
}

func (b *Block) String() string {
	str := fmt.Sprintf(`Block(#%v):`, b.Number())

	return str
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	return &Block{
		Header:       &cpy,
		Transactions: b.Transactions,
		Uncles:       b.Uncles,
	}
}
