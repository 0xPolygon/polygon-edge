package jsonrpc

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
)

const jsonRPCMetric = "json_rpc"

// For union type of transaction and types.Hash
type transactionOrHash interface {
	getHash() types.Hash
}

type transaction struct {
	Nonce       argUint64      `json:"nonce"`
	GasPrice    *argBig        `json:"gasPrice,omitempty"`
	GasTipCap   *argBig        `json:"maxPriorityFeePerGas,omitempty"`
	GasFeeCap   *argBig        `json:"maxFeePerGas,omitempty"`
	Gas         argUint64      `json:"gas"`
	To          *types.Address `json:"to"`
	Value       argBig         `json:"value"`
	Input       argBytes       `json:"input"`
	V           argBig         `json:"v"`
	R           argBig         `json:"r"`
	S           argBig         `json:"s"`
	Hash        types.Hash     `json:"hash"`
	From        types.Address  `json:"from"`
	BlockHash   *types.Hash    `json:"blockHash"`
	BlockNumber *argUint64     `json:"blockNumber"`
	TxIndex     *argUint64     `json:"transactionIndex"`
	ChainID     *argBig        `json:"chainId,omitempty"`
	Type        argUint64      `json:"type"`
}

func (t transaction) getHash() types.Hash { return t.Hash }

// Redefine to implement getHash() of transactionOrHash
type transactionHash types.Hash

func (h transactionHash) getHash() types.Hash { return types.Hash(h) }

func (h transactionHash) MarshalText() ([]byte, error) {
	return []byte(types.Hash(h).String()), nil
}

func toPendingTransaction(t *types.Transaction) *transaction {
	return toTransaction(t, nil, nil, nil)
}

func toTransaction(
	t *types.Transaction,
	blockNumber *argUint64,
	blockHash *types.Hash,
	txIndex *int,
) *transaction {
	res := &transaction{
		Nonce:       argUint64(t.Nonce),
		Gas:         argUint64(t.Gas),
		To:          t.To,
		Value:       argBig(*t.Value),
		Input:       t.Input,
		V:           argBig(*t.V),
		R:           argBig(*t.R),
		S:           argBig(*t.S),
		Hash:        t.Hash,
		From:        t.From,
		Type:        argUint64(t.Type),
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
	}

	if t.GasPrice != nil {
		gasPrice := argBig(*t.GasPrice)
		res.GasPrice = &gasPrice
	}

	if t.GasTipCap != nil {
		gasTipCap := argBig(*t.GasTipCap)
		res.GasTipCap = &gasTipCap
	}

	if t.GasFeeCap != nil {
		gasFeeCap := argBig(*t.GasFeeCap)
		res.GasFeeCap = &gasFeeCap
	}

	if t.ChainID != nil {
		chainID := argBig(*t.ChainID)
		res.ChainID = &chainID
	}

	if txIndex != nil {
		res.TxIndex = argUintPtr(uint64(*txIndex))
	}

	return res
}

type block struct {
	ParentHash      types.Hash          `json:"parentHash"`
	Sha3Uncles      types.Hash          `json:"sha3Uncles"`
	Miner           argBytes            `json:"miner"`
	StateRoot       types.Hash          `json:"stateRoot"`
	TxRoot          types.Hash          `json:"transactionsRoot"`
	ReceiptsRoot    types.Hash          `json:"receiptsRoot"`
	LogsBloom       types.Bloom         `json:"logsBloom"`
	Difficulty      argUint64           `json:"difficulty"`
	TotalDifficulty argUint64           `json:"totalDifficulty"`
	Size            argUint64           `json:"size"`
	Number          argUint64           `json:"number"`
	GasLimit        argUint64           `json:"gasLimit"`
	GasUsed         argUint64           `json:"gasUsed"`
	Timestamp       argUint64           `json:"timestamp"`
	ExtraData       argBytes            `json:"extraData"`
	MixHash         types.Hash          `json:"mixHash"`
	Nonce           types.Nonce         `json:"nonce"`
	Hash            types.Hash          `json:"hash"`
	Transactions    []transactionOrHash `json:"transactions"`
	Uncles          []types.Hash        `json:"uncles"`
	BaseFee         argUint64           `json:"baseFeePerGas,omitempty"`
}

func (b *block) Copy() *block {
	bb := new(block)
	*bb = *b

	bb.Miner = make([]byte, len(b.Miner))
	copy(bb.Miner[:], b.Miner[:])

	bb.ExtraData = make([]byte, len(b.ExtraData))
	copy(bb.ExtraData[:], b.ExtraData[:])

	return bb
}

