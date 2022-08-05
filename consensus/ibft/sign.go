package ibft

import (
	"crypto/ecdsa"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

const (
	// legacyCommitCode is the value that is contained in
	// legacy committed seals, so it needs to be preserved in order
	// for new clients to read old committed seals
	legacyCommitCode = 2
)

var (
	ErrEmptyCommittedSeals        = errors.New("empty committed seals")
	ErrInvalidCommittedSealLength = errors.New("invalid committed seal length")
	ErrRepeatedCommittedSeal      = errors.New("repeated seal in committed seals")
	ErrNonValidatorCommittedSeal  = errors.New("found committed seal signed by non validator")
	ErrNotEnoughCommittedSeals    = errors.New("not enough seals to seal block")
	ErrSignerNotFound             = errors.New("not found signer in validator set")
)

func wrapCommitHash(b []byte) []byte {
	return crypto.Keccak256(b, []byte{byte(legacyCommitCode)})
}

func ecrecoverImpl(sig, msg []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(msg))
	if err != nil {
		return types.Address{}, err
	}

	return crypto.PubKeyToAddress(pub), nil
}

func ecrecoverProposer(h *types.Header) (types.Address, error) {
	seal, err := unpackProposerSealFromIbftExtra(h)
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

func signSealImpl(prv *ecdsa.PrivateKey, hash []byte, committed bool) ([]byte, error) {
	// if we are singing the committed seals we need to do something more
	msg := hash
	if committed {
		msg = wrapCommitHash(hash)
	}

	return crypto.Sign(prv, crypto.Keccak256(msg))
}

// writeProposerSeal creates and sets a signature to Seal in IBFT Extra
func writeProposerSeal(prv *ecdsa.PrivateKey, h *types.Header) (*types.Header, error) {
	h = h.Copy()

	hash, err := calculateHeaderHash(h)
	if err != nil {
		return nil, err
	}

	seal, err := signSealImpl(prv, hash, false)

	if err != nil {
		return nil, err
	}

	if err := packProposerSealIntoIbftExtra(h, seal); err != nil {
		return nil, err
	}

	return h, nil
}

// createCommittedSeal returns the commit signature
func createCommittedSeal(prv *ecdsa.PrivateKey, proposalHash []byte) ([]byte, error) {
	return signSealImpl(prv, proposalHash, true)
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
	signer, err := ecrecoverProposer(header)
	if err != nil {
		return err
	}

	if !snap.Set.Includes(signer) {
		return ErrSignerNotFound
	}

	return nil
}

// verifyCommittedFields is checking for consensus proof in the header
func verifyCommittedFields(
	snap *Snapshot,
	header *types.Header,
	quorumSizeFn QuorumImplementation,
) error {
	extra, err := getIbftExtra(header)
	if err != nil {
		return err
	}

	// Committed seals shouldn't be empty
	if len(extra.CommittedSeal) == 0 {
		return fmt.Errorf("empty committed seals")
	}

	// get the message that needs to be signed
	// this not signing! just removing the fields that should be signed
	hash, err := calculateHeaderHash(header)
	if err != nil {
		return err
	}

	rawMsg := wrapCommitHash(hash)

	return verifyCommittedSealsImpl(extra.CommittedSeal, rawMsg, snap.Set, quorumSizeFn)
}

// verifyCommittedFields verifies ParentCommittedSeal in IBFT Extra of Header
func verifyParentCommittedSeal(
	parentSnap *Snapshot,
	parent, header *types.Header,
	quorumSizeFn QuorumImplementation,
) error {
	parentCommittedSeals, err := unpackParentCommittedSealFromIbftExtra(header)
	if err != nil {
		return err
	}

	hash, err := calculateHeaderHash(parent)
	if err != nil {
		return err
	}

	rawMsg := wrapCommitHash(hash)

	return verifyCommittedSealsImpl(parentCommittedSeals, rawMsg, parentSnap.Set, quorumSizeFn)
}

// verifyCommittedSealsImpl verifies given committedSeals
// checks committedSeals are the correct signatures signed by the validators and the number of seals
func verifyCommittedSealsImpl(
	committedSeals [][]byte,
	msg []byte,
	validators ValidatorSet,
	quorumSizeFn QuorumImplementation,
) error {
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
		}

		if !validators.Includes(addr) {
			return ErrNonValidatorCommittedSeal
		}

		visited[addr] = struct{}{}
	}

	// Valid committed seals must be at least 2F+1
	// 	2F 	is the required number of honest validators who provided the committed seals
	// 	+1	is the proposer
	if validSeals := len(visited); validSeals < quorumSizeFn(validators) {
		return fmt.Errorf("not enough seals to seal block")
	}

	return nil
}
