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

type unmarshalRLPFromFunc func(TxType, *fastrlp.Parser, *fastrlp.Value) error

func UnmarshalRlp(obj unmarshalRLPFunc, input []byte) error {
	pr := fastrlp.DefaultParserPool.Get()

	v, err := pr.Parse(input)
	if err != nil {
		fastrlp.DefaultParserPool.Put(pr)

		return err
	}

	if err = obj(pr, v); err != nil {
		fastrlp.DefaultParserPool.Put(pr)

		return err
	}

	fastrlp.DefaultParserPool.Put(pr)

	return nil
}

func unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value, cb unmarshalRLPFromFunc) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	for i := 0; i < len(elems); i++ {
		// Non-legacy tx raw contains a tx type prefix in the beginning according to EIP-2718.
		// Here we check if the first element is a tx type and unmarshal it first.
		txType := LegacyTx
		if elems[i].Type() == fastrlp.TypeBytes {
			if err = txType.unmarshalRLPFrom(p, elems[i]); err != nil {
				return err
			}

			// Then we increment element number in order to go to the actual tx data raw below.
			i++
		}

		if err = cb(txType, p, elems[i]); err != nil {
			return err
		}
	}

	return nil
}

func (t *TxType) unmarshalRLPFrom(_ *fastrlp.Parser, v *fastrlp.Value) error {
	bytes, err := v.Bytes()
	if err != nil {
		return err
	}

	if l := len(bytes); l != 1 {
		return fmt.Errorf("expected 1 byte transaction type, but size is %d", l)
	}

	tt, err := txTypeFromByte(bytes[0])
	if err != nil {
		return err
	}

	*t = tt

	return nil
}

func (b *Block) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(b.unmarshalRLPFrom, input)
}

func (b *Block) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode block, expected 3 but found %d", len(elems))
	}

	// header
	b.Header = &Header{}
	if err = b.Header.unmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// transactions
	if err = unmarshalRLPFrom(p, elems[1], func(txType TxType, p *fastrlp.Parser, v *fastrlp.Value) error {
		bTxn := &Transaction{
			Type: txType,
		}

		if err = bTxn.unmarshalRLPFrom(p, v); err != nil {
			return err
		}

		bTxn = bTxn.ComputeHash(b.Header.Number)

		b.Transactions = append(b.Transactions, bTxn)

		return nil
	}); err != nil {
		return err
	}

	// uncles
	uncles, err := elems[2].GetElems()
	if err != nil {
		return err
	}

	for _, uncle := range uncles {
		bUncle := &Header{}
		if err = bUncle.unmarshalRLPFrom(p, uncle); err != nil {
			return err
		}

		b.Uncles = append(b.Uncles, bUncle)
	}

	return nil
}

func (h *Header) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(h.unmarshalRLPFrom, input)
}

func (h *Header) unmarshalRLPFrom(_ *fastrlp.Parser, v *fastrlp.Value) error {
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
	return UnmarshalRlp(r.unmarshalRLPFrom, input)
}

func (r *Receipts) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	return unmarshalRLPFrom(p, v, func(txType TxType, p *fastrlp.Parser, v *fastrlp.Value) error {
		obj := &Receipt{
			TransactionType: txType,
		}

		if err := obj.unmarshalRLPFrom(p, v); err != nil {
			return err
		}

		*r = append(*r, obj)

		return nil
	})
}

func (r *Receipt) UnmarshalRLP(input []byte) error {
	r.TransactionType = LegacyTx
	offset := 0

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if r.TransactionType, err = txTypeFromByte(input[0]); err != nil {
			return err
		}

		offset = 1
	}

	return UnmarshalRlp(r.unmarshalRLPFrom, input[offset:])
}

// unmarshalRLPFrom unmarshals a Receipt in RLP format
func (r *Receipt) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
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
		if err = log.unmarshalRLPFrom(p, elem); err != nil {
			return err
		}

		r.Logs = append(r.Logs, log)
	}

	return nil
}

func (l *Log) unmarshalRLPFrom(_ *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode log, expected 3 but found %d", len(elems))
	}

	// address
	if err = elems[0].GetAddr(l.Address[:]); err != nil {
		return err
	}

	// topics
	topicElems, err := elems[1].GetElems()
	if err != nil {
		return err
	}

	l.Topics = make([]Hash, len(topicElems))

	for indx, topic := range topicElems {
		if err = topic.GetHash(l.Topics[indx][:]); err != nil {
			return err
		}
	}

	// data
	if l.Data, err = elems[2].GetBytes(l.Data[:0]); err != nil {
		return err
	}

	return nil
}

// UnmarshalRLP unmarshals transaction from byte slice
// Caution: Hash calculation should be done from the outside!
func (t *Transaction) UnmarshalRLP(input []byte) error {
	t.Type = LegacyTx
	offset := 0

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if t.Type, err = txTypeFromByte(input[0]); err != nil {
			return err
		}

		offset = 1
	}

	if err := UnmarshalRlp(t.unmarshalRLPFrom, input[offset:]); err != nil {
		return err
	}

	return nil
}

// unmarshalRLPFrom unmarshals a Transaction in RLP format
// Be careful! This function does not de-serialize tx type, it assumes that t.Type is already set
// Hash calculation should also be done from the outside!
// Use UnmarshalRLP in most cases
func (t *Transaction) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
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

	// Load Chain ID for dynamic transactions
	if t.Type == DynamicFeeTx {
		t.ChainID = new(big.Int)
		if err = getElem().GetBigInt(t.ChainID); err != nil {
			return err
		}
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
