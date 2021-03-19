package ibft

import (
	"crypto/ecdsa"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
)

func commitMsg(h types.Hash) []byte {
	// message that the nodes need to sign to commit to a block
	return crypto.Keccak256(h.Bytes(), []byte{byte(2)})
}

func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}
	return crypto.PubKeyToAddress(pub), nil
}

func ecrecover(h *types.Header) (types.Address, error) {
	// get the extra part that contains the seal
	extra, err := getIbftExtra(h)
	if err != nil {
		return types.Address{}, err
	}
	// get the sig
	msg, err := signHash(h)
	if err != nil {
		return types.Address{}, err
	}
	return ecrecoverImpl(extra.Seal, msg)
}

func writeSeal(prv *ecdsa.PrivateKey, h *types.Header) (*types.Header, error) {
	h = h.Copy()
	sig, err := signHash(h)
	if err != nil {
		return nil, err
	}
	seal, err := crypto.Sign(prv, crypto.Keccak256(sig))
	if err != nil {
		return nil, err
	}
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}
	extra.Seal = seal
	if err := putIbftExtra(h, extra); err != nil {
		return nil, err
	}
	return h, nil
}

func signHash(h *types.Header) ([]byte, error) {
	h = h.Copy() // make a copy since we update the extra field

	arena := fastrlp.DefaultArenaPool.Get()
	defer fastrlp.DefaultArenaPool.Put(arena)

	// when hashign the block for signing we have to remove from
	// the extra field the seal and commitedseal items
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}
	putIbftExtraValidators(h, extra.Validators)

	vv := arena.NewArray()
	vv.Set(arena.NewBytes(h.ParentHash.Bytes()))
	vv.Set(arena.NewBytes(h.Sha3Uncles.Bytes()))
	vv.Set(arena.NewBytes(h.Miner.Bytes()))
	vv.Set(arena.NewBytes(h.StateRoot.Bytes()))
	vv.Set(arena.NewBytes(h.TxRoot.Bytes()))
	vv.Set(arena.NewBytes(h.ReceiptsRoot.Bytes()))
	vv.Set(arena.NewBytes(h.LogsBloom[:]))
	vv.Set(arena.NewUint(h.Difficulty))
	vv.Set(arena.NewUint(h.Number))
	vv.Set(arena.NewUint(h.GasLimit))
	vv.Set(arena.NewUint(h.GasUsed))
	vv.Set(arena.NewUint(h.Timestamp))
	vv.Set(arena.NewCopyBytes(h.ExtraData))

	buf := keccak.Keccak256Rlp(nil, vv)
	return buf, nil
}
