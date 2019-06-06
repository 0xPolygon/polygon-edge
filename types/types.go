package types

import (
	"fmt"
	"hash"
	"math/big"
	"strings"
	"sync/atomic"

	goHex "encoding/hex"

	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/helper/hex"
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

func (h *Header) Hash() Hash {
	if hash := h.hash.Load(); hash != nil {
		return hash.(Hash)
	}
	v := rlpHash(h)
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

func rlpHash(x interface{}) (h Hash) {
	hw := sha3.NewLegacyKeccak256()
	err := rlp.Encode(hw, x)
	if err != nil {
		panic(err)
	}
	hw.Sum(h[:0])
	return h
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

var receiptSuccessBytes = []byte{0x01}

type ReceiptStatus uint64

const (
	ReceiptFailed ReceiptStatus = iota
	ReceiptSuccess
)

type Receipt struct {
	Root              []byte        `json:"root"`
	Status            ReceiptStatus `json:"status"`
	CumulativeGasUsed uint64        `json:"cumulativeGasUsed"`
	LogsBloom         Bloom         `json:"logsBloom"`
	Logs              []*Log        `json:"logs"`
	TxHash            Hash          `json:"transactionHash"`
	ContractAddress   Address       `json:"contractAddress"`
	GasUsed           uint64        `json:"gasUsed"`
}

// ConsensusEncode encodes the receipt given the consensus rules
func (r *Receipt) ConsensusEncode() ([]byte, error) {
	logs := []interface{}{}
	for _, log := range r.Logs {
		logs = append(logs, []interface{}{
			log.Address,
			log.Topics,
			log.Data,
		})
	}

	var root []byte
	if r.Root == nil {
		if r.Status == ReceiptSuccess {
			root = receiptSuccessBytes
		}
	} else {
		root = r.Root
	}

	obj := []interface{}{
		root,
		r.CumulativeGasUsed,
		r.LogsBloom,
		logs,
	}

	return rlp.EncodeToBytes(obj)
}

type Log struct {
	Address     Address `json:"address"`
	Topics      []Hash  `json:"topics"`
	Data        []byte  `json:"data"`
	BlockNumber uint64  `json:"blockNumber"`
	TxHash      Hash    `json:"transactionHash"`
	TxIndex     uint    `json:"transactionIndex"`
	BlockHash   Hash    `json:"blockHash"`
	LogIndex    uint    `json:"logIndex"`
	Removed     bool    `json:"removed"`
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

func (tx *Transaction) Hash() Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(Hash)
	}
	v := rlpHash(tx)
	tx.hash.Store(v)
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
