package types

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
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
	Hash         Hash
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
	nn := hex.MustDecodeHex(string(src.([]byte)))
	copy(n[:], nn[:])
	return nil
}

var marshalArenaPool fastrlp.ArenaPool

func (h *Header) CalculateHash() (hash Hash) {
	// If the mix digest is equivalent to the predefined Istanbul digest, use Istanbul
	// specific hash calculation.
	if h.MixHash == IstanbulDigest {
		// Seal is reserved in extra-data. To prove block is signed by the proposer.
		if istanbulHeader := IstanbulFilteredHeader(h, true); istanbulHeader != nil {
			return rlpHash(istanbulHeader)
		}
	}
	return rlpHash(h)
}

// ComputeHash computes the hash of the header
func (h *Header) ComputeHash() *Header {
	// If the mix digest is equivalent to the predefined Istanbul digest, use Istanbul
	// specific hash calculation.
	if h.MixHash == IstanbulDigest {
		// Seal is reserved in extra-data. To prove block is signed by the proposer.
		if istanbulHeader := IstanbulFilteredHeader(h, true); istanbulHeader != nil {
			h.Hash = rlpHash(istanbulHeader)
			return h
		}
	}

	ar := marshalArenaPool.Get()
	hash := keccak.DefaultKeccakPool.Get()

	v := h.MarshalWith(ar)
	hash.WriteRlp(h.Hash[:0], v)

	marshalArenaPool.Put(ar)
	keccak.DefaultKeccakPool.Put(hash)
	return h
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

func (b *Body) UnmarshalRLP(p *fastrlp.Parser, v *fastrlp.Value) error {
	tuple, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("Two elements expected")
	}

	// transactions
	txns, err := tuple[0].GetElems()
	if err != nil {
		return err
	}
	for _, txn := range txns {
		bTxn := &Transaction{}
		if err := bTxn.UnmarshalRLP(p, txn); err != nil {
			return err
		}
		b.Transactions = append(b.Transactions, bTxn)
	}

	// uncles
	uncles, err := tuple[1].GetElems()
	if err != nil {
		return err
	}
	for _, uncle := range uncles {
		bUncle := &Header{}
		if err := bUncle.UnmarshalRLP(p, uncle); err != nil {
			return err
		}
		b.Uncles = append(b.Uncles, bUncle)
	}

	return nil
}

func (b *Body) MarshalWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	if len(b.Transactions) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v0 := ar.NewArray()
		for _, tx := range b.Transactions {
			v0.Set(tx.MarshalWith(ar))
		}
		vv.Set(v0)
	}

	if len(b.Uncles) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v1 := ar.NewArray()
		for _, uncle := range b.Uncles {
			v1.Set(uncle.MarshalWith(ar))
		}
		vv.Set(v1)
	}

	return vv
}

type Block struct {
	Header       *Header
	Transactions []*Transaction
	Uncles       []*Header
}

func (b *Block) UnmarshalRLP(buf []byte) error {
	p := &fastrlp.Parser{}
	v, err := p.Parse(buf)
	if err != nil {
		return err
	}

	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	// header
	b.Header = &Header{}
	if err := b.Header.UnmarshalRLP(p, elems[0]); err != nil {
		return err
	}

	// transactions
	txns, err := elems[1].GetElems()
	if err != nil {
		return err
	}
	for _, txn := range txns {
		bTxn := &Transaction{}
		if err := bTxn.UnmarshalRLP(p, txn); err != nil {
			return err
		}
		b.Transactions = append(b.Transactions, bTxn)
	}

	// uncles
	uncles, err := elems[2].GetElems()
	if err != nil {
		return err
	}
	for _, uncle := range uncles {
		bUncle := &Header{}
		if err := bUncle.UnmarshalRLP(p, uncle); err != nil {
			return err
		}
		b.Uncles = append(b.Uncles, bUncle)
	}

	return nil
}

func (b *Block) MarshalWith(ar *fastrlp.Arena) *fastrlp.Value {
	// merge with the other

	vv := ar.NewArray()
	vv.Set(b.Header.MarshalWith(ar))

	if len(b.Transactions) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v0 := ar.NewArray()
		for _, tx := range b.Transactions {
			v0.Set(tx.MarshalWith(ar))
		}
		vv.Set(v0)
	}

	if len(b.Uncles) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v1 := ar.NewArray()
		for _, uncle := range b.Uncles {
			v1.Set(uncle.MarshalWith(ar))
		}
		vv.Set(v1)
	}

	return vv

}

func (b *Block) Hash() Hash {
	return b.Header.CalculateHash()
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

func CalcUncleHash(uncles []*Header) Hash {
	if len(uncles) == 0 {
		return EmptyUncleHash
	}
	return rlpHash(uncles)
}

func rlpHash(x interface{}) (h Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
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
