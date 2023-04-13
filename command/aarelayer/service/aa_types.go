package service

import (
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/crypto"
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
	StatusPending   = "pending"   // tx is currently on the pending pool
	StatusQueued    = "queued"    // tx is in a queue and waiting to be sent
	StatusSent      = "sent"      // tx has been sent and is waiting for a receipt
	StatusCompleted = "completed" // tx has been successfully mined into a block and is now complete
	StatusFailed    = "failed"    // tx failed during the process and may or may not have been included in a block
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

	// invokerMethodAbiType is function invoke(
	//	Signature calldata signature, Transaction calldata transaction) external payable nonReentrant
	invokerMethodAbiType = abi.MustNewMethod(
		`function invoke(
			tuple(uint256 r,uint256 s,bool v) signature,
			tuple(address from,uint256 nonce,tuple(address to,uint256 value,uint256 gasLimit,bytes data)[] payload) transaction
		) public payable`,
	)

	//  aaInvokerNoncesAbiType is mapping(address => uint256) public nonces;
	aaInvokerNoncesAbiType = abi.MustNewMethod("function nonces(address) returns (uint256)")
)

type MagicHashFn func(chainID int64, address types.Address, commitHash []byte) []byte

// AATransaction represents an AA transaction
type AATransaction struct {
	Signature   aaSignature `json:"signature"`
	Transaction Transaction `json:"transaction"`
}

// RecoverSender recovers sender address from the AATransaction
// The implementation involves several steps:
// First, it creates an EIP712 hash for the AA transaction, which includes computing the domain separator hash.
// This is a standard procedure for creating typed structured data hashes in the Ethereum ecosystem.
// https://eips.ethereum.org/EIPS/eip-712
// The second step involves using the magicHashFn parameter, if provided, to perform additional hashing
// because of support for EIP3074. https://eips.ethereum.org/EIPS/eip-3074
// Finally, the ethgowallet.Ecrecover function is executed to retrieve the actual address from the hash and signature.
func (t *AATransaction) RecoverSender(
	address types.Address, chainID int64, magicHashFn MagicHashFn) types.Address {
	domainSeparator, err := getDomainSeparatorHash(address, chainID)
	if err != nil {
		return types.ZeroAddress
	}

	commitHash, err := t.Transaction.ComputeEip712Hash(domainSeparator)
	if err != nil {
		return types.ZeroAddress
	}

	hash := commitHash[:]

	if magicHashFn != nil {
		hash = magicHashFn(chainID, address, hash)
	}

	recoveredAddress, err := ethgowallet.Ecrecover(hash, t.Signature)
	if err != nil {
		return types.ZeroAddress
	}

	return types.Address(recoveredAddress)
}

// Sign makes signature for the Transaction and write it down inside AATransaction object
// It uses analog steps from RecoverSender method
func (t *AATransaction) Sign(
	address types.Address, chainID int64, key ethgo.Key, magicHashFn MagicHashFn) error {
	domainSeparator, err := getDomainSeparatorHash(address, chainID)
	if err != nil {
		return err
	}

	commitHash, err := t.Transaction.ComputeEip712Hash(domainSeparator)
	if err != nil {
		return err
	}

	hash := commitHash[:]

	if magicHashFn != nil {
		hash = magicHashFn(chainID, address, hash)
	}

	t.Signature, err = key.Sign(hash)
	if err != nil {
		return err
	}

	return nil
}

// ToAbi encodes AATransaction to the aa invoker smart contract call
func (t *AATransaction) ToAbi() ([]byte, error) {
	// signature "tuple(uint256 r,uint256 s,bool v)"
	signature := map[string]interface{}{
		"r": new(big.Int).SetBytes(t.Signature[:32]),
		"s": new(big.Int).SetBytes(t.Signature[32:64]),
		"v": t.Signature[64] == 1 || t.Signature[64] == 28, // {0, 1} and {27, 28} support
	}

	// payloads "tuple(address to,uint256 value,uint256 gasLimit,bytes data)"
	payloads := make([]map[string]interface{}, len(t.Transaction.Payload))

	for i, payload := range t.Transaction.Payload {
		to := types.ZeroAddress
		if payload.To != nil {
			to = *payload.To
		}

		payloads[i] = map[string]interface{}{
			"to":       to,
			"value":    new(big.Int).Set(payload.Value),
			"gasLimit": new(big.Int).Set(payload.GasLimit),
			"data":     payload.Input,
		}
	}

	// transaction
	// "tuple(address from,uint256 nonce,tuple(address to,uint256 value,uint256 gasLimit,bytes data)[] payload)"
	transaction := map[string]interface{}{
		"from":    t.Transaction.From,
		"nonce":   new(big.Int).SetUint64(t.Transaction.Nonce),
		"payload": payloads,
	}

	return invokerMethodAbiType.Encode(map[string]interface{}{
		"signature":   signature,
		"transaction": transaction,
	})
}

// Transaction represents a transaction
type Transaction struct {
	From    types.Address `json:"from"`
	Nonce   uint64        `json:"nonce"`
	Payload []Payload     `json:"payload"`
}

// ComputeEip712Hash computes hash for transaction defined by EIP-712
// explanation for algorithm is here https://eips.ethereum.org/EIPS/eip-712
func (t *Transaction) ComputeEip712Hash(domainSeparator types.Hash) (types.Hash, error) {
	txHashBytes, err := t.computeHash()
	if err != nil {
		return types.ZeroHash, err
	}

	// This encoding is deterministic because the individual components are.
	// The encoding is injective because the three cases always differ in first byte.
	// RLP_encode(transaction) does not start with \x19.
	// The encoding is compliant with EIP-191.
	// The ‘version byte’ is fixed to 0x01, the ‘version specific data’ is the 32-byte
	// domain separator domainSeparator and the ‘data to sign’ is the 32-byte hashStruct(message).
	headerBytes := [2]byte{0x19, 0x1}
	bytes := make([]byte, len(headerBytes)+len(domainSeparator)+len(txHashBytes))
	copy(bytes, headerBytes[:])
	copy(bytes[len(headerBytes):], domainSeparator[:])
	copy(bytes[len(headerBytes)+len(domainSeparator):], txHashBytes[:])

	return types.BytesToHash(crypto.Keccak256(bytes)), nil
}

func (t *Transaction) computeHash() (types.Hash, error) {
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

// AAStateTransaction represents AATransaction that is kept in store and in the pool
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
	Hash         ethgo.Hash     `json:"hash,omitempty"`
	Raw          []byte         `json:"raw,omitempty"`
	Nonce        uint64         `json:"nonce,omitempty"`
}

func getDomainSeparatorHash(address types.Address, chainID int64) (types.Hash, error) {
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
