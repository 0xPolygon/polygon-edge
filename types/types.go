package types

import (
	"fmt"
	"hash"
	"math/big"
	"strings"
	"sync/atomic"

	goHex "encoding/hex"

	"github.com/umbracle/minimal/helper/hex"
	"github.com/umbracle/minimal/rlpv2"
	"golang.org/x/crypto/sha3"
)

const (
	HashLength    = 32
	AddressLength = 20
)

type Hash [HashLength]byte

type Address [AddressLength]byte

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

func BytesToHash(b []byte) Hash {
	var h Hash

	size := len(b)
	min := min(size, HashLength)

	copy(h[HashLength-min:], b[len(b)-min:])
	return h
}

func (h Hash) Bytes() []byte {
	return h[:]
}

func (h Hash) String() string {
	return hex.EncodeToHex(h[:])
}

func (a Address) EIP55() string {
	// TODO
	return hex.EncodeToHex(a[:])
}

func (a Address) String() string {
	return a.EIP55()
}

func (a Address) Bytes() []byte {
	return a[:]
}

func StringToHash(str string) Hash {
	return BytesToHash(stringToBytes(str))
}

func StringToAddress(str string) Address {
	return BytesToAddress(stringToBytes(str))
}

func BytesToAddress(b []byte) Address {
	var a Address

	size := len(b)
	min := min(size, AddressLength)

	copy(a[AddressLength-min:], b[len(b)-min:])
	return a
}

func stringToBytes(str string) []byte {
	if strings.HasPrefix(str, "0x") {
		str = str[2:]
	}
	if len(str)%2 == 1 {
		str = "0" + str
	}
	b, _ := hex.DecodeString(str)
	return b
}

// UnmarshalText parses a hash in hex syntax.
func (h *Hash) UnmarshalText(input []byte) error {
	*h = BytesToHash(stringToBytes(string(input)))
	return nil
}

// UnmarshalText parses an address in hex syntax.
func (a *Address) UnmarshalText(input []byte) error {
	buf := stringToBytes(string(input))
	if len(buf) != AddressLength {
		return fmt.Errorf("incorrect length")
	}
	*a = BytesToAddress(buf)
	return nil
}

func (h *Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (a *Address) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

var (
	// EmptyRootHash is the root when there are no transactions
	EmptyRootHash = StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
)

// Header represents a block header in the Ethereum blockchain.
type Header struct {
	ParentHash   Hash     `json:"parentHash"`
	Sha3Uncles   Hash     `json:"sha3Uncles"`
	Miner        Address  `json:"miner"`
	StateRoot    Hash     `json:"stateRoot"`
	TxRoot       Hash     `json:"transactionsRoot"`
	ReceiptsRoot Hash     `json:"receiptsRoot"`
	LogsBloom    Bloom    `json:"logsBloom"`
	Difficulty   *big.Int `json:"difficulty"`
	Number       uint64   `json:"number"`
	GasLimit     uint64   `json:"gasLimit"`
	GasUsed      uint64   `json:"gasUsed"`
	Timestamp    uint64   `json:"timestamp"`
	ExtraData    []byte   `json:"extraData"`
	MixHash      Hash     `json:"mixHash"`
	Nonce        [8]byte  `json:"nonce"`

	hash atomic.Value
}

var headerArenaPool rlpv2.ArenaPool

func (h *Header) MarshalWith(arena *rlpv2.Arena) *rlpv2.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(h.ParentHash.Bytes()))
	vv.Set(arena.NewBytes(h.Sha3Uncles.Bytes()))
	vv.Set(arena.NewBytes(h.Miner.Bytes()))
	vv.Set(arena.NewBytes(h.StateRoot.Bytes()))
	vv.Set(arena.NewBytes(h.TxRoot.Bytes()))
	vv.Set(arena.NewBytes(h.ReceiptsRoot.Bytes()))
	vv.Set(arena.NewBytes(h.LogsBloom[:]))

	vv.Set(arena.NewBigInt(h.Difficulty))

	vv.Set(arena.NewUint(h.Number))
	vv.Set(arena.NewUint(h.GasLimit))
	vv.Set(arena.NewUint(h.GasUsed))
	vv.Set(arena.NewUint(h.Timestamp))

	vv.Set(arena.NewCopyBytes(h.ExtraData))
	vv.Set(arena.NewBytes(h.MixHash.Bytes()))
	vv.Set(arena.NewBytes(h.Nonce[:]))

	return vv
}

func (h *Header) computeHash() Hash {
	arena := headerArenaPool.Get()
	defer headerArenaPool.Put(arena)
	v := h.MarshalWith(arena)

	return BytesToHash(arena.HashTo(nil, v))
}

func (h *Header) Hash() Hash {
	if hash := h.hash.Load(); hash != nil {
		return hash.(Hash)
	}

	v := h.computeHash()
	h.hash.Store(v)
	return v
}

func (h *Header) Copy() *Header {
	hh := new(Header)
	*hh = *h

	if h.Difficulty != nil {
		hh.Difficulty = new(big.Int).Set(h.Difficulty)
	}

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
}

