package invoker

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

type InvokerSignature struct {
	R *big.Int `abi:"r"`
	S *big.Int `abi:"s"`
	V bool     `abi:"v"`
}

func (is *InvokerSignature) String() string {
	return fmt.Sprintf("r: %v, s: %v v: %v", hex.EncodeToHex(is.R.Bytes()), hex.EncodeToHex(is.S.Bytes()), is.V)
}

func eip3074Magic(commit []byte, invokerAddr types.Address) []byte {
	var msg [64]byte
	// EIP-3074 messages are of the form
	// keccak256(type ++ invoker ++ commit)
	msg[0] = 0x03
	copy(msg[13:33], invokerAddr.Bytes())
	copy(msg[33:], commit)
	return ethgo.Keccak256(msg[:])
}

func (is *InvokerSignature) Recover(commit []byte, invokerAddr types.Address) (addr types.Address) {

	sig := make([]byte, 65)
	copy(sig[32-len(is.R.Bytes()):32], is.R.Bytes())
	copy(sig[64-len(is.S.Bytes()):64], is.S.Bytes())
	if is.V {
		sig[64] = 0x01
	}

	if pubKey, err := crypto.RecoverPubkey(sig, eip3074Magic(commit, invokerAddr)); err == nil {
		addr = crypto.PubKeyToAddress(pubKey)
	}
	return
}

func (is *InvokerSignature) SignCommit(key ethgo.Key, commit []byte, invokerAddr ethgo.Address) (err error) {

	var sig []byte
	if sig, err = key.Sign(eip3074Magic(commit, types.Address(invokerAddr))); err != nil {
		return
	}

	is.R = new(big.Int).SetBytes(sig[0:32])
	is.S = new(big.Int).SetBytes(sig[32:64])
	if sig[64] == 0 {
		is.V = false
	} else {
		is.V = true
	}

	return
}

type InvokerTransaction struct {
	From     types.Address        `abi:"from"`
	Nonce    *big.Int             `abi:"nonce"`
	Payloads []TransactionPayload `abi:"payloads"` // should this be an array of pointers?
}

func (it *InvokerTransaction) ComputeHash() *InvokerTransaction {
	return it
}

// return keccak256(abi.encode(TRANSACTION_TYPE, transaction.from, transaction.nonce, hashPayloads(transaction.payloads)));

type encTransaction struct {
	TypeHash     [32]byte      `abi:"typeHash"`
	From         types.Address `abi:"from"`
	Nonce        *big.Int      `abi:"nonce"`
	PayloadsHash [32]byte      `abi:"payloadsHash"`
}

var transactionType = ethgo.Keccak256([]byte("Transaction(address from,uint256 nonce,TransactionPayload[] payloads)TransactionPayload(address to,uint256 value,uint256 gasLimit,bytes data)"))

func (it *InvokerTransaction) InvokerCommit(domainSeparator []byte) (c []byte, err error) {
	values := []byte{0x19, 0x01}
	values = append(values, domainSeparator...)
	if c, err = it.InvokerHash(); err != nil {
		return
	}
	values = append(values, c...)
	c = ethgo.Keccak256(values)
	return
}

func (it *InvokerTransaction) InvokerHash() (h []byte, err error) {

	var t *abi.Type
	if t, err = abi.NewType("tuple(bytes32 typeHash, address from, uint256 nonce, bytes32 payloadsHash)"); err != nil {
		return
	}

	enc := encTransaction{
		From:  it.From,
		Nonce: it.Nonce,
	}
	copy(enc.TypeHash[:], transactionType)
	tps := TransactionPayloads(it.Payloads)
	if h, err = tps.InvokerHash(); err != nil {
		return
	}
	copy(enc.PayloadsHash[:], h)

	var encBytes []byte
	if encBytes, err = abi.Encode(&enc, t); err != nil {
		return
	}

	h = ethgo.Keccak256(encBytes)
	return

}

type TransactionPayloads []TransactionPayload

func (tps TransactionPayloads) InvokerHash() (h []byte, err error) {
	var values []byte
	for i := range tps {
		if h, err = tps[i].InvokerHash(); err != nil {
			return
		}
		values = append(values, h...)
	}
	// return keccak256(abi.encodePacked(values));
	h = ethgo.Keccak256(values)
	return
}

func (tps TransactionPayloads) Payloads() []TransactionPayload {
	return tps
}

type TransactionPayload struct {
	To       types.Address `abi:"to"`
	Value    *big.Int      `abi:"value"`
	GasLimit *big.Int      `abi:"gasLimit"`
	Data     []byte        `abi:"data"`
}

type encPayload struct {
	TypeHash [32]byte      `abi:"typeHash"`
	To       types.Address `abi:"to"`
	Value    *big.Int      `abi:"value"`
	GasLimit *big.Int      `abi:"gasLimit"`
	DataHash [32]byte      `abi:"dataHash"`
}

// from AccountAbstractionInvoker.sol: TRANSACTION_PAYLOAD_TYPE
var transactionPayloadType = ethgo.Keccak256([]byte("TransactionPayload(address to,uint256 value,uint256 gasLimit,bytes data)"))

func (tp *TransactionPayload) InvokerHash() (h []byte, err error) {

	var t *abi.Type
	if t, err = abi.NewType("tuple(bytes32 typeHash, address to, uint256 value, uint256 gasLimit, bytes32 dataHash)"); err != nil {
		return
	}

	enc := encPayload{
		To:       tp.To,
		Value:    tp.Value,
		GasLimit: tp.GasLimit,
	}
	copy(enc.TypeHash[:], transactionPayloadType)
	copy(enc.DataHash[:], ethgo.Keccak256(tp.Data))

	var encBytes []byte
	if encBytes, err = abi.Encode(&enc, t); err != nil {
		return
	}

	h = ethgo.Keccak256(encBytes)
	return
}

type SessionToken struct {
	Delegate   types.Address `abi:"delegate"`
	Expiration *big.Int      `abi:"expiration"`
}

type encSessionToken struct {
	TypeHash   [32]byte      `abi:"typeHash"`
	Delegate   types.Address `abi:"delegate"`
	Expiration *big.Int      `abi:"expiration"`
}

// from AccountSessionInvoker.sol: SESSION_TOKEN_TYPE
var sessionTokenType = ethgo.Keccak256([]byte("SessionToken(address delegate,uint256 expiration)"))

func (tp *SessionToken) InvokerHash() (h []byte, err error) {

	var t *abi.Type
	if t, err = abi.NewType("tuple(bytes32 typeHash, address delegate, uint256 expiration)"); err != nil {
		return
	}

	enc := encSessionToken{
		Delegate:   tp.Delegate,
		Expiration: tp.Expiration,
	}
	copy(enc.TypeHash[:], sessionTokenType)

	var encBytes []byte
	if encBytes, err = abi.Encode(&enc, t); err != nil {
		return
	}

	h = ethgo.Keccak256(encBytes)
	return
}
