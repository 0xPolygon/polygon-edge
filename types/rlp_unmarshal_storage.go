package types

import (
	"errors"
	"fmt"

	"github.com/umbracle/fastrlp"
)

type RLPStoreUnmarshaler interface {
	UnmarshalStoreRLP(input []byte) error
}

// UnmarshalRLP unmarshals body from byte slice.
// Caution: Hash for each tx must be computed manually after!
func (b *Body) UnmarshalRLP(input []byte) error {
	return UnmarshalRlp(b.unmarshalRLPFrom, input)
}

func (b *Body) unmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	tuple, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(tuple) < 2 {
		return fmt.Errorf("incorrect number of elements to decode header, expected 2 but found %d", len(tuple))
	}

	// transactions
	if err = unmarshalRLPFrom(p, tuple[0], func(txType TxType, p *fastrlp.Parser, v *fastrlp.Value) error {
		bTxn := &Transaction{
			Type: txType,
		}

		if err = bTxn.unmarshalStoreRLPFrom(p, v); err != nil {
			return err
		}

		b.Transactions = append(b.Transactions, bTxn)

		return nil
	}); err != nil {
		return err
	}

	// uncles
	uncles, err := tuple[1].GetElems()
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

// UnmarshalStoreRLP unmarshals transaction from byte slice. Hash must be computed manually after!
func (t *Transaction) UnmarshalStoreRLP(input []byte) error {
	t.Type = LegacyTx

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if t.Type, err = txTypeFromByte(input[0]); err != nil {
			return err
		}
	}

	return UnmarshalRlp(t.unmarshalStoreRLPFrom, input)
}

func (t *Transaction) unmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	// come TransactionType first if exist
	if len(elems) != 2 && len(elems) != 3 {
		return fmt.Errorf("incorrect number of elements, expected 2 or 3 but found %d", len(elems))
	}

	if len(elems) == 3 {
		if err = t.Type.unmarshalRLPFrom(p, elems[0]); err != nil {
			return err
		}

		elems = elems[1:]
	}

	// consensus part
	if err = t.unmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// context part
	if err = elems[1].GetAddr(t.From[:]); err != nil {
		return err
	}

	return nil
}

func (r *Receipts) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.unmarshalStoreRLPFrom, input)
}

func (r *Receipts) unmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	return unmarshalRLPFrom(p, v, func(txType TxType, p *fastrlp.Parser, v *fastrlp.Value) error {
		obj := &Receipt{
			TransactionType: txType,
		}

		if err := obj.unmarshalStoreRLPFrom(p, v); err != nil {
			return err
		}

		*r = append(*r, obj)

		return nil
	})
}

func (r *Receipt) UnmarshalStoreRLP(input []byte) error {
	r.TransactionType = LegacyTx

	if len(input) > 0 && input[0] <= RLPSingleByteUpperLimit {
		var err error
		if r.TransactionType, err = txTypeFromByte(input[0]); err != nil {
			return err
		}
	}

	return UnmarshalRlp(r.unmarshalStoreRLPFrom, input)
}

func (r *Receipt) unmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 4 {
		return errors.New("expected at least 4 elements")
	}

	// come TransactionType first if exist
	if len(elems) == 5 {
		if err = r.TransactionType.unmarshalRLPFrom(p, elems[0]); err != nil {
			return err
		}

		elems = elems[1:]
	}

	if err = r.unmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// contract address
	vv, err := elems[1].Bytes()
	if err != nil {
		return err
	}

	if len(vv) == 20 {
		// address
		r.SetContractAddress(BytesToAddress(vv))
	}

	// gas used
	if r.GasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}

	// tx hash
	// backwards compatibility, old receipts did not marshal a TxHash
	if len(elems) == 4 {
		vv, err = elems[3].Bytes()
		if err != nil {
			return err
		}

		r.TxHash = BytesToHash(vv)
	}

	return nil
}
