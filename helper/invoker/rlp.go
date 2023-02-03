package invoker

import (
	"fmt"
	"math/big"

	"github.com/umbracle/fastrlp"
)

var (
	invokerSignatureParserPool,
	invokerTransactionParserPool,
	transactionPayloadParserPool fastrlp.ParserPool
)

func (is *InvokerSignature) UnmarshalRLP(b []byte) error {
	pool := invokerSignatureParserPool.Get()
	defer invokerSignatureParserPool.Put(pool)

	parsed, err := pool.Parse(b)
	if err != nil {
		return err
	}

	elems, err := parsed.GetElems()

	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode invoker signature, expected 3 but found %d", len(elems))
	}

	if is.R == nil {
		is.R = new(big.Int)
	}

	if err = elems[0].GetBigInt(is.R); err != nil {
		return err
	}

	if is.S == nil {
		is.S = new(big.Int)
	}

	if err = elems[1].GetBigInt(is.S); err != nil {
		return err
	}

	if is.V, err = elems[2].GetBool(); err != nil {
		return err
	}

	return nil
}

func (it *InvokerTransaction) UnmarshalRLP(b []byte) error {
	pool := invokerTransactionParserPool.Get()
	defer invokerTransactionParserPool.Put(pool)

	parsed, err := pool.Parse(b)
	if err != nil {
		return err
	}

	elems, err := parsed.GetElems()

	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode invoker transaction, expected 3 but found %d", len(elems))
	}

	var fromBytes []byte
	if err = elems[0].GetAddr(fromBytes); err != nil {
		return err
	}
	copy(it.From[:], fromBytes)

	if it.Nonce == nil {
		it.Nonce = new(big.Int)
	}

	if err = elems[1].GetBigInt(it.Nonce); err != nil {
		return err
	}

	var payloads []*fastrlp.Value
	payloads, err = elems[2].GetElems()
	if err != nil {
		return err
	}

	for _, p := range payloads {
		var bytes []byte
		bytes, err = p.Bytes()
		if err != nil {
			return err
		}

		payload := TransactionPayload{}

		if err = payload.UnmarshalRLP(bytes); err != nil {
			return err
		}

		it.Payloads = append(it.Payloads, payload)
	}

	return nil
}

func (tp *TransactionPayload) UnmarshalRLP(b []byte) error {

	pool := transactionPayloadParserPool.Get()
	defer transactionPayloadParserPool.Put(pool)

	parsed, err := pool.Parse(b)
	if err != nil {
		return err
	}

	elems, err := parsed.GetElems()

	if err != nil {
		return err
	}

	if len(elems) < 4 {
		return fmt.Errorf("incorrect number of elements to decode transaction payload, expected 4 but found %d", len(elems))
	}

	var fromBytes []byte
	if err = elems[0].GetAddr(fromBytes); err != nil {
		return err
	}
	copy(tp.To[:], fromBytes)

	if tp.Value == nil {
		tp.Value = new(big.Int)
	}

	if err = elems[1].GetBigInt(tp.Value); err != nil {
		return err
	}

	if tp.GasLimit == nil {
		tp.GasLimit = new(big.Int)
	}

	if err = elems[2].GetBigInt(tp.GasLimit); err != nil {
		return err
	}

	if tp.Data, err = elems[3].Bytes(); err != nil {
		return err
	}

	return nil
}
