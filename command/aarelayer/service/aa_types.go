package service

import (
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	ethgowallet "github.com/umbracle/ethgo/wallet"
)

const (
	signatureLength        = 65
	domainSeparatorName    = "Account Abstraction Invoker"
	domainSeparatorVersion = "1.0.0"

	// Statuses
	StatusPending   = "pending"   // The AA transaction is on the Pool
	StatusQueued    = "queued"    // The AA transaction is waiting to be mined.
	StatusCompleted = "completed" // The `AA transaction` was mined in a block.
	StatusFailed    = "failed"    // AA transaction` failed during the process.
)

// Types and keccak256 values of types from AccountAbstractionInvoker.sol
var (
	transactionType        = crypto.Keccak256([]byte("Transaction(address from,uint256 nonce,TransactionPayload[] payload)TransactionPayload(address to,uint256 value,uint256 gasLimit,bytes data)")) //nolint
	transactionPayloadType = crypto.Keccak256([]byte("TransactionPayload(address to,uint256 value,uint256 gasLimit,bytes data)"))                                                                     //nolint
	eip712DomainType       = crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))                                                           //nolint

	transactionTypeAbi = abi.MustNewType( // Transaction
		"tuple(bytes32 typeHash, address from, uint256 nonce, bytes32 payloadsHash)",
	)
	transactionPayloadTypeAbi = abi.MustNewType( // TransactionPayload
		"tuple(bytes32 typeHash, address to, uint256 value, uint256 gasLimit, bytes32 dataHash)",
	)
	eip712DomainTypeAbi = abi.MustNewType( // EIP712Domain
		"tuple(bytes32 typeHash, bytes32 name, bytes32 version, uint256 chainId, address verifyingContract)",
	)

	//  aaInvokerNoncesAbiType is mapping(address => uint256) public nonces;
	aaInvokerNoncesAbiType = abi.MustNewMethod("function nonces(address) returns (uint256)")
)

// AATransaction represents an AA transaction
type AATransaction struct {
	Signature   aaSignature `json:"signature"`
	Transaction Transaction `json:"transaction"`
}

func (t *AATransaction) GetAddressFromSignature(address types.Address, chainID int64) types.Address {
	domainSeparator, err := GetDomainSeperatorHash(address, chainID)
	if err != nil {
		return types.ZeroAddress
	}

	hash, err := t.Transaction.ComputeEip712Hash(domainSeparator)
	if err != nil {
		return types.ZeroAddress
	}

	recoveredAddress, err := ethgowallet.Ecrecover(Make3074Hash(chainID, address, hash[:]), t.Signature)
	if err != nil {
		return types.ZeroAddress
	}

	return types.Address(recoveredAddress)
}

func (t *AATransaction) MakeSignature(address types.Address, chainID int64, key ethgo.Key) error {
	domainSeparator, err := GetDomainSeperatorHash(address, chainID)
	if err != nil {
		return err
	}

	hash, err := t.Transaction.ComputeEip712Hash(domainSeparator)
	if err != nil {
		return err
	}

	t.Signature, err = key.Sign(Make3074Hash(chainID, address, hash[:]))
	if err != nil {
		return err
	}

	return nil
}

// Transaction represents a transaction
type Transaction struct {
	From    types.Address `json:"from"`
	Nonce   uint64        `json:"nonce"`
	Payload []Payload     `json:"payload"`
}

func (t *Transaction) ComputeEip712Hash(domainSeparator types.Hash) (types.Hash, error) {
	txHashBytes, err := t.ComputeHash()
	if err != nil {
		return types.ZeroHash, err
	}

	headerBytes := [2]byte{0x19, 0x1}
	bytes := make([]byte, len(headerBytes)+len(domainSeparator)+len(txHashBytes))
	copy(bytes, headerBytes[:])
	copy(bytes[len(headerBytes):], domainSeparator[:])
	copy(bytes[len(headerBytes)+len(domainSeparator):], txHashBytes[:])

	return types.BytesToHash(crypto.Keccak256(bytes)), nil
}

func (t *Transaction) ComputeHash() (types.Hash, error) {
	// "tuple(bytes32 typeHash, address from, uint256 nonce, bytes32 payloadsHash)")
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
		[]interface{}{
			transactionType,
			t.From,
			t.Nonce,
			crypto.Keccak256(payload),
		},
		transactionTypeAbi)
	if err != nil {
		return types.ZeroHash, err
	}

	return types.BytesToHash(crypto.Keccak256(bytes)), nil
}

// Payload represents a transaction payload
type Payload struct {
	To       *types.Address `json:"to"`
	Value    *big.Int       `json:"value"`
	GasLimit *big.Int       `json:"gasLimit"`
	Input    []byte         `json:"data"`
}

func (p *Payload) Hash() (types.Hash, error) {
	if p.GasLimit == nil {
		return types.ZeroHash, errors.New("gas limit not specified")
	}

	if p.Value == nil {
		return types.ZeroHash, errors.New("value not specified")
	}

	to := types.ZeroAddress
	if p.To != nil {
		to = *p.To
	}

	bytes, err := abi.Encode(
		[]interface{}{
			transactionPayloadType,
			to,
			p.Value,
			p.GasLimit,
			crypto.Keccak256(p.Input),
		},
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
	GasUsed     uint64     `json:"gasUsed"`
}

// Log represents a transaction log
type Log struct {
	Address types.Address `json:"address"`
	Topics  []types.Hash  `json:"topics"`
	Data    []byte        `json:"data"`
}

type AAStateTransaction struct {
	ID           string         `json:"id"`
	Tx           *AATransaction `json:"tx,omitempty"`
	Time         int64          `json:"time"`
	TimeQueued   int64          `json:"time_queued"`
	TimeFinished int64          `json:"time_completed"`
	Status       string         `json:"status"`
	Gas          uint64         `json:"gas"`
	Mined        *Mined         `json:"mined,omitempty"`
	Error        *string        `json:"error,omitempty"`
}

func GetDomainSeperatorHash(address types.Address, chainID int64) (types.Hash, error) {
	bytes, err := abi.Encode(
		[]interface{}{
			eip712DomainType,
			crypto.Keccak256([]byte(domainSeparatorName)),
			crypto.Keccak256([]byte(domainSeparatorVersion)),
			new(big.Int).SetInt64(chainID),
			address,
		},
		eip712DomainTypeAbi)
	if err != nil {
		return types.ZeroHash, err
	}

	return types.BytesToHash(crypto.Keccak256(bytes)), nil
}

// Make3074Hash serialize EIP-3074 messages in form keccak256(type ++ invoker ++ commit)
func Make3074Hash(chainID int64, invokerAddr types.Address, commit []byte) []byte {
	var msg [97]byte
	msg[0] = 0x03
	copy(msg[1:], common.PadLeftOrTrim(big.NewInt(chainID).Bytes(), 32))
	copy(msg[45:65], invokerAddr.Bytes())
	copy(msg[65:], commit)

	return ethgo.Keccak256(msg[:])
}

type aaSignature []byte

func (sig *aaSignature) UnmarshalText(text []byte) (err error) {
	*sig, err = hex.DecodeString(strings.TrimPrefix(string(text), "0x"))

	return err
}

func (sig aaSignature) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(sig)), nil
}

func (sig aaSignature) IsValid() bool {
	return len(sig) == signatureLength
}
