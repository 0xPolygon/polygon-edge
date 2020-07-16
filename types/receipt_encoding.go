package types

import (
	"fmt"

	"github.com/umbracle/fastrlp"
)

// UnmarshalRLP unmarshals a Receipt in RLP format
func (r *Receipt) UnmarshalRLP(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) != 5 {
		return fmt.Errorf("expected 4 elements")
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
		return fmt.Errorf("bad root/status size %d", size)
	}

	if _, err = elems[1].GetBytes(r.TxHash[:0], 32); err != nil {
		return err
	}
	// cumulativeGasUsed
	if r.CumulativeGasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}
	// logsBloom
	if _, err = elems[3].GetBytes(r.LogsBloom[:0], 256); err != nil {
		return err
	}

	// logs
	logsElems, err := v.Get(4).GetElems()
	if err != nil {
		return err
	}
	for _, elem := range logsElems {
		log := &Log{}

		subElems, err := elem.GetElems()
		if err != nil {
			return err
		}
		if len(subElems) != 3 {
			return fmt.Errorf("bad elems")
		}

		// address
		if err := subElems[0].GetAddr(log.Address[:]); err != nil {
			return err
		}
		// topics
		topicElems, err := subElems[1].GetElems()
		if err != nil {
			return err
		}
		log.Topics = make([]Hash, len(topicElems))
		for indx, topic := range topicElems {
			if err := topic.GetHash(log.Topics[indx][:]); err != nil {
				return err
			}
		}
		// data
		if log.Data, err = subElems[2].GetBytes(log.Data[:0]); err != nil {
			return err
		}
		r.Logs = append(r.Logs, log)
	}
	return nil
}

// MarshalWith marshals a receipt with a specific fastrlp.Arena
func (r *Receipt) MarshalWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()
	if r.Status != nil {
		vv.Set(a.NewUint(uint64(*r.Status)))
	} else {
		vv.Set(a.NewBytes(r.Root[:]))
	}
	vv.Set(a.NewBytes(r.TxHash.Bytes()))
	vv.Set(a.NewUint(r.CumulativeGasUsed))
	vv.Set(a.NewCopyBytes(r.LogsBloom[:]))
	vv.Set(r.MarshalLogsWith(a))
	return vv
}

// MarshalLogsWith marshals the logs of the receipt to RLP with a specific fastrlp.Arena
func (r *Receipt) MarshalLogsWith(a *fastrlp.Arena) *fastrlp.Value {
	if len(r.Logs) == 0 {
		// There are no receipts, write the RLP null array entry
		return a.NewNullArray()
	}

	logs := a.NewArray()
	for _, l := range r.Logs {

		log := a.NewArray()
		log.Set(a.NewBytes(l.Address.Bytes()))

		topics := a.NewArray()
		for _, t := range l.Topics {
			topics.Set(a.NewBytes(t.Bytes()))
		}
		log.Set(topics)
		log.Set(a.NewBytes(l.Data))
		logs.Set(log)
	}
	return logs
}
