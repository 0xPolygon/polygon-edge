package types

import (
	"errors"
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
	txns, err := tuple[0].GetElems()
	if err != nil {
		return err
	}

	for i := 0; i < len(txns); i++ {
		txType := TxTypeLegacy

		if txns[i].Type() == fastrlp.TypeBytes {
			bytes, err := txns[i].Bytes()
			if err != nil {
				return err
			}

			if l := len(bytes); l != 1 {
				return fmt.Errorf("expected 1 byte transaction type, but size is %d", l)
			}

			if txType, err = ToTransactionType(bytes[0]); err != nil {
				return err
			}

			i++
		}

		bTxn := &Transaction{}
		if err := bTxn.UnmarshalStoreRLPFrom(p, txns[i]); err != nil {
			return err
		}

		bTxn.Type = txType
		bTxn.ComputeHash()

		b.Transactions = append(b.Transactions, bTxn)
	}

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

func (t *Transaction) UnmarshalStoreRLP(input []byte) error {
	txType := TxTypeLegacy

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if txType, err = ToTransactionType(input[0]); err != nil {
			return err
		}
	}

	if err := UnmarshalRlp(t.UnmarshalStoreRLPFrom, input); err != nil {
		return err
	}

	t.Type = txType

	return nil
}

func (t *Transaction) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) != 2 && len(elems) != 3 {
		return errors.New("expected 2 or 3 elements")
	}

	if len(elems) == 2 {
		// consensus part
		if err := t.UnmarshalRLPFrom(p, elems[0]); err != nil {
			return err
		}
		// context part
		if err = elems[1].GetAddr(t.From[:]); err != nil {
			return err
		}
	} else if len(elems) == 3 {
		// consensus part
		if elems[0].Type() != fastrlp.TypeBytes {
			return errors.New("expected ByteType")
		}

		bytes, err := elems[0].Bytes()
		if err != nil {
			return err
		}

		if l := len(bytes); l != 1 {
			return fmt.Errorf("expected 1 byte transaction type, but size is %d", l)
		}

		if t.Type, err = ToTransactionType(bytes[0]); err != nil {
			return err
		}

		if err := t.UnmarshalRLPFrom(p, elems[1]); err != nil {
			return err
		}

		// context part
		if err = elems[2].GetAddr(t.From[:]); err != nil {
			return err
		}
	}

	t.ComputeHash()

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

		rr := &Receipt{}
		if err := rr.UnmarshalStoreRLPFrom(p, elems[i]); err != nil {
			return err
		}

		rr.TransactionType = txType
		*r = append(*r, rr)
	}

	return nil
}

func (r *Receipt) UnmarshalStoreRLP(input []byte) error {
	txType := TxTypeLegacy

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if txType, err = ToTransactionType(input[0]); err != nil {
			return err
		}
	}

	if err := UnmarshalRlp(r.UnmarshalStoreRLPFrom, input); err != nil {
		return err
	}

	r.TransactionType = txType

	return nil
}

func (r *Receipt) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) != 3 {
		return errors.New("expected 3 elements")
	}

	if err := r.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	{
		// contract address
		vv, err := elems[1].Bytes()
		if err != nil {
			return err
		}
		if len(vv) == 20 {
			// address
			r.ContractAddress = BytesToAddress(vv)
		}
	}

	// gas used
	if r.GasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}

	return nil
}
