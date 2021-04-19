package ibft

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
)

func commitMsg(b []byte) []byte {
	// message that the nodes need to sign to commit to a block
	return crypto.Keccak256(b, []byte{byte(2)})
}

func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}
	return crypto.PubKeyToAddress(pub), nil
}

func ecrecoverFromHeader(h *types.Header) (types.Address, error) {
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

func signSealImpl(prv *ecdsa.PrivateKey, h *types.Header, committed bool) ([]byte, error) {
	sig, err := signHash(h)
	if err != nil {
		return nil, err
	}

	// if we are singing the commited seals we need to do something more
	if committed {
		sig = commitMsg(sig)
	}
	seal, err := crypto.Sign(prv, crypto.Keccak256(sig))
	if err != nil {
		return nil, err
	}
	return seal, nil
}

func writeSeal(prv *ecdsa.PrivateKey, h *types.Header) (*types.Header, error) {
	h = h.Copy()
	seal, err := signSealImpl(prv, h, false)
	if err != nil {
		return nil, err
	}
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}
	extra.Seal = seal
	if err := PutIbftExtra(h, extra); err != nil {
		return nil, err
	}
	return h, nil
}

func writeCommittedSeal(prv *ecdsa.PrivateKey, h *types.Header) ([]byte, error) {
	return signSealImpl(prv, h, true)
}

func writeCommittedSeals(h *types.Header, seals [][]byte) (*types.Header, error) {
	h = h.Copy()

	if len(seals) == 0 {
		return nil, fmt.Errorf("bad")
	}

	for _, seal := range seals {
		if len(seal) != IstanbulExtraSeal {
			return nil, fmt.Errorf("bad")
		}
	}

	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}
	extra.CommittedSeal = seals
	if err := PutIbftExtra(h, extra); err != nil {
		return nil, err
	}
	return h, nil
}

func signHash(h *types.Header) ([]byte, error) {
	//hash := istambulHeaderHash(h)
	//return hash.Bytes(), nil

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

func verifySigner(snap *Snapshot, header *types.Header) error {
	signer, err := ecrecoverFromHeader(header)
	if err != nil {
		return err
	}
	if !snap.Set.Includes(signer) {
		return fmt.Errorf("not found signer")
	}
	return nil
}

func verifyCommitedFields(snap *Snapshot, header *types.Header) error {
	extra, err := getIbftExtra(header)
	if err != nil {
		return err
	}
	if len(extra.CommittedSeal) == 0 {
		return fmt.Errorf("empty committed seals")
	}

	// get the message that needs to be signed
	signMsg, err := signHash(header)
	if err != nil {
		return err
	}
	signMsg = commitMsg(signMsg)

	visited := map[types.Address]struct{}{}
	for _, seal := range extra.CommittedSeal {
		addr, err := ecrecoverImpl(seal, signMsg)
		if err != nil {
			return err
		}

		if _, ok := visited[addr]; ok {
			return fmt.Errorf("repeated seal")
		} else {
			if !snap.Set.Includes(addr) {
				return fmt.Errorf("signed by non validator")
			}
			visited[addr] = struct{}{}
		}
	}

	validSeals := len(visited)
	if validSeals <= 2*snap.Set.MinFaultyNodes() {
		return fmt.Errorf("not enough seals to seal block")
	}
	return nil
}

func validateMsg(msg *proto.MessageReq) error {
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}
	buf, err := hex.DecodeHex(msg.Signature)
	if err != nil {
		return err
	}
	addr, err := ecrecoverImpl(buf, signMsg)
	if err != nil {
		return err
	}
	msg.From = addr.String()
	return nil
}

func signMsg(key *ecdsa.PrivateKey, msg *proto.MessageReq) error {
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		return err
	}
	sig, err := crypto.Sign(key, crypto.Keccak256(signMsg))
	if err != nil {
		return err
	}
	msg.Signature = hex.EncodeToHex(sig)
	return nil
}
