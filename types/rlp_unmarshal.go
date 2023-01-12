package types

import (
	"fmt"
	"math/big"

	"github.com/umbracle/fastrlp"
)

type RLPUnmarshaler interface {
	UnmarshalRLP(input []byte) error
}

type unmarshalRLPFunc func(p *fastrlp.Parser, v *fastrlp.Value) error

func UnmarshalRlp(obj unmarshalRLPFunc, input []byte) error {
	pr := fastrlp.DefaultParserPool.Get()

	v, err := pr.Parse(input)
	if err != nil {
		fastrlp.DefaultParserPool.Put(pr)

		return err
	}

	if err := obj(pr, v); err != nil {
		fastrlp.DefaultParserPool.Put(pr)

		return err
	}

	fastrlp.DefaultParserPool.Put(pr)

	return nil
}

func (b *Block) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(b.UnmarshalRLPFrom, input)
}

func (b *Block) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode block, expected 3 but found %d", len(elems))
	}

	// header
	b.Header = &Header{}
	if err := b.Header.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// transactions
	txns, err := elems[1].GetElems()
	if err != nil {
		return err
	}

	for _, txn := range txns {
		bTxn := &Transaction{}
		if err := bTxn.UnmarshalRLPFrom(p, txn); err != nil {
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
		if err := bUncle.UnmarshalRLPFrom(p, uncle); err != nil {
			return err
		}

		b.Uncles = append(b.Uncles, bUncle)
	}

	return nil
}

func (h *Header) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(h.UnmarshalRLPFrom, input)
}

func (h *Header) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 15 {
		return fmt.Errorf("incorrect number of elements to decode header, expected 15 but found %d", len(elems))
	}

	// parentHash
	if err = elems[0].GetHash(h.ParentHash[:]); err != nil {
		return err
	}
	// sha3uncles
	if err = elems[1].GetHash(h.Sha3Uncles[:]); err != nil {
		return err
	}
	// miner
	if h.Miner, err = elems[2].GetBytes(h.Miner[:]); err != nil {
		return err
	}
	// stateroot
	if err = elems[3].GetHash(h.StateRoot[:]); err != nil {
		return err
	}
	// txroot
	if err = elems[4].GetHash(h.TxRoot[:]); err != nil {
		return err
	}
	// receiptroot
	if err = elems[5].GetHash(h.ReceiptsRoot[:]); err != nil {
		return err
	}
	// logsBloom
	if _, err = elems[6].GetBytes(h.LogsBloom[:0], 256); err != nil {
		return err
	}
	// difficulty
	if h.Difficulty, err = elems[7].GetUint64(); err != nil {
		return err
	}
	// number
	if h.Number, err = elems[8].GetUint64(); err != nil {
		return err
	}
	// gasLimit
	if h.GasLimit, err = elems[9].GetUint64(); err != nil {
		return err
	}
	// gasused
	if h.GasUsed, err = elems[10].GetUint64(); err != nil {
		return err
	}
	// timestamp
	if h.Timestamp, err = elems[11].GetUint64(); err != nil {
		return err
	}
	// extraData
	if h.ExtraData, err = elems[12].GetBytes(h.ExtraData[:0]); err != nil {
		return err
	}
	// mixHash
	if err = elems[13].GetHash(h.MixHash[:0]); err != nil {
		return err
	}
	// nonce
	nonce, err := elems[14].GetUint64()
	if err != nil {
		return err
	}

	h.SetNonce(nonce)

	// compute the hash after the decoding
	h.ComputeHash()

	return err
}

func (r *Receipts) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalRLPFrom, input)
}

func (r *Receipts) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	for _, elem := range elems {
		rr := &Receipt{}
		if err := rr.UnmarshalRLPFrom(p, elem); err != nil {
			return err
		}

		(*r) = append(*r, rr)
	}

	return nil
}

