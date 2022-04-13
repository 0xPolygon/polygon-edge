package types

import (
	"fmt"

	"github.com/umbracle/fastrlp"
)

type RLPStoreUnmarshaler interface {
	UnmarshalStoreRLP(input []byte) error
}

func (b *Body) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(b.UnmarshalRLPFrom, input)
}

func (b *Body) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	tuple, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(tuple) != 2 {
		return fmt.Errorf("not enough elements to decode header, expected 15 but found %d", len(tuple))
	}

	// transactions
	var txns Transactions
	if err := txns.UnmarshalStoreRLPFrom(p, tuple[0]); err != nil {
		return err
	}

	b.Transactions = txns

	// uncles
	uncles, err := tuple[1].GetElems()
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

func (tt *Transactions) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	txns, err := v.GetElems()
	if err != nil {
		return err
	}

	for i := 0; i < len(txns); i++ {
		txType := TxTypeLegacy
		if txns[i].Type() == fastrlp.TypeBytes {
			if err := txType.UnmarshalRLPFrom(p, txns[i]); err != nil {
				return err
			}

			i++
		}

		txn := &Transaction{}

		if txn.Payload, err = newTxPayload(txType); err != nil {
			return err
		}

		if err := txn.Payload.UnmarshalStoreRLPFrom(p, txns[i]); err != nil {
			return err
		}

		txn.ComputeHash()

		*tt = append(*tt, txn)
	}

	return nil
}

func (t *Transaction) UnmarshalStoreRLP(input []byte) error {
	txType := TxTypeLegacy
	offset := 0

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if txType, err = ToTransactionType(input[0]); err != nil {
			return err
		}

		offset += 1
	}

	var err error
	if t.Payload, err = newTxPayload(txType); err != nil {
		return err
	}

	if err := UnmarshalRlp(t.UnmarshalStoreRLPFrom, input[offset:]); err != nil {
		return err
	}

	return nil
}

func (t *Transaction) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	if err := t.Payload.UnmarshalStoreRLPFrom(p, v); err != nil {
		return err
	}

	t.ComputeHash()

	return nil
}

func (t *LegacyTransaction) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 2 {
		return fmt.Errorf("not enough elements to decode transaction, expected 2 but found %d", num)
	}

	// consensus part
	if err := t.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// context part
	if err = elems[1].GetAddr(t.From[:]); err != nil {
		return err
	}

	return nil
}

func (t *StateTransaction) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 1 {
		return fmt.Errorf("not enough elements to decode transaction, expected 1 but found %d", num)
	}

	// consensus part
	if err := t.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// context part

	return nil
}

func (r *Receipts) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalStoreRLPFrom, input)
}

func (r *Receipts) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	for i := 0; i < len(elems); i++ {
		txType := TxTypeLegacy
		if elems[i].Type() == fastrlp.TypeBytes {
			if err := txType.UnmarshalRLPFrom(p, elems[i]); err != nil {
				return err
			}

			i++
		}

		rr := &Receipt{
			TransactionType: txType,
		}
		if err := rr.UnmarshalStoreRLPFrom(p, elems[i]); err != nil {
			return err
		}

		*r = append(*r, rr)
	}

	return nil
}

func (r *Receipt) UnmarshalStoreRLP(input []byte) error {
	txType := TxTypeLegacy
	offset := 0

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if txType, err = ToTransactionType(input[0]); err != nil {
			return err
		}

		offset += 1
	}

	r.TransactionType = txType
	if err := UnmarshalRlp(r.UnmarshalStoreRLPFrom, input[offset:]); err != nil {
		return err
	}

	return nil
}

func (r *Receipt) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 3 {
		return fmt.Errorf("not enough elements to decode receipt, expected 3 but found %d", num)
	}

	if err := r.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// contract address
	vv, err := elems[1].Bytes()
	if err != nil {
		return err
	}

	if len(vv) == 20 {
		// address
		r.ContractAddress = BytesToAddress(vv)
	}

	// gas used
	if r.GasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}

	return nil
}
