package signer

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ECDSASigner struct {
	key     *ecdsa.PrivateKey
	address types.Address
}

type SerializedSeal [][]byte

func NewECDSASigner(manager secrets.SecretsManager) (Signer, error) {
	key, err := obtainOrCreateECDSAKey(manager)
	if err != nil {
		return nil, err
	}

	return NewECDSASignerFromKey(key), nil
}

func NewECDSASignerFromKey(key *ecdsa.PrivateKey) Signer {
	return &ECDSASigner{
		key:     key,
		address: crypto.PubKeyToAddress(&key.PublicKey),
	}
}

func NewMockECDSASigner(addr types.Address) Signer {
	return &ECDSASigner{
		address: addr,
	}
}

func (s *ECDSASigner) Address() types.Address {
	return s.address
}

func (s *ECDSASigner) InitIBFTExtra(header, parent *types.Header, set validators.ValidatorSet) error {
	var parentCommittedSeal Sealer

	if parent.Number >= 1 {
		parentExtra, err := s.GetIBFTExtra(parent)
		if err != nil {
			return err
		}

		parentCommittedSeal = parentExtra.CommittedSeal
	}

	s.initIbftExtra(header, set, parentCommittedSeal)

	return nil
}

func (s *ECDSASigner) GetIBFTExtra(h *types.Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		// return nil, fmt.Errorf(
		// 	"wrong extra size, expected greater than or equal to %d but actual %d",
		// 	IstanbulExtraVanity,
		// 	len(h.ExtraData),
		// )
		err := fmt.Errorf(
			"wrong extra size, expected greater than or equal to %d but actual %d",
			IstanbulExtraVanity,
			len(h.ExtraData),
		)

		panic(err)
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		Validators:          &validators.ECDSAValidatorSet{},
		CommittedSeal:       &SerializedSeal{},
		ParentCommittedSeal: &SerializedSeal{},
	}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

func (s *ECDSASigner) WriteSeal(header *types.Header) (*types.Header, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	seal, err := crypto.Sign(s.key, crypto.Keccak256(hash[:]))
	if err != nil {
		return nil, err
	}

	err = s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.Seal = seal
	})

	if err != nil {
		return nil, err
	}

	return header, nil
}

func (s *ECDSASigner) EcrecoverFromHeader(header *types.Header) (types.Address, error) {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return types.Address{}, err
	}

	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return types.Address{}, err
	}

	return ecrecoverImpl(extra.Seal, hash[:])
}

func (s *ECDSASigner) CreateCommittedSeal(header *types.Header) ([]byte, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	msg := commitMsg(hash[:])

	return crypto.Sign(s.key, crypto.Keccak256(msg))
}

func (s *ECDSASigner) WriteCommittedSeals(
	header *types.Header,
	sealMap map[types.Address][]byte,
) (*types.Header, error) {
	if len(sealMap) == 0 {
		return nil, ErrEmptyCommittedSeals
	}

	seals := [][]byte{}

	for _, seal := range sealMap {
		if len(seal) != IstanbulExtraSeal {
			return nil, ErrInvalidCommittedSealLength
		}

		seals = append(seals, seal)
	}

	serializedSeal := SerializedSeal(seals)

	err := s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.CommittedSeal = &serializedSeal
	})

	if err != nil {
		return nil, err
	}

	return header, nil
}

func (s *ECDSASigner) VerifyCommittedSeal(
	rawSet validators.ValidatorSet,
	header *types.Header,
	quorumFn validators.QuorumImplementation,
) error {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	cs, ok := extra.CommittedSeal.(*SerializedSeal)
	if !ok {
		return ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawSet.(*validators.ECDSAValidatorSet)
	if !ok {
		return ErrInvalidValidatorSet
	}

	// get the message that needs to be signed
	// this not signing! just removing the fields that should be signed
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash[:])

	return s.verifyCommittedSealsImpl(cs, rawMsg, *validatorSet, quorumFn)
}

func (s *ECDSASigner) VerifyParentCommittedSeal(
	rawParentSet validators.ValidatorSet,
	parent, header *types.Header,
	quorumFn validators.QuorumImplementation,
) error {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	parentCs, ok := extra.ParentCommittedSeal.(*SerializedSeal)
	if !ok {
		return ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawParentSet.(*validators.ECDSAValidatorSet)
	if !ok {
		return ErrInvalidValidatorSet
	}

	// get the message that needs to be signed
	// this not signing! just removing the fields that should be signed
	hash, err := s.CalculateHeaderHash(parent)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash[:])

	return s.verifyCommittedSealsImpl(parentCs, rawMsg, *validatorSet, quorumFn)
}

func (s *ECDSASigner) SignGossipMessage(msg *proto.MessageReq) error {
	return signMsg(s.key, msg)
}

func (s *ECDSASigner) ValidateGossipMessage(msg *proto.MessageReq) error {
	return ValidateMsg(msg)
}

func (s *ECDSASigner) initIbftExtra(header *types.Header, vals validators.ValidatorSet, parentCommittedSeal Sealer) {
	putIbftExtra(header, &IstanbulExtra{
		Validators:          vals,
		Seal:                nil,
		CommittedSeal:       &SerializedSeal{},
		ParentCommittedSeal: parentCommittedSeal,
	})
}

func (s *ECDSASigner) packFieldIntoIbftExtra(h *types.Header, updateFn func(*IstanbulExtra)) error {
	extra, err := s.GetIBFTExtra(h)
	if err != nil {
		return err
	}

	updateFn(extra)

	putIbftExtra(h, extra)

	return nil
}

func (s *ECDSASigner) filterHeaderForHash(header *types.Header) (*types.Header, error) {
	// This will effectively remove the Seal and Committed Seal fields,
	// while keeping proposer vanity and validator set
	// because extra.Validators, extra.ParentCommittedSeal is what we got from `h` in the first place.
	clone := header.Copy()

	extra, err := s.GetIBFTExtra(clone)
	if err != nil {
		return nil, err
	}

	s.initIbftExtra(clone, extra.Validators, extra.ParentCommittedSeal)

	return clone, nil
}

func (s *ECDSASigner) CalculateHeaderHash(header *types.Header) (types.Hash, error) {
	filteredHeader, err := s.filterHeaderForHash(header)
	if err != nil {
		return types.ZeroHash, err
	}

	return calculateHeaderHash(filteredHeader), nil
}

func (s *ECDSASigner) verifyCommittedSealsImpl(
	committedSeal *SerializedSeal,
	msg []byte,
	validators validators.ECDSAValidatorSet,
	quorumFn validators.QuorumImplementation,
) error {
	// Committed seals shouldn't be empty
	if len(*committedSeal) == 0 {
		return ErrEmptyCommittedSeals
	}

	visited := map[types.Address]struct{}{}

	for _, seal := range *committedSeal {
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
	if validSeals := len(visited); validSeals < quorumFn(&validators) {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SerializedSeal) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	if len(*s) == 0 {
		return ar.NewNullArray()
	}

	committed := ar.NewArray()

	for _, a := range *s {
		if len(a) == 0 {
			committed.Set(ar.NewNull())
		} else {
			committed.Set(ar.NewBytes(a))
		}
	}

	return committed
}

func (s *SerializedSeal) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	vals, err := v.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", v.Type())
	}

	(*s) = make([][]byte, len(vals))

	for indx, val := range vals {
		if (*s)[indx], err = val.GetBytes((*s)[indx]); err != nil {
			return err
		}
	}

	return nil
}
