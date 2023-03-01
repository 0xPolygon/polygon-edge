package service

import (
	"crypto/ecdsa"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

// Types for the structures to be hashed
var (
	transactionTypeAbi = abi.MustNewType( // Transaction
		"tuple(address from,uint256 nonce,bytes32 payloadHash)",
	)
	transactionPayloadTypeAbi = abi.MustNewType( // TransactionPayload
		"tuple(address to,uint256 value,uint256 gasLimit,bytes data)",
	)
)

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
	hash, err := t.Transaction.ComputeHash()
	if err != nil {
		return err
	}

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
	hash, err := t.ComputeHash() // recompute hash
	if err != nil {
		return false
	}

	pubKey, err := crypto.Ecrecover(hash[:], signature)
	if err != nil {
		return false
	}

	return t.From == types.BytesToAddress(crypto.Keccak256(pubKey[1:])[12:])
}

func (t *Transaction) ComputeHash() (types.Hash, error) {
	payload := make([]byte, len(t.Payload)*types.HashLength)

	for i, p := range t.Payload {
		hash, err := p.Hash()
		if err != nil {
			return types.ZeroHash, err
		}

		// abi.encodePacked joins all the bytes into single slice
		copy(payload[i*types.HashLength:], hash[:])
	}

	bytes, err := abi.Encode(
		[]interface{}{t.From, t.Nonce, crypto.Keccak256(payload)},
		transactionTypeAbi)
	if err != nil {
		return types.ZeroHash, err
	}

	return types.BytesToHash(crypto.Keccak256(bytes)), nil
}

// Payload represents a transaction payload
type Payload struct {
	To       *types.Address `json:"to"` // TODO: allow contract creation eq To == nil?
	Value    uint64         `json:"value"`
	GasLimit uint64         `json:"gasLimit"`
	Input    []byte         `json:"data"`
}

func (p *Payload) Hash() (types.Hash, error) {
	var (
		data []byte
		to   = types.ZeroAddress
	)

	if p.Input != nil {
		data = crypto.Keccak256(p.Input)
	}

	if p.To != nil {
		to = *p.To
	}

	bytes, err := abi.Encode(
		[]interface{}{to, p.Value, p.GasLimit, data},
		transactionPayloadTypeAbi)
	if err != nil {
		return types.ZeroHash, err
	}

	return types.BytesToHash(crypto.Keccak256(bytes)), nil
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

type AAStateTransaction struct {
	ID     string         `json:"id"`
	Tx     *AATransaction `json:"tx,omitempty"`
	Time   int64          `json:"time"`
	Status string         `json:"status"`
	Gas    uint64         `json:"gas"`
	Mined  *Mined         `json:"mined,omitempty"`
	Error  *string        `json:"error,omitempty"`
}
