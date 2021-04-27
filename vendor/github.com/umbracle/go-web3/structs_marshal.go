package web3

import (
	"encoding/hex"
	"fmt"

	"github.com/valyala/fastjson"
)

var defaultArena fastjson.ArenaPool

// MarshalJSON implements the marshal interface
func (l *Log) MarshalJSON() ([]byte, error) {
	a := defaultArena.Get()

	o := a.NewObject()
	if l.Removed {
		o.Set("removed", a.NewTrue())
	} else {
		o.Set("removed", a.NewFalse())
	}
	o.Set("logIndex", a.NewString(fmt.Sprintf("0x%x", l.LogIndex)))
	o.Set("transactionIndex", a.NewString(fmt.Sprintf("0x%x", l.TransactionIndex)))
	o.Set("transactionHash", a.NewString(l.TransactionHash.String()))
	o.Set("blockHash", a.NewString(l.BlockHash.String()))
	o.Set("blockNumber", a.NewString(fmt.Sprintf("0x%x", l.BlockNumber)))
	o.Set("address", a.NewString(l.Address.String()))
	o.Set("data", a.NewString("0x"+hex.EncodeToString(l.Data)))

	vv := a.NewArray()
	for indx, topic := range l.Topics {
		vv.SetArrayItem(indx, a.NewString(topic.String()))
	}
	o.Set("topics", vv)

	res := o.MarshalTo(nil)
	defaultArena.Put(a)
	return res, nil
}

// MarshalJSON implements the marshal interface
func (t *Block) MarshalJSON() ([]byte, error) {
	a := defaultArena.Get()

	o := a.NewObject()
	o.Set("number", a.NewString(fmt.Sprintf("0x%x", t.Number)))
	o.Set("hash", a.NewString(t.Hash.String()))
	o.Set("parentHash", a.NewString(t.ParentHash.String()))
	o.Set("sha3Uncles", a.NewString(t.Sha3Uncles.String()))
	o.Set("transactionsRoot", a.NewString(t.TransactionsRoot.String()))
	o.Set("stateRoot", a.NewString(t.StateRoot.String()))
	o.Set("receiptsRoot", a.NewString(t.ReceiptsRoot.String()))
	o.Set("miner", a.NewString(t.Miner.String()))
	o.Set("gasLimit", a.NewString(fmt.Sprintf("0x%x", t.GasLimit)))
	o.Set("gasUsed", a.NewString(fmt.Sprintf("0x%x", t.GasUsed)))
	o.Set("timestamp", a.NewString(fmt.Sprintf("0x%x", t.Timestamp)))
	o.Set("difficulty", a.NewString(fmt.Sprintf("0x%x", t.Difficulty)))
	o.Set("extraData", a.NewString("0x"+hex.EncodeToString(t.ExtraData)))

	// uncles
	if len(t.Uncles) != 0 {
		uncles := a.NewArray()
		for indx, uncle := range t.Uncles {
			uncles.SetArrayItem(indx, a.NewString(uncle.String()))
		}
		o.Set("uncles", uncles)
	}

	res := o.MarshalTo(nil)
	defaultArena.Put(a)
	return res, nil
}

// MarshalJSON implements the Marshal interface.
func (t *Transaction) MarshalJSON() ([]byte, error) {
	a := defaultArena.Get()

	o := a.NewObject()
	o.Set("hash", a.NewString(t.Hash.String()))
	o.Set("from", a.NewString(t.From.String()))
	if t.To != nil {
		o.Set("to", a.NewString(t.To.String()))
	}
	if len(t.Input) != 0 {
		o.Set("input", a.NewString("0x"+hex.EncodeToString(t.Input)))
	}
	if t.Value != nil {
		o.Set("value", a.NewString(fmt.Sprintf("0x%x", t.Value)))
	}
	o.Set("gasPrice", a.NewString(fmt.Sprintf("0x%x", t.GasPrice)))
	o.Set("gas", a.NewString(fmt.Sprintf("0x%x", t.Gas)))
	if t.Nonce != 0 {
		// we can remove this once we include support for custom nonces
		o.Set("nonce", a.NewString(fmt.Sprintf("0x%x", t.Nonce)))
	}

	o.Set("v", a.NewString("0x"+hex.EncodeToString(t.V)))
	o.Set("r", a.NewString("0x"+hex.EncodeToString(t.R)))
	o.Set("s", a.NewString("0x"+hex.EncodeToString(t.R)))

	o.Set("blockHash", a.NewString(t.BlockHash.String()))
	o.Set("blockNumber", a.NewString(fmt.Sprintf("0x%x", t.BlockNumber)))
	o.Set("transactionIndex", a.NewString(fmt.Sprintf("0x%x", t.TxnIndex)))

	res := o.MarshalTo(nil)
	defaultArena.Put(a)
	return res, nil
}

// MarshalJSON implements the Marshal interface.
func (c *CallMsg) MarshalJSON() ([]byte, error) {
	a := defaultArena.Get()

	o := a.NewObject()
	o.Set("from", a.NewString(c.From.String()))
	o.Set("to", a.NewString(c.To.String()))
	if len(c.Data) != 0 {
		o.Set("data", a.NewString("0x"+hex.EncodeToString(c.Data)))
	}
	if c.GasPrice != 0 {
		o.Set("gasPrice", a.NewString(fmt.Sprintf("0x%x", c.GasPrice)))
	}
	if c.Value != nil {
		o.Set("value", a.NewString(fmt.Sprintf("0x%x", c.Value)))
	}

	res := o.MarshalTo(nil)
	defaultArena.Put(a)
	return res, nil
}

// MarshalJSON implements the Marshal interface.
func (l *LogFilter) MarshalJSON() ([]byte, error) {
	a := defaultArena.Get()

	o := a.NewObject()
	if len(l.Address) == 1 {
		o.Set("address", a.NewString(l.Address[0].String()))
	} else if len(l.Address) > 1 {
		v := a.NewArray()
		for indx, addr := range l.Address {
			v.SetArrayItem(indx, a.NewString(addr.String()))
		}
	}

	v := a.NewArray()
	for indx, topic := range l.Topics {
		if topic == nil {
			v.SetArrayItem(indx, a.NewNull())
		} else {
			v.SetArrayItem(indx, a.NewString(topic.String()))
		}
	}
	o.Set("topics", v)

	if l.BlockHash != nil {
		o.Set("blockhash", a.NewString((*l.BlockHash).String()))
	}
	if l.From != nil {
		o.Set("fromBlock", a.NewString((*l.From).String()))
	}
	if l.To != nil {
		o.Set("toBlock", a.NewString((*l.To).String()))
	}

	res := o.MarshalTo(nil)
	defaultArena.Put(a)
	return res, nil
}
