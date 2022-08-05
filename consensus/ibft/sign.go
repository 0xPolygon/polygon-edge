package ibft

import (
	"crypto/ecdsa"
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
	// get the extra part that contains the seal
	extra, err := getIbftExtra(h)
	if err != nil {
		return types.Address{}, err
	}

	// Calculate the header hash (keccak of RLP)
	hash, err := calculateHeaderHash(h)
	if err != nil {
		return types.Address{}, err
	}

	return ecrecoverImpl(extra.ProposerSeal, hash)
}

func signSealImpl(prv *ecdsa.PrivateKey, h *types.Header) ([]byte, error) {
	hash, err := calculateHeaderHash(h)
	if err != nil {
		return nil, err
	}

	return crypto.Sign(prv, crypto.Keccak256(hash))
}

func writeProposerSeal(prv *ecdsa.PrivateKey, h *types.Header) (*types.Header, error) {
	h = h.Copy()
	seal, err := signSealImpl(prv, h)

	if err != nil {
		return nil, err
	}

	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}

	extra.ProposerSeal = seal
	if err := PutIbftExtra(h, extra); err != nil {
		return nil, err
	}

	return h, nil
}

// writeCommittedSeal generates the legacy committed seal using the passed in
// header hash and the private key
func writeCommittedSeal(prv *ecdsa.PrivateKey, headerHash []byte) ([]byte, error) {
	return crypto.Sign(
		prv,
		// Of course, this keccaking of an extended array is not according to the IBFT 2.0 spec,
		// but almost nothing in this legacy signing package is. This is kept
		// in order to preserve the running chains that used these
		// old (and very, very incorrect) signing schemes
		crypto.Keccak256(
			wrapCommitHash(headerHash),
		),
	)
}

func writeCommittedSeals(h *types.Header, seals [][]byte) (*types.Header, error) {
	h = h.Copy()

	if len(seals) == 0 {
		return nil, fmt.Errorf("empty committed seals")
	}

	for _, seal := range seals {
		if len(seal) != IstanbulExtraSeal {
			return nil, fmt.Errorf("invalid committed seal length")
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

func calculateHeaderHash(h *types.Header) ([]byte, error) {
	h = h.Copy() // make a copy since we update the extra field

	arena := fastrlp.DefaultArenaPool.Get()
	defer fastrlp.DefaultArenaPool.Put(arena)

	// when hashing the block for signing we have to remove from
	// the extra field the seal and committed seal items
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}

	// This will effectively remove the ProposerSeal and Committed ProposerSeal fields,
	// while keeping proposer vanity and validator set
	// because extra.Validators is what we got from `h` in the first place.
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
	signer, err := ecrecoverProposer(header)
	if err != nil {
		return err
	}

	if !snap.Set.Includes(signer) {
		return fmt.Errorf("not found signer")
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

	hash, err := calculateHeaderHash(header)
	if err != nil {
		return err
	}

	rawMsg := wrapCommitHash(hash)

	visited := map[types.Address]struct{}{}

	for _, seal := range extra.CommittedSeal {
		addr, err := ecrecoverImpl(seal, rawMsg)
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

	// Valid committed seals must be at least 2F+1
	// 2F 	is the required number of honest validators who provided the committed seals
	// +1	is the proposer
	if validSeals := len(visited); validSeals < quorumSizeFn(snap.Set) {
		return fmt.Errorf("not enough seals to seal block")
	}

	return nil
}