func (r *Receipt) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom unmarshals a Receipt in RLP format
func (r *Receipt) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 4 {
		return fmt.Errorf("incorrect number of elements to decode receipt, expected 4 but found %d", len(elems))
	}

	// root or status
	buf, err := elems[0].Bytes()
	if err != nil {
		return err
	}

	switch size := len(buf); size {
	case 32:
		// root
		copy(r.Root[:], buf[:])
	case 1:
		// status
		r.SetStatus(ReceiptStatus(buf[0]))
	default:
		r.SetStatus(0)
	}

	// cumulativeGasUsed
	if r.CumulativeGasUsed, err = elems[1].GetUint64(); err != nil {
		return err
	}
	// logsBloom
	if _, err = elems[2].GetBytes(r.LogsBloom[:0], 256); err != nil {
		return err
	}

	// logs
	logsElems, err := v.Get(3).GetElems()
	if err != nil {
		return err
	}

	for _, elem := range logsElems {
		log := &Log{}
		if err := log.UnmarshalRLPFrom(p, elem); err != nil {
			return err
		}

		r.Logs = append(r.Logs, log)
	}

	// Type
	if len(elems) >= 5 {
		if r.TransactionType, err = ReadRlpTxType(elems[4]); err != nil {
			return err
		}
	}

	return nil
}

func (l *Log) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode log, expected 3 but found %d", len(elems))
	}

	// address
	if err := elems[0].GetAddr(l.Address[:]); err != nil {
		return err
	}
	// topics
	topicElems, err := elems[1].GetElems()
	if err != nil {
		return err
	}

	l.Topics = make([]Hash, len(topicElems))

	for indx, topic := range topicElems {
		if err := topic.GetHash(l.Topics[indx][:]); err != nil {
			return err
		}
	}

	// data
	if l.Data, err = elems[2].GetBytes(l.Data[:0]); err != nil {
		return err
	}

	return nil
}

func (t *Transaction) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(t.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom unmarshals a Transaction in RLP format
func (t *Transaction) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 10 {
		return fmt.Errorf("incorrect number of elements to decode transaction, expected 10 but found %d", len(elems))
	}

	p.Hash(t.Hash[:0], v)

	// Type
	i := 0
	if t.Type, err = ReadRlpTxType(v.Get(i)); err != nil {
		return err
	}

	// nonce
	i++
	if t.Nonce, err = v.Get(i).GetUint64(); err != nil {
		return err
	}

	// gasPrice
	i++
	t.GasPrice = new(big.Int)
	if err = v.Get(i).GetBigInt(t.GasPrice); err != nil {
		return err
	}

	if t.Type == DynamicGeeTx {
		// gasFeeCap
		i++
		t.GasFeeCap = new(big.Int)
		if err = v.Get(i).GetBigInt(t.GasFeeCap); err != nil {
			return err
		}

		// gasTipCap
		i++
		t.GasTipCap = new(big.Int)
		if err = v.Get(i).GetBigInt(t.GasTipCap); err != nil {
			return err
		}
	}

	// gas
	i++
	if t.Gas, err = v.Get(i).GetUint64(); err != nil {
		return err
	}

	// to
	i++
	t.To = nil
	if vv, _ := v.Get(i).Bytes(); len(vv) == 20 {
		// address
		addr := BytesToAddress(vv)
		t.To = &addr
	}

	// value
	i++
	t.Value = new(big.Int)
	if err = v.Get(i).GetBigInt(t.Value); err != nil {
		return err
	}

	// input
	i++
	if t.Input, err = v.Get(i).GetBytes(t.Input[:0]); err != nil {
		return err
	}

	// V
	i++
	t.V = new(big.Int)
	if err = v.Get(i).GetBigInt(t.V); err != nil {
		return err
	}

	// R
	i++
	t.R = new(big.Int)
	if err = v.Get(i).GetBigInt(t.R); err != nil {
		return err
	}

	// S
	i++
	t.S = new(big.Int)
	if err = v.Get(i).GetBigInt(t.S); err != nil {
		return err
	}

	if t.Type == StateTx {
		// We need to set From field for state transaction,
		// because we are using unique, predefined address, for sending such transactions
		// From
		i++
		t.From = ZeroAddress
		if vv, err := v.Get(i).Bytes(); err == nil && len(vv) == AddressLength {
			// address
			addr := BytesToAddress(vv)
			t.From = addr
		}
	}

	return nil
}

func ReadRlpTxType(rlpValue *fastrlp.Value) (TxType, error) {
	bytes, err := rlpValue.Bytes()
	if err != nil {
		return LegacyTx, err
	}

	if len(bytes) != 1 {
		return LegacyTx, fmt.Errorf("expected 1 byte transaction type, but size is %d", len(bytes))
	}

	b := TxType(bytes[0])

	switch b {
	case LegacyTx, StateTx, DynamicGeeTx:
		return b, nil
	default:
		return LegacyTx, fmt.Errorf("invalid tx type value: %d", bytes[0])
	}
}
