package service

import (
	"crypto/ecdsa"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var marshalArenaPool fastrlp.ArenaPool

// string mapping
const (
	StatusPending   = "pending"   // The AA transaction is on the Pool
	StatusQueued    = "queued"    // The AA transaction is waiting to be mined.
	StatusCompleted = "completed" // The `AA transaction` was mined in a block.
	StatusFailed    = "failed"    // AA transaction` failed during the process.
)

// AATransaction represents an AA transaction
type AATransaction struct {
	Signature   []byte      `json:"signature"`
	Transaction Transaction `json:"transaction"`
}

func (t *AATransaction) IsFromValid() bool {
	return t.Transaction.IsFromValid(t.Signature)
}

func (t *AATransaction) MakeSignature(pk *ecdsa.PrivateKey) error {
	hash := t.Transaction.ComputeHash()

	sig, err := crypto.Sign(pk, hash[:])
	if err != nil {
		return err
	}

	t.Signature = sig

	return nil
}

// Transaction represents a transaction
type Transaction struct {
	From    types.Address `json:"from"`
	Nonce   uint64        `json:"nonce"`
	Payload []Payload     `json:"payload"`
}

func (t *Transaction) UpdateFrom(pk *ecdsa.PrivateKey) {
	t.From = crypto.PubKeyToAddress(&pk.PublicKey)
}

func (t *Transaction) IsFromValid(signature []byte) bool {
	hash := t.ComputeHash() // recompute hash

	pubKey, err := crypto.Ecrecover(hash[:], signature)
	if err != nil {
		return false
	}

	return t.From == types.BytesToAddress(crypto.Keccak256(pubKey[1:])[12:])
}

func (t *Transaction) ComputeHash() types.Hash {
	var result types.Hash

	ar := marshalArenaPool.Get()
	hash := keccak.DefaultKeccakPool.Get()

	v := t.MarshalRLPWith(ar)
	hash.WriteRlp(result[:0], v)

	marshalArenaPool.Put(ar)
	keccak.DefaultKeccakPool.Put(hash)

	return result
}

func (t *Transaction) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(t.From[:]))
	vv.Set(arena.NewUint(t.Nonce))

	// Address may be empty
	if t.Payload != nil {
		for _, v := range t.Payload {
			vv.Set(v.MarshalRLPWith(arena))
		}
	} else {
		vv.Set(arena.NewNull())
	}

	return vv
}

// Payload represents a transaction payload
type Payload struct {
	To       *types.Address `json:"to"` // TODO: allow contract creation eq To == nil?
	Value    uint64         `json:"value"`
	GasLimit uint64         `json:"gasLimit"`
	Input    []byte         `json:"data"`
}

func (p *Payload) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	// Address may be empty
	if p.To != nil {
		vv.Set(arena.NewBytes(p.To[:]))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewUint(p.Value))
	vv.Set(arena.NewUint(p.GasLimit))

	// Address may be empty
	if p.Input != nil {
		vv.Set(arena.NewBytes(p.Input))
	} else {
		vv.Set(arena.NewNull())
	}

	return vv
}

// AAReceipt represents a transaction receipt
type AAReceipt struct {
	ID     string  `json:"id"`
	Gas    uint64  `json:"gas"`
	Status string  `json:"status"`
	Mined  *Mined  `json:"mined,omitempty"`
	Error  *string `json:"error,omitempty"`
}

// Mined represents the metadata for the mined block
type Mined struct {
	BlockHash   types.Hash `json:"blockHash"`
	BlockNumber uint64     `json:"blockNumber"`
	TxnHash     types.Hash `json:"txnHash"`
	Logs        []Log      `json:"logs"`
	GasUsed     *uint64    `json:"gasUsed,omitempty"`
}

// Log represents a transaction log
type Log struct {
	Address types.Address `json:"address"`
	Topics  []string      `json:"topics"`
	Data    []byte        `json:"data"`
}

type AAPoolTransaction struct {
	ID string       `json:"id"`
	Tx *Transaction `json:"tx,omitempty"`
}

type AAStateTransaction struct {
	ID     string       `json:"id"`
	Tx     *Transaction `json:"tx,omitempty"`
	Status string       `json:"status"`
	Gas    uint64       `json:"gas"`
	Mined  *Mined       `json:"mined,omitempty"`
	Error  *string      `json:"error,omitempty"`
}
