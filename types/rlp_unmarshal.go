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

	// basefee
	// In order to be backward compatible, the len should be checked before accessing the element
	if len(elems) > 15 {
		if h.BaseFee, err = elems[15].GetUint64(); err != nil {
			return err
		}
	}

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

// UnmarshalRLP unmarshals a Receipt in RLP format
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

	getElem := func() *fastrlp.Value {
		val := elems[0]
		elems = elems[1:]

		return val
	}

	var num int

	switch t.Type {
	case LegacyTx:
		num = 9
	case StateTx:
		num = 10
	case DynamicFeeTx:
		num = 12
	default:
		return fmt.Errorf("transaction type %d not found", t.Type)
	}

	if numElems := len(elems); numElems != num {
		return fmt.Errorf("incorrect number of transaction elements, expected %d but found %d", num, numElems)
	}

	p.Hash(t.Hash[:0], v)

	// Skipping Chain ID field since we don't support it (yet)
	// This is needed to be compatible with other EVM chains and have the same format.
	// Since we don't have a chain ID, just skip it here.
	if t.Type == DynamicFeeTx {
		_ = getElem()
	}

	// nonce
	if t.Nonce, err = getElem().GetUint64(); err != nil {
		return err
	}

	if t.Type == DynamicFeeTx {
		// gasTipCap
		t.GasTipCap = new(big.Int)
		if err = getElem().GetBigInt(t.GasTipCap); err != nil {
			return err
		}

		// gasFeeCap
		t.GasFeeCap = new(big.Int)
		if err = getElem().GetBigInt(t.GasFeeCap); err != nil {
			return err
		}
	} else {
		// gasPrice
		t.GasPrice = new(big.Int)
		if err = getElem().GetBigInt(t.GasPrice); err != nil {
			return err
		}
	}

	// gas
	if t.Gas, err = getElem().GetUint64(); err != nil {
		return err
	}
	// to
	if vv, _ := getElem().Bytes(); len(vv) == 20 {
		// address
		addr := BytesToAddress(vv)
		t.To = &addr
	} else {
		// reset To
		t.To = nil
	}
	// value
	t.Value = new(big.Int)
	if err = getElem().GetBigInt(t.Value); err != nil {
		return err
	}
	// input
	if t.Input, err = getElem().GetBytes(t.Input[:0]); err != nil {
		return err
	}

	// Skipping Access List field since we don't support it.
	// This is needed to be compatible with other EVM chains and have the same format.
	// Since we don't have access list, just skip it here.
	if t.Type == DynamicFeeTx {
		_ = getElem()
	}

	// V
	t.V = new(big.Int)
	if err = getElem().GetBigInt(t.V); err != nil {
		return err
	}

	// R
	t.R = new(big.Int)
	if err = getElem().GetBigInt(t.R); err != nil {
		return err
	}
	// S
	t.S = new(big.Int)
	if err = getElem().GetBigInt(t.S); err != nil {
		return err
	}

	if t.Type == StateTx {
		t.From = ZeroAddress

		// We need to set From field for state transaction,
		// because we are using unique, predefined address, for sending such transactions
		if vv, err := getElem().Bytes(); err == nil && len(vv) == AddressLength {
			// address
			t.From = BytesToAddress(vv)
		}
	}

	return nil
}