func toBlock(b *types.Block, fullTx bool) *block {
	h := b.Header
	res := &block{
		ParentHash:      h.ParentHash,
		Sha3Uncles:      h.Sha3Uncles,
		Miner:           argBytes(h.Miner),
		StateRoot:       h.StateRoot,
		TxRoot:          h.TxRoot,
		ReceiptsRoot:    h.ReceiptsRoot,
		LogsBloom:       h.LogsBloom,
		Difficulty:      argUint64(h.Difficulty),
		TotalDifficulty: argUint64(h.Difficulty), // not needed for POS
		Size:            argUint64(b.Size()),
		Number:          argUint64(h.Number),
		GasLimit:        argUint64(h.GasLimit),
		GasUsed:         argUint64(h.GasUsed),
		Timestamp:       argUint64(h.Timestamp),
		ExtraData:       argBytes(h.ExtraData),
		MixHash:         h.MixHash,
		Nonce:           h.Nonce,
		Hash:            h.Hash,
		Transactions:    []transactionOrHash{},
		Uncles:          []types.Hash{},
		BaseFee:         argUint64(h.BaseFee),
	}

	for idx, txn := range b.Transactions {
		if fullTx {
			txn.GasPrice = txn.GetGasPrice(b.Header.BaseFee)
			res.Transactions = append(
				res.Transactions,
				toTransaction(
					txn,
					argUintPtr(b.Number()),
					argHashPtr(b.Hash()),
					&idx,
				),
			)
		} else {
			res.Transactions = append(
				res.Transactions,
				transactionHash(txn.Hash),
			)
		}
	}

	for _, uncle := range b.Uncles {
		res.Uncles = append(res.Uncles, uncle.Hash)
	}

	return res
}

type receipt struct {
	Root              types.Hash     `json:"root"`
	CumulativeGasUsed argUint64      `json:"cumulativeGasUsed"`
	LogsBloom         types.Bloom    `json:"logsBloom"`
	Logs              []*Log         `json:"logs"`
	Status            argUint64      `json:"status"`
	TxHash            types.Hash     `json:"transactionHash"`
	TxIndex           argUint64      `json:"transactionIndex"`
	BlockHash         types.Hash     `json:"blockHash"`
	BlockNumber       argUint64      `json:"blockNumber"`
	GasUsed           argUint64      `json:"gasUsed"`
	ContractAddress   *types.Address `json:"contractAddress"`
	FromAddr          types.Address  `json:"from"`
	ToAddr            *types.Address `json:"to"`
}

func toReceipt(src *types.Receipt, tx *types.Transaction,
	txIndex uint64, header *types.Header, logs []*Log) *receipt {
	return &receipt{
		Root:              src.Root,
		CumulativeGasUsed: argUint64(src.CumulativeGasUsed),
		LogsBloom:         src.LogsBloom,
		Status:            argUint64(*src.Status),
		TxHash:            tx.Hash,
		TxIndex:           argUint64(txIndex),
		BlockHash:         header.Hash,
		BlockNumber:       argUint64(header.Number),
		GasUsed:           argUint64(src.GasUsed),
		ContractAddress:   src.ContractAddress,
		FromAddr:          tx.From,
		ToAddr:            tx.To,
		Logs:              logs,
	}
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

func toLogs(srcLogs []*types.Log, baseIdx, txIdx uint64, header *types.Header, txHash types.Hash) []*Log {
	logs := make([]*Log, len(srcLogs))
	for i, srcLog := range srcLogs {
		logs[i] = toLog(srcLog, baseIdx+uint64(i), txIdx, header, txHash)
	}

	return logs
}

func toLog(src *types.Log, logIdx, txIdx uint64, header *types.Header, txHash types.Hash) *Log {
	return &Log{
		Address:     src.Address,
		Topics:      src.Topics,
		Data:        argBytes(src.Data),
		BlockNumber: argUint64(header.Number),
		BlockHash:   header.Hash,
		TxHash:      txHash,
		TxIndex:     argUint64(txIdx),
		LogIndex:    argUint64(logIdx),
	}
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

func argHashPtr(h types.Hash) *types.Hash {
	return &h
}

type argUint64 uint64

func argUintPtr(n uint64) *argUint64 {
	v := argUint64(n)

	return &v
}

func (u argUint64) MarshalText() ([]byte, error) {
	buf := make([]byte, 2, 10)
	copy(buf, `0x`)
	buf = strconv.AppendUint(buf, uint64(u), 16)

	return buf, nil
}

func (u *argUint64) UnmarshalText(input []byte) error {
	str := strings.Trim(string(input), "\"")

	num, err := common.ParseUint64orHex(&str)
	if err != nil {
		return err
	}

	*u = argUint64(num)

	return nil
}

func (u *argUint64) UnmarshalJSON(buffer []byte) error {
	return u.UnmarshalText(buffer)
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
	From      *types.Address
	To        *types.Address
	Gas       *argUint64
	GasPrice  *argBytes
	GasTipCap *argBytes
	GasFeeCap *argBytes
	Value     *argBytes
	Data      *argBytes
	Input     *argBytes
	Nonce     *argUint64
	Type      *argUint64
}

type progression struct {
	Type          string    `json:"type"`
	StartingBlock argUint64 `json:"startingBlock"`
	CurrentBlock  argUint64 `json:"currentBlock"`
	HighestBlock  argUint64 `json:"highestBlock"`
}

type feeHistoryResult struct {
	OldestBlock   argUint64     `json:"oldestBlock"`
	BaseFeePerGas []argUint64   `json:"baseFeePerGas,omitempty"`
	GasUsedRatio  []float64     `json:"gasUsedRatio"`
	Reward        [][]argUint64 `json:"reward,omitempty"`
}

func convertToArgUint64Slice(slice []uint64) []argUint64 {
	argSlice := make([]argUint64, len(slice))
	for i, value := range slice {
		argSlice[i] = argUint64(value)
	}

	return argSlice
}

func convertToArgUint64SliceSlice(slice [][]uint64) [][]argUint64 {
	argSlice := make([][]argUint64, len(slice))
	for i, value := range slice {
		argSlice[i] = convertToArgUint64Slice(value)
	}

	return argSlice
}
