package types

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

func CalculateHash(bytes []byte) types.Hash {
	hash := types.Hash{}

	kec := keccak.DefaultKeccakPool.Get()
	if _, err := kec.Write(bytes); err == nil {
		kec.Sum(hash[:0])
		keccak.DefaultKeccakPool.Put(kec)
	}

	return hash
}

type Checkpoint struct {
	Proposer        types.Address
	Start           uint64
	End             uint64
	RootHash        types.Hash // merkle root of the block hashes
	AccountRootHash types.Hash // merkle root of the validators
	ChainID         uint64     // Edge Chain ID
}

func (c *Checkpoint) Hash() types.Hash {
	return CalculateHash(c.Bytes())
}

func (c *Checkpoint) Bytes() []byte {
	return c.MarshalRLP()
}

func (c *Checkpoint) MarshalRLP() []byte {
	return c.MarshalRLPTo(nil)
}

func (c *Checkpoint) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(c.MarshalRLPWith, dst)
}

func (c *Checkpoint) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(c.Proposer.Bytes()))
	vv.Set(arena.NewUint(c.Start))
	vv.Set(arena.NewUint(c.End))
	vv.Set(arena.NewBytes(c.RootHash[:]))
	vv.Set(arena.NewBytes(c.AccountRootHash[:]))
	vv.Set(arena.NewUint(c.ChainID))

	return vv
}

func (c *Checkpoint) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(c.UnmarshalRLPFrom, input)
}

func (c *Checkpoint) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 6 {
		return fmt.Errorf("not enough elements to decode checkpoint, expected 6 but found %d", num)
	}

	// proposer
	if vv, _ := elems[0].Bytes(); len(vv) == types.AddressLength {
		c.Proposer = types.BytesToAddress(vv)
	} else {
		return fmt.Errorf("expected %d bytes data for proposer, but got %d bytes", types.AddressLength, len(vv))
	}

	// start
	if c.Start, err = elems[1].GetUint64(); err != nil {
		return err
	}

	// end
	if c.End, err = elems[2].GetUint64(); err != nil {
		return err
	}

	// roothash
	if vv, _ := elems[3].Bytes(); len(vv) == types.HashLength {
		c.RootHash = types.BytesToHash(vv)
	} else {
		return fmt.Errorf("expected %d bytes data for account root hash, but got %d bytes", types.HashLength, len(vv))
	}

	// account roothash
	if vv, _ := elems[4].Bytes(); len(vv) == types.HashLength {
		c.AccountRootHash = types.BytesToHash(vv)
	} else {
		return fmt.Errorf("expected %d bytes data for account root hash, but got %d bytes", types.HashLength, len(vv))
	}

	// chain ID
	if c.ChainID, err = elems[5].GetUint64(); err != nil {
		return err
	}

	return nil
}

type Ack struct {
	Epoch          uint64
	CheckpointHash types.Hash
	TxHash         types.Hash
}

func (a *Ack) Hash() types.Hash {
	return CalculateHash(a.Bytes())
}

func (a *Ack) Bytes() []byte {
	return a.MarshalRLP()
}

func (a *Ack) MarshalRLP() []byte {
	return a.MarshalRLPTo(nil)
}

func (a *Ack) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(a.MarshalRLPWith, dst)
}

func (a *Ack) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(a.Epoch))
	vv.Set(arena.NewBytes(a.CheckpointHash[:]))
	vv.Set(arena.NewBytes(a.TxHash[:]))

	return vv
}

func (a *Ack) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(a.UnmarshalRLPFrom, input)
}

func (a *Ack) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 3 {
		return fmt.Errorf("not enough elements to decode ack, expected 3 but found %d", num)
	}

	// epoch
	if a.Epoch, err = elems[0].GetUint64(); err != nil {
		return err
	}

	// checkpoint hash
	if vv, _ := elems[1].Bytes(); len(vv) == types.HashLength {
		a.CheckpointHash = types.BytesToHash(vv)
	} else {
		return fmt.Errorf("expected %d bytes data for account root hash, but got %d bytes", types.HashLength, len(vv))
	}

	// account roothash
	if vv, _ := elems[2].Bytes(); len(vv) == types.HashLength {
		a.TxHash = types.BytesToHash(vv)
	} else {
		return fmt.Errorf("expected %d bytes data for tx hash, but got %d bytes", types.HashLength, len(vv))
	}

	return nil
}

type NoAck struct {
	Epoch uint64
}

func (n *NoAck) Hash() types.Hash {
	return CalculateHash(n.Bytes())
}

func (n *NoAck) Bytes() []byte {
	return n.MarshalRLP()
}

func (n *NoAck) MarshalRLP() []byte {
	return n.MarshalRLPTo(nil)
}

func (n *NoAck) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(n.MarshalRLPWith, dst)
}

func (n *NoAck) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(n.Epoch))

	return vv
}

func (n *NoAck) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(n.UnmarshalRLPFrom, input)
}

func (n *NoAck) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 1 {
		return fmt.Errorf("not enough elements to decode no-ack, expected 1 but found %d", num)
	}

	// epoch
	if n.Epoch, err = elems[0].GetUint64(); err != nil {
		return err
	}

	return nil
}