func (b *Block) Hash() Hash {
	return b.Header.Hash()
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

var ReceiptSuccessBytes = []byte{0x01}

type ReceiptStatus uint64

const (
	ReceiptFailed ReceiptStatus = iota
	ReceiptSuccess
)

type Receipt struct {
	Root              []byte        `json:"root"`
	CumulativeGasUsed uint64        `json:"cumulativeGasUsed"`
	LogsBloom         Bloom         `json:"logsBloom"`
	Logs              []*Log        `json:"logs"`
	Status            ReceiptStatus `json:"status" rlp:"-"`
	TxHash            Hash          `json:"transactionHash" rlp:"-"`
	ContractAddress   Address       `json:"contractAddress" rlp:"-"`
	GasUsed           uint64        `json:"gasUsed" rlp:"-"`
}

var receiptArenaPool rlpv2.ArenaPool

func (r *Receipt) Marshal() []byte {
	a := receiptArenaPool.Get()
	defer receiptArenaPool.Put(a)

	vv := a.NewArray()
	vv.Set(a.NewBytes(r.Root))
	vv.Set(a.NewUint(r.CumulativeGasUsed))
	vv.Set(a.NewBytes(r.LogsBloom[:]))

	if len(r.Logs) == 0 {
		// There are no receipts, write the RLP null array entry
		vv.Set(a.NewNullArray())
	} else {
		logs := a.NewArray()
		for _, l := range r.Logs {

			log := a.NewArray()
			log.Set(a.NewBytes(l.Address.Bytes()))

			topics := a.NewArray()
			for _, t := range l.Topics {
				topics.Set(a.NewBytes(t.Bytes()))
			}
			log.Set(topics)
			log.Set(a.NewBytes(l.Data))
			logs.Set(log)
		}
		vv.Set(logs)
	}

	dst := vv.MarshalTo(nil)
	return dst
}

// TODO, remove the rlp tags

type Log struct {
	Address     Address `json:"address"`
	Topics      []Hash  `json:"topics"`
	Data        []byte  `json:"data"`
	BlockNumber uint64  `json:"blockNumber" rlp:"-"`
	TxHash      Hash    `json:"transactionHash" rlp:"-"`
	TxIndex     uint    `json:"transactionIndex" rlp:"-"`
	BlockHash   Hash    `json:"blockHash" rlp:"-"`
	LogIndex    uint    `json:"logIndex" rlp:"-"`
	Removed     bool    `json:"removed" rlp:"-"`
}

const BloomByteLength = 256

type Bloom [BloomByteLength]byte

func (b *Bloom) UnmarshalText(input []byte) error {
	input = input[2:]
	if _, err := goHex.Decode(b[:], input); err != nil {
		return err
	}
	return nil
}

// CreateBloom creates a new bloom filter from a set of receipts
func CreateBloom(receipts []*Receipt) (b Bloom) {
	h := sha3.NewLegacyKeccak256()
	for _, receipt := range receipts {
		for _, log := range receipt.Logs {
			b.setEncode(h, log.Address[:])
			for _, topic := range log.Topics {
				b.setEncode(h, topic[:])
			}
		}
	}
	return
}

func (b *Bloom) setEncode(hasher hash.Hash, h []byte) {
	hasher.Reset()
	hasher.Write(h[:])
	h = hasher.Sum(nil)

	for i := 0; i < 6; i += 2 {
		bit := (uint(h[i+1]) + (uint(h[i]) << 8)) & 2047

		i := 256 - 1 - bit/8
		j := bit % 8
		b[i] = b[i] | (1 << j)
	}
}

type Transaction struct {
	Nonce    uint64   `json:"nonce"`
	GasPrice *big.Int `json:"gasPrice"`
	Gas      uint64   `json:"gas"`
	To       *Address `json:"to" rlp:"nil"`
	Value    *big.Int `json:"value"`
	Input    []byte   `json:"input"`

	V *big.Int `json:"v"`
	R *big.Int `json:"r"`
	S *big.Int `json:"s"`

	hash atomic.Value
	from Address
}

func (t *Transaction) SetFrom(from Address) {
	t.from = from
}

func (t *Transaction) From() Address {
	return t.from
}

var txArenaPool rlpv2.ArenaPool

func (t *Transaction) MarshalWith(arena *rlpv2.Arena) *rlpv2.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(t.Nonce))
	vv.Set(arena.NewBigInt(t.GasPrice))
	vv.Set(arena.NewUint(t.Gas))

	// Address may be empty
	if t.To != nil {
		vv.Set(arena.NewBytes((*t.To).Bytes()))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewBigInt(t.Value))
	vv.Set(arena.NewCopyBytes(t.Input))

	// signature values
	vv.Set(arena.NewBigInt(t.V))
	vv.Set(arena.NewBigInt(t.R))
	vv.Set(arena.NewBigInt(t.S))

	return vv
}

func (t *Transaction) computeHash() Hash {
	arena := txArenaPool.Get()
	defer txArenaPool.Put(arena)

	v := t.MarshalWith(arena)

	buf := arena.HashTo(nil, v)
	return BytesToHash(buf)
}

func (t *Transaction) Hash() Hash {
	if hash := t.hash.Load(); hash != nil {
		return hash.(Hash)
	}

	v := t.computeHash()
	t.hash.Store(v)
	return v
}

func (t *Transaction) Copy() *Transaction {
	tt := new(Transaction)
	*tt = *t

	tt.GasPrice = new(big.Int).Set(t.GasPrice)
	tt.Value = new(big.Int).Set(t.Value)
	tt.V = new(big.Int).Set(t.V)
	tt.R = new(big.Int).Set(t.R)
	tt.S = new(big.Int).Set(t.S)

	tt.Input = make([]byte, len(t.Input))
	copy(tt.Input[:], t.Input[:])
	return tt
}
