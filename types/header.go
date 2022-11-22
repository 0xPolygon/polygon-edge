package types

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/helper/hex"
)

// Header represents a block header in the Ethereum blockchain.
type Header struct {
	ParentHash   Hash
	Sha3Uncles   Hash
	Miner        []byte
	StateRoot    Hash
	TxRoot       Hash
	ReceiptsRoot Hash
	LogsBloom    Bloom
	Difficulty   uint64
	Number       uint64
	GasLimit     uint64
	GasUsed      uint64
	Timestamp    uint64
	ExtraData    []byte
	MixHash      Hash
	Nonce        Nonce
	Hash         Hash
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

func (h *Header) IsGenesis() bool {
	return h.Hash != ZeroHash && h.Number == 0
}

type Nonce [8]byte

func (n Nonce) String() string {
	return hex.EncodeToHex(n[:])
}

// MarshalText implements encoding.TextMarshaler
func (n Nonce) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (h *Header) Copy() *Header {
	newHeader := &Header{
		ParentHash:   h.ParentHash,
		Sha3Uncles:   h.Sha3Uncles,
		StateRoot:    h.StateRoot,
		TxRoot:       h.TxRoot,
		ReceiptsRoot: h.ReceiptsRoot,
		MixHash:      h.MixHash,
		Hash:         h.Hash,
		LogsBloom:    h.LogsBloom,
		Nonce:        h.Nonce,
		Difficulty:   h.Difficulty,
		Number:       h.Number,
		GasLimit:     h.GasLimit,
		GasUsed:      h.GasUsed,
		Timestamp:    h.Timestamp,
	}

	newHeader.Miner = make([]byte, len(h.Miner))
	copy(newHeader.Miner[:], h.Miner[:])

	newHeader.ExtraData = make([]byte, len(h.ExtraData))
	copy(newHeader.ExtraData[:], h.ExtraData[:])

	return newHeader
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
