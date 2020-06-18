package types

import (
	"github.com/0xPolygon/minimal/helper/keccak"
)

type Transaction struct {
	Nonce    uint64   `json:"nonce" db:"nonce"`
	GasPrice HexBytes `json:"gasPrice" db:"gas_price"`
	Gas      uint64   `json:"gas" db:"gas"`
	To       *Address `json:"to" db:"dst"`
	Value    HexBytes `json:"value" db:"value"`
	Input    HexBytes `json:"input" db:"input"`

	V byte     `json:"v" db:"v"`
	R HexBytes `json:"r" db:"r"`
	S HexBytes `json:"s" db:"s"`

	Hash Hash
	From Address
}

func (t *Transaction) IsContractCreation() bool {
	return t.To == nil
}

func (t *Transaction) GetGasPrice() []byte {
	return t.GasPrice
}

// ComputeHash computes the hash of the transaction
func (t *Transaction) ComputeHash() *Transaction {
	ar := marshalArenaPool.Get()
	hash := keccak.DefaultKeccakPool.Get()

	v := t.MarshalWith(ar)
	hash.WriteRlp(t.Hash[:0], v)

	marshalArenaPool.Put(ar)
	keccak.DefaultKeccakPool.Put(hash)
	return t
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
