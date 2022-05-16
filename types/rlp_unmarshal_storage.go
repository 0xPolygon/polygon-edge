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

	if len(tuple) < 2 {
		return fmt.Errorf("incorrect number of elements to decode header, expected 2 but found %d", len(tuple))
	}

	// transactions
	txns, err := tuple[0].GetElems()
	if err != nil {
		return err
	}

	for _, txn := range txns {
		bTxn := &Transaction{}
		if err := bTxn.UnmarshalStoreRLPFrom(p, txn); err != nil {
			return err
		}

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
	return UnmarshalRlp(t.UnmarshalStoreRLPFrom, input)
}

func (t *Transaction) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 2 {
		return fmt.Errorf("incorrect number of elements to decode transaction, expected 2 but found %d", len(elems))
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

func (r *Receipts) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalStoreRLPFrom, input)
}

func (r *Receipts) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	for _, elem := range elems {
		rr := &Receipt{}
		if err := rr.UnmarshalStoreRLPFrom(p, elem); err != nil {
			return err
		}

		(*r) = append(*r, rr)
	}

	return nil
}

func (r *Receipt) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalStoreRLPFrom, input)
}

func (r *Receipt) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode receipt, expected at least 3 but found %d", len(elems))
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
		r.SetContractAddress(BytesToAddress(vv))
	}

	// gas used
	if r.GasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}

	// tx hash
	// backwards compatibility, old receipts did not marshal a TxHash
	if len(elems) == 4 {
		vv, err := elems[3].Bytes()
		if err != nil {
			return err
		}

		r.TxHash = BytesToHash(vv)
	}

	return nil
}
