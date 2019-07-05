package types

import (
	"database/sql/driver"
	"fmt"
	"hash"
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

func (h Hash) Value() (driver.Value, error) {
	return h.String(), nil
}

func (h *Hash) Scan(src interface{}) error {
	hh := hex.MustDecodeHex(string(src.([]byte)))
	copy(h[:], hh[:])
	return nil
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

func (a Address) Value() (driver.Value, error) {
	return a.String(), nil
}

func (a *Address) Scan(src interface{}) error {
	aa := hex.MustDecodeHex(string(src.([]byte)))
	copy(a[:], aa[:])
	return nil
}

type HexBytes []byte

func (h HexBytes) String() string {
	return hex.EncodeToHex(h)
}

func (h HexBytes) Bytes() []byte {
	return h[:]
}

func (h HexBytes) Value() (driver.Value, error) {
	return h.String(), nil
}

func (h *HexBytes) Scan(src interface{}) error {
	str, ok := src.(string)
	if !ok {
		str = string(src.([]byte))
	}
	hh := hex.MustDecodeHex(str)
	aux := make([]byte, len(hh))
	copy(aux[:], hh[:])
	*h = aux
	return nil
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
	ParentHash   Hash     `json:"parentHash" db:"parent_hash"`
	Sha3Uncles   Hash     `json:"sha3Uncles" db:"sha3_uncles"`
	Miner        Address  `json:"miner" db:"miner"`
	StateRoot    Hash     `json:"stateRoot" db:"state_root"`
	TxRoot       Hash     `json:"transactionsRoot" db:"transactions_root"`
	ReceiptsRoot Hash     `json:"receiptsRoot" db:"receipts_root"`
	LogsBloom    Bloom    `json:"logsBloom" db:"logs_bloom"`
	Difficulty   uint64   `json:"difficulty" db:"difficulty"`
	Number       uint64   `json:"number" db:"number"`
	GasLimit     uint64   `json:"gasLimit" db:"gas_limit"`
	GasUsed      uint64   `json:"gasUsed" db:"gas_used"`
	Timestamp    uint64   `json:"timestamp" db:"timestamp"`
	ExtraData    HexBytes `json:"extraData" db:"extradata"`
	MixHash      Hash     `json:"mixHash" db:"mixhash"`
	Nonce        Nonce    `json:"nonce" db:"nonce"`

	hash atomic.Value
}

type Nonce [8]byte

func (n Nonce) String() string {
	return hex.EncodeToHex(n[:])
}

func (n Nonce) Value() (driver.Value, error) {
	return n.String(), nil
}

func (n *Nonce) Scan(src interface{}) error {
	nn := hex.MustDecodeHex(string(src.([]byte)))
	copy(n[:], nn[:])
	return nil
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

	vv.Set(arena.NewUint(h.Difficulty))
	vv.Set(arena.NewUint(h.Number))
	vv.Set(arena.NewUint(h.GasLimit))
	vv.Set(arena.NewUint(h.GasUsed))
	vv.Set(arena.NewUint(h.Timestamp))

	vv.Set(arena.NewCopyBytes(h.ExtraData))
	vv.Set(arena.NewBytes(h.MixHash.Bytes()))
	vv.Set(arena.NewCopyBytes(h.Nonce[:]))

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
	Root              HexBytes      `json:"root" db:"root"`
	CumulativeGasUsed uint64        `json:"cumulativeGasUsed" db:"cumulative_gas_used"`
	LogsBloom         Bloom         `json:"logsBloom" db:"bloom"`
	Logs              []*Log        `json:"logs"`
	Status            ReceiptStatus `json:"status" rlp:"-"`
	TxHash            Hash          `json:"transactionHash" rlp:"-" db:"txhash"`
	ContractAddress   Address       `json:"contractAddress" rlp:"-" db:"contract_address"`
	GasUsed           uint64        `json:"gasUsed" rlp:"-" db:"gas_used"`
}

var receiptArenaPool rlpv2.ArenaPool

func (r *Receipt) MarshalLogsWith(a *rlpv2.Arena) *rlpv2.Value {
	if len(r.Logs) == 0 {
		// There are no receipts, write the RLP null array entry
		return a.NewNullArray()
	}

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
	return logs
}

func (r *Receipt) Marshal() []byte {
	a := receiptArenaPool.Get()
	defer receiptArenaPool.Put(a)

	vv := a.NewArray()
	vv.Set(a.NewBytes(r.Root))
	vv.Set(a.NewUint(r.CumulativeGasUsed))
	vv.Set(a.NewBytes(r.LogsBloom[:]))
	vv.Set(r.MarshalLogsWith(a))

	dst := vv.MarshalTo(nil)
	return dst
}

// TODO, remove the rlp tags

type Log struct {
	Address     Address  `json:"address"`
	Topics      []Hash   `json:"topics"`
	Data        HexBytes `json:"data"`
	BlockNumber uint64   `json:"blockNumber" rlp:"-"`
	TxHash      Hash     `json:"transactionHash" rlp:"-"`
	TxIndex     uint     `json:"transactionIndex" rlp:"-"`
	BlockHash   Hash     `json:"blockHash" rlp:"-"`
	LogIndex    uint     `json:"logIndex" rlp:"-"`
	Removed     bool     `json:"removed" rlp:"-"`
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

func (b Bloom) String() string {
	return hex.EncodeToHex(b[:])
}

func (b Bloom) Value() (driver.Value, error) {
	return b.String(), nil
}

func (b *Bloom) Scan(src interface{}) error {
	bb := hex.MustDecodeHex(string(src.([]byte)))
	copy(b[:], bb[:])
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
	Nonce    uint64   `json:"nonce" db:"nonce"`
	GasPrice HexBytes `json:"gasPrice" db:"gas_price"`
	Gas      uint64   `json:"gas" db:"gas"`
	To       *Address `json:"to" rlp:"nil" db:"dst"`
	Value    HexBytes `json:"value" db:"value"`
	Input    HexBytes `json:"input" db:"input"`

	V byte     `json:"v" db:"v"`
	R HexBytes `json:"r" db:"r"`
	S HexBytes `json:"s" db:"s"`

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
	vv.Set(arena.NewCopyBytes(t.GasPrice))
	vv.Set(arena.NewUint(t.Gas))

	// Address may be empty
	if t.To != nil {
		vv.Set(arena.NewBytes((*t.To).Bytes()))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewCopyBytes(t.Value))
	vv.Set(arena.NewCopyBytes(t.Input))

	// signature values
	vv.Set(arena.NewUint(uint64(t.V)))
	vv.Set(arena.NewCopyBytes(t.R))
	vv.Set(arena.NewCopyBytes(t.S))

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

	tt.GasPrice = make([]byte, len(t.GasPrice))
	copy(tt.GasPrice[:], t.GasPrice[:])

	tt.Value = make([]byte, len(t.Value))
	copy(tt.Value[:], t.Value[:])

	tt.R = make([]byte, len(t.R))
	copy(tt.R[:], t.R[:])
	tt.S = make([]byte, len(t.S))
	copy(tt.S[:], t.S[:])

	tt.Input = make([]byte, len(t.Input))
	copy(tt.Input[:], t.Input[:])
	return tt
}
