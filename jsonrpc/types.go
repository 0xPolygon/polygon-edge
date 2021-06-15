package jsonrpc

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
)

type transaction struct {
	Nonce       argUint64      `json:"nonce"`
	GasPrice    argBig         `json:"gasPrice"`
	Gas         argUint64      `json:"gas"`
	To          *types.Address `json:"to"`
	Value       argBig         `json:"value"`
	Input       argBytes       `json:"input"`
	V           argByte        `json:"v"`
	R           argBytes       `json:"r"`
	S           argBytes       `json:"s"`
	Hash        types.Hash     `json:"hash"`
	From        types.Address  `json:"from"`
	BlockHash   types.Hash     `json:"blockHash"`
	BlockNumber argUint64      `json:"blockNumber"`
	TxIndex     argUint64      `json:"transactionIndex"`
}

func toTransaction(t *types.Transaction, b *types.Block, txIndex int) *transaction {
	return &transaction{
		Nonce:       argUint64(t.Nonce),
		GasPrice:    argBig(*t.GasPrice),
		Gas:         argUint64(t.Gas),
		To:          t.To,
		Value:       argBig(*t.Value),
		Input:       argBytes(t.Input),
		V:           argByte(t.V),
		R:           argBytes(t.R),
		S:           argBytes(t.S),
		Hash:        t.Hash,
		From:        t.From,
		BlockHash:   b.Hash(),
		BlockNumber: argUint64(b.Number()),
		TxIndex:     argUint64(txIndex),
	}
}

type block struct {
	ParentHash   types.Hash     `json:"parentHash"`
	Sha3Uncles   types.Hash     `json:"sha3Uncles"`
	Miner        types.Address  `json:"miner"`
	StateRoot    types.Hash     `json:"stateRoot"`
	TxRoot       types.Hash     `json:"transactionsRoot"`
	ReceiptsRoot types.Hash     `json:"receiptsRoot"`
	LogsBloom    types.Bloom    `json:"logsBloom"`
	Difficulty   argUint64      `json:"difficulty"`
	Number       argUint64      `json:"number"`
	GasLimit     argUint64      `json:"gasLimit"`
	GasUsed      argUint64      `json:"gasUsed"`
	Timestamp    argUint64      `json:"timestamp"`
	ExtraData    argBytes       `json:"extraData"`
	MixHash      types.Hash     `json:"mixHash"`
	Nonce        types.Nonce    `json:"nonce"`
	Hash         types.Hash     `json:"hash"`
	Transactions []*transaction `json:"transactions"`
}

func toBlock(b *types.Block) *block {
	h := b.Header
	res := &block{
		ParentHash:   h.ParentHash,
		Sha3Uncles:   h.Sha3Uncles,
		Miner:        h.Miner,
		StateRoot:    h.StateRoot,
		TxRoot:       h.TxRoot,
		ReceiptsRoot: h.ReceiptsRoot,
		LogsBloom:    h.LogsBloom,
		Difficulty:   argUint64(h.Difficulty),
		Number:       argUint64(h.Number),
		GasLimit:     argUint64(h.GasLimit),
		GasUsed:      argUint64(h.GasUsed),
		Timestamp:    argUint64(h.Timestamp),
		ExtraData:    argBytes(h.ExtraData),
		MixHash:      h.MixHash,
		Nonce:        h.Nonce,
		Hash:         h.Hash,
		Transactions: []*transaction{},
	}
	for idx, txn := range b.Transactions {
		res.Transactions = append(res.Transactions, toTransaction(txn, b, idx))
	}
	return res
}

type receipt struct {
	Root              types.Hash           `json:"root"`
	CumulativeGasUsed argUint64            `json:"cumulativeGasUsed"`
	LogsBloom         types.Bloom          `json:"logsBloom"`
	Logs              []*Log               `json:"logs"`
	Status            *types.ReceiptStatus `json:"status"`
	TxHash            types.Hash           `json:"transactionHash"`
	TxIndex           argUint64            `json:"transactionIndex"`
	BlockHash         types.Hash           `json:"blockHash"`
	BlockNumber       argUint64            `json:"blockNumber"`
	GasUsed           argUint64            `json:"gasUsed"`
	ContractAddress   types.Address        `json:"contractAddress"`
	FromAddr          types.Address        `json:"from"`
	ToAddr            *types.Address       `json:"to"`
}

type Log struct {
	Address     types.Address `json:"address"`
	Topics      []types.Hash  `json:"topics"`
	Data        argBytes      `json:"data"`
	BlockNumber argUint64     `json:"blockNumber"`
	TxHash      types.Hash    `json:"transactionHash"`
	TxIndex     argUint64     `json:"transactionIndex"`
	BlockHash   types.Hash    `json:"blockHash"`
	LogIndex    argUint64     `json:"logIndex"`
	Removed     bool          `json:"removed"`
}

type argByte byte

func (a argByte) MarshalText() ([]byte, error) {
	return encodeToHex([]byte{byte(a)}), nil
}

type argBig big.Int

func argBigPtr(b *big.Int) *argBig {
	v := argBig(*b)
	return &v
}

func (a *argBig) UnmarshalText(input []byte) error {
	buf, err := decodeToHex(input)
	if err != nil {
		return err
	}
	b := new(big.Int)
	b.SetBytes(buf)
	*a = argBig(*b)
	return nil
}

func (a argBig) MarshalText() ([]byte, error) {
	b := (*big.Int)(&a)
	return []byte("0x" + b.Text(16)), nil
}

func argAddrPtr(a types.Address) *types.Address {
	return &a
}

type argUint64 uint64

func argUintPtr(n uint64) *argUint64 {
	v := argUint64(n)
	return &v
}

func (b argUint64) MarshalText() ([]byte, error) {
	buf := make([]byte, 2, 10)
	copy(buf, `0x`)
	buf = strconv.AppendUint(buf, uint64(b), 16)
	return buf, nil
}

func (u *argUint64) UnmarshalText(input []byte) error {
	str := strings.TrimPrefix(string(input), "0x")
	num, err := strconv.ParseUint(str, 16, 64)
	if err != nil {
		return err
	}
	*u = argUint64(num)
	return nil
}

type argBytes []byte

func argBytesPtr(b []byte) *argBytes {
	bb := argBytes(b)
	return &bb
}

func (b argBytes) MarshalText() ([]byte, error) {
	return encodeToHex(b), nil
}

func (b *argBytes) UnmarshalText(input []byte) error {
	hh, err := decodeToHex(input)
	if err != nil {
		return nil
	}
	aux := make([]byte, len(hh))
	copy(aux[:], hh[:])
	*b = aux
	return nil
}

func decodeToHex(b []byte) ([]byte, error) {
	str := string(b)
	str = strings.TrimPrefix(str, "0x")
	if len(str)%2 != 0 {
		str = "0" + str
	}
	return hex.DecodeString(str)
}

func encodeToHex(b []byte) []byte {
	str := hex.EncodeToString(b)
	if len(str)%2 != 0 {
		str = "0" + str
	}
	return []byte("0x" + str)
}

// txnArgs is the transaction argument for the rpc endpoints
type txnArgs struct {
	From     *types.Address
	To       *types.Address
	Gas      *argUint64
	GasPrice *argBytes
	Value    *argBytes
	Input    *argBytes
	Data     *argBytes
	Nonce    *argUint64
}
