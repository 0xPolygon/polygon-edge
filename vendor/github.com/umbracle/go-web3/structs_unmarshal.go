package web3

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/valyala/fastjson"
)

var defaultPool fastjson.ParserPool

// UnmarshalJSON implements the unmarshal interface
func (b *Block) UnmarshalJSON(buf []byte) error {
	p := defaultPool.Get()
	defer defaultPool.Put(p)

	v, err := p.Parse(string(buf))
	if err != nil {
		return err
	}

	if err := decodeHash(&b.Hash, v, "hash"); err != nil {
		return err
	}
	if err := decodeHash(&b.ParentHash, v, "parentHash"); err != nil {
		return err
	}
	if err := decodeHash(&b.Sha3Uncles, v, "sha3Uncles"); err != nil {
		return err
	}
	if err := decodeHash(&b.TransactionsRoot, v, "transactionsRoot"); err != nil {
		return err
	}
	if err := decodeHash(&b.StateRoot, v, "stateRoot"); err != nil {
		return err
	}
	if err := decodeHash(&b.ReceiptsRoot, v, "receiptsRoot"); err != nil {
		return err
	}
	if err := decodeAddr(&b.Miner, v, "miner"); err != nil {
		return err
	}
	if b.Number, err = decodeUint(v, "number"); err != nil {
		return err
	}
	if b.GasLimit, err = decodeUint(v, "gasLimit"); err != nil {
		return err
	}
	if b.GasUsed, err = decodeUint(v, "gasUsed"); err != nil {
		return err
	}
	if b.Timestamp, err = decodeUint(v, "timestamp"); err != nil {
		return err
	}
	if b.Difficulty, err = decodeBigInt(b.Difficulty, v, "difficulty"); err != nil {
		return err
	}
	if b.ExtraData, err = decodeBytes(b.ExtraData[:0], v, "extraData"); err != nil {
		return err
	}

	b.TransactionsHashes = b.TransactionsHashes[:0]
	b.Transactions = b.Transactions[:0]

	elems := v.GetArray("transactions")
	if len(elems) != 0 {
		if elems[0].Type() == fastjson.TypeString {
			// hashes (non full block)
			for _, elem := range elems {
				var h Hash
				if err := h.UnmarshalText(elem.GetStringBytes()); err != nil {
					return err
				}
				b.TransactionsHashes = append(b.TransactionsHashes, h)
			}
		} else {
			// structs (full block)
			for _, elem := range elems {
				txn := new(Transaction)
				if err := txn.unmarshalJSON(elem); err != nil {
					return err
				}
				b.Transactions = append(b.Transactions, txn)
			}
		}
	}

	// uncles
	b.Uncles = b.Uncles[:0]
	for _, elem := range v.GetArray("uncles") {
		var h Hash
		if err := h.UnmarshalText(elem.GetStringBytes()); err != nil {
			return err
		}
		b.Uncles = append(b.Uncles, h)
	}

	return nil
}

// UnmarshalJSON implements the unmarshal interface
func (t *Transaction) UnmarshalJSON(buf []byte) error {
	p := defaultPool.Get()
	defer defaultPool.Put(p)

	v, err := p.Parse(string(buf))
	if err != nil {
		return err
	}
	return t.unmarshalJSON(v)
}

func (t *Transaction) unmarshalJSON(v *fastjson.Value) error {
	var err error
	if err := decodeHash(&t.Hash, v, "hash"); err != nil {
		return err
	}
	if err = decodeAddr(&t.From, v, "from"); err != nil {
		return err
	}
	if t.GasPrice, err = decodeUint(v, "gasPrice"); err != nil {
		return err
	}
	if t.Gas, err = decodeUint(v, "gas"); err != nil {
		return err
	}
	if t.Input, err = decodeBytes(t.Input[:0], v, "input"); err != nil {
		return err
	}
	if t.Value, err = decodeBigInt(t.Value, v, "value"); err != nil {
		return err
	}
	if t.Nonce, err = decodeUint(v, "nonce"); err != nil {
		return err
	}
	return nil
}

// UnmarshalJSON implements the unmarshal interface
func (r *Receipt) UnmarshalJSON(buf []byte) error {
	p := defaultPool.Get()
	defer defaultPool.Put(p)

	v, err := p.Parse(string(buf))
	if err != nil {
		return nil
	}

	if err := decodeAddr(&r.From, v, "from"); err != nil {
		return err
	}
	if fieldNotFull(v, "contractAddress") {
		if err := decodeAddr(&r.ContractAddress, v, "contractAddress"); err != nil {
			return err
		}
	}
	if err := decodeHash(&r.TransactionHash, v, "transactionHash"); err != nil {
		return err
	}
	if err := decodeHash(&r.BlockHash, v, "blockHash"); err != nil {
		return err
	}
	if r.TransactionIndex, err = decodeUint(v, "transactionIndex"); err != nil {
		return err
	}
	if r.BlockNumber, err = decodeUint(v, "blockNumber"); err != nil {
		return err
	}
	if r.GasUsed, err = decodeUint(v, "gasUsed"); err != nil {
		return err
	}
	if r.CumulativeGasUsed, err = decodeUint(v, "cumulativeGasUsed"); err != nil {
		return err
	}
	if r.LogsBloom, err = decodeBytes(r.LogsBloom[:0], v, "logsBloom", 256); err != nil {
		return err
	}

	// logs
	r.Logs = r.Logs[:0]
	for _, elem := range v.GetArray("logs") {
		log := new(Log)
		if err := log.unmarshalJSON(elem); err != nil {
			return err
		}
		r.Logs = append(r.Logs, log)
	}

	return nil
}

