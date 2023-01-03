package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/helper/invoker"
)

type Invoker struct {
	store txPoolStore
}

func (i *Invoker) SendInvokerTransaction(bufTrans, bufSig argBytes) (interface{}, error) {

	tx := &invoker.InvokerTransaction{}
	if err := tx.UnmarshalRLP(bufTrans); err != nil {
		return nil, err
	}

	// tx.InvokerHash(), tx.ComputeHash() ???

	sig := &invoker.InvokerSignature{}
	if err := sig.UnmarshalRLP(bufSig); err != nil {
		return nil, err
	}

	// TODO ???
	//if err := i.store.AddTx(tx, sig); err != nil {
	//	return nil, err
	//}

	//return tx.Hash.String(), nil

	return nil, nil
}
