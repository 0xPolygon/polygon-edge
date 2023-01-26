package jsonrpc

import (
	"errors"
)

type Invoker struct {
}

func (i *Invoker) SendInvokerTransaction(bufTrans, bufSig argBytes) (interface{}, error) {
	return nil, errors.New("not implemented")
}