// UnmarshalJSON implements the unmarshal interface
func (r *Log) UnmarshalJSON(buf []byte) error {
	p := defaultPool.Get()
	defer defaultPool.Put(p)

	v, err := p.Parse(string(buf))
	if err != nil {
		return nil
	}
	return r.unmarshalJSON(v)
}

func (r *Log) unmarshalJSON(v *fastjson.Value) error {
	var err error
	if r.Removed, err = decodeBool(v, "removed"); err != nil {
		return err
	}
	if r.LogIndex, err = decodeUint(v, "logIndex"); err != nil {
		return err
	}
	if r.BlockNumber, err = decodeUint(v, "blockNumber"); err != nil {
		return err
	}
	if r.TransactionIndex, err = decodeUint(v, "transactionIndex"); err != nil {
		return err
	}
	if err := decodeHash(&r.TransactionHash, v, "transactionHash"); err != nil {
		return err
	}
	if err := decodeHash(&r.BlockHash, v, "blockHash"); err != nil {
		return err
	}
	if err := decodeAddr(&r.Address, v, "address"); err != nil {
		return err
	}
	if r.Data, err = decodeBytes(r.Data[:0], v, "data"); err != nil {
		return err
	}

	r.Topics = r.Topics[:0]
	for _, topic := range v.GetArray("topics") {
		var t Hash
		b, err := topic.StringBytes()
		if err != nil {
			return err
		}
		if err := t.UnmarshalText(b); err != nil {
			return err
		}
		r.Topics = append(r.Topics, t)
	}
	return nil
}

func fieldNotFull(v *fastjson.Value, key string) bool {
	vv := v.Get(key)
	if vv == nil {
		return false
	}
	if vv.String() == "null" {
		return false
	}
	return true
}

func decodeBigInt(b *big.Int, v *fastjson.Value, key string) (*big.Int, error) {
	vv := v.Get(key)
	if vv == nil {
		return nil, fmt.Errorf("field '%s' not found", key)
	}
	str := vv.String()
	str = strings.Trim(str, "\"")

	if !strings.HasPrefix(str, "0x") {
		return nil, fmt.Errorf("field %s does not have 0x prefix", str)
	}
	if b == nil {
		b = new(big.Int)
	}

	var ok bool
	b, ok = b.SetString(str[2:], 16)
	if !ok {
		return nil, fmt.Errorf("failed to decode big int")
	}
	return b, nil
}

func decodeBytes(dst []byte, v *fastjson.Value, key string, bits ...int) ([]byte, error) {
	vv := v.Get(key)
	if vv == nil {
		return nil, fmt.Errorf("field '%s' not found", key)
	}
	str := vv.String()
	str = strings.Trim(str, "\"")

	if !strings.HasPrefix(str, "0x") {
		return nil, fmt.Errorf("field %s does not have 0x prefix", str)
	}

	buf, err := hex.DecodeString(str[2:])
	if err != nil {
		return nil, err
	}
	if len(bits) > 0 && bits[0] != len(buf) {
		return nil, fmt.Errorf("field %s invalid length, expected %d but found %d", str, bits[0], len(buf))
	}
	dst = append(dst, buf...)
	return dst, nil
}

func decodeUint(v *fastjson.Value, key string) (uint64, error) {
	vv := v.Get(key)
	if vv == nil {
		return 0, fmt.Errorf("field '%s' not found", key)
	}
	str := vv.String()
	str = strings.Trim(str, "\"")

	if !strings.HasPrefix(str, "0x") {
		return 0, fmt.Errorf("field %s does not have 0x prefix", str)
	}
	return strconv.ParseUint(str[2:], 16, 64)
}

func decodeHash(h *Hash, v *fastjson.Value, key string) error {
	b := v.GetStringBytes(key)
	if len(b) == 0 {
		return fmt.Errorf("field '%s' not found", key)
	}
	h.UnmarshalText(b)
	return nil
}

func decodeAddr(a *Address, v *fastjson.Value, key string) error {
	b := v.GetStringBytes(key)
	if len(b) == 0 {
		return fmt.Errorf("field '%s' not found", key)
	}
	a.UnmarshalText(b)
	return nil
}

func decodeBool(v *fastjson.Value, key string) (bool, error) {
	vv := v.Get(key)
	if vv == nil {
		return false, fmt.Errorf("field '%s' not found", key)
	}
	str := vv.String()
	if str == "false" {
		return false, nil
	} else if str == "true" {
		return true, nil
	}
	return false, fmt.Errorf("field '%s' with content '%s' cannot be decoded as bool", key, str)
}

func unmarshalTextByte(dst, src []byte, size int) error {
	str := string(src)

	str = strings.Trim(str, "\"")
	if !strings.HasPrefix(str, "0x") {
		return fmt.Errorf("0x prefix not found")
	}
	str = str[2:]
	b, err := hex.DecodeString(str)
	if err != nil {
		return err
	}
	if len(b) != size {
		return fmt.Errorf("length %d is not correct, expected %d", len(b), size)
	}
	copy(dst, b)
	return nil
}
