package ibft

import (
	"crypto/ecdsa"
	"errors"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var (
	ErrEmptyCommittedSeals        = errors.New("empty committed seals")
	ErrInvalidCommittedSealLength = errors.New("invalid committed seal length")
	ErrRepeatedCommittedSeal      = errors.New("repeated seal in committed seals")
	ErrNonValidatorCommittedSeal  = errors.New("found committed seal signed by non validator")
	ErrNotEnoughCommittedSeals    = errors.New("not enough seals to seal block")
	ErrSignerNotFound             = errors.New("not found signer in validator set")
)

func commitMsg(b []byte) []byte {
	// message that the nodes need to sign to commit to a block
	// hash with COMMIT_MSG_CODE which is the same value used in quorum
	return crypto.Keccak256(b, []byte{byte(proto.MessageReq_Commit)})
}

func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}

	return crypto.PubKeyToAddress(pub), nil
}

func ecrecoverFromHeader(h *types.Header) (types.Address, error) {
	seal, err := unpackSealFromIbftExtra(h)
	if err != nil {
		return types.Address{}, err
	}

	// get the sig
	msg, err := calculateHeaderHash(h)
	if err != nil {
		return types.Address{}, err
	}

	return ecrecoverImpl(seal, msg)
}

func signSealImpl(prv *ecdsa.PrivateKey, h *types.Header, committed bool) ([]byte, error) {
	hash, err := calculateHeaderHash(h)
	if err != nil {
		return nil, err
	}

	// if we are singing the committed seals we need to do something more
	msg := hash
	if committed {
		msg = commitMsg(hash)
	}

	seal, err := crypto.Sign(prv, crypto.Keccak256(msg))

	if err != nil {
		return nil, err
	}

	return seal, nil
}

// writeSeal creates and sets a signatures to Seal in IBFT Extra
func writeSeal(prv *ecdsa.PrivateKey, h *types.Header) (*types.Header, error) {
	h = h.Copy()
	seal, err := signSealImpl(prv, h, false)

	if err != nil {
		return nil, err
	}

	if err := packSealIntoIbftExtra(h, seal); err != nil {
		return nil, err
	}

	return h, nil
}

// getCommittedSeal returns the commit signature
func getCommittedSeal(prv *ecdsa.PrivateKey, h *types.Header) ([]byte, error) {
	return signSealImpl(prv, h, true)
}

// writeCommittedSeals sets given commit signatures to CommittedSeal in IBFT Extra
func writeCommittedSeals(h *types.Header, seals [][]byte) (*types.Header, error) {
	h = h.Copy()

	if len(seals) == 0 {
		return nil, ErrEmptyCommittedSeals
	}

	for _, seal := range seals {
		if len(seal) != IstanbulExtraSeal {
			return nil, ErrInvalidCommittedSealLength
		}
	}

	if err := packCommittedSealIntoIbftExtra(h, seals); err != nil {
		return nil, err
	}

	return h, nil
}

func calculateHeaderHash(h *types.Header) ([]byte, error) {
	h = h.Copy() // make a copy since we update the extra field

	arena := fastrlp.DefaultArenaPool.Get()
	defer fastrlp.DefaultArenaPool.Put(arena)

	if err := filterIbftExtraForHash(h); err != nil {
		return nil, err
	}

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
		return ErrSignerNotFound
	}

	return nil
}

// verifyCommittedFields verifies CommittedSeal in IBFT Extra of Header
func verifyCommittedSeal(snap *Snapshot, header *types.Header) error {
	committedSeal, err := unpackCommittedSealFromIbftExtra(header)
	if err != nil {
		return err
	}

	// get the message that needs to be signed
	// this not signing! just removing the fields that should be signed
	hash, err := calculateHeaderHash(header)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash)

	return verifyCommittedSealsImpl(committedSeal, rawMsg, snap.Set)
}

// verifyCommittedFields verifies ParentCommittedSeal in IBFT Extra of Header
func verifyParentCommittedSeal(parentSnap *Snapshot, parent, header *types.Header) error {
	parentCommittedSeals, err := unpackParentCommittedSealFromIbftExtra(header)
	if err != nil {
		return err
	}

	hash, err := calculateHeaderHash(parent)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash)

	return verifyCommittedSealsImpl(parentCommittedSeals, rawMsg, parentSnap.Set)
}

// verifyCommittedSealsImpl verifies given committedSeals
// checks committedSeals are the correct signatures signed by the validators and the number of seals
func verifyCommittedSealsImpl(committedSeals [][]byte, msg []byte, validators ValidatorSet) error {
	// Committed seals shouldn't be empty
	if len(committedSeals) == 0 {
		return ErrEmptyCommittedSeals
	}

	visited := map[types.Address]struct{}{}

	for _, seal := range committedSeals {
		addr, err := ecrecoverImpl(seal, msg)
		if err != nil {
			return err
		}

		if _, ok := visited[addr]; ok {
			return ErrRepeatedCommittedSeal
		} else {
			if !validators.Includes(addr) {
				return ErrNonValidatorCommittedSeal
			}
			visited[addr] = struct{}{}
		}
	}

	// Valid committed seals must be at least 2F+1
	// 	2F 	is the required number of honest validators who provided the committed seals
	// 	+1	is the proposer
	if validSeals := len(visited); validSeals < validators.QuorumSize() {
		return ErrNotEnoughCommittedSeals
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
