package signer

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	ErrEmptyCommittedSeals        = errors.New("empty committed seals")
	ErrInvalidCommittedSealLength = errors.New("invalid committed seal length")
	ErrInvalidCommittedSealType   = errors.New("invalid committed seal type")
	ErrRepeatedCommittedSeal      = errors.New("repeated seal in committed seals")
	ErrNonValidatorCommittedSeal  = errors.New("found committed seal signed by non validator")
	ErrNotEnoughCommittedSeals    = errors.New("not enough seals to seal block")
	ErrValidatorNotFound          = errors.New("not found validator in validator set")
	ErrInvalidValidatorSet        = errors.New("invalid validator set type")
	ErrInvalidBLSSignature        = errors.New("invalid BLS signature")
	ErrNilParentHeader            = errors.New("parent header is nil")
)

// Signer is responsible for signing for blocks and messages in IBFT
type Signer interface {
	Address() types.Address
	InitIBFTExtra(header, parent *types.Header, set validators.ValidatorSet) error
	GetIBFTExtra(*types.Header) (*IstanbulExtra, error)
	WriteSeal(*types.Header) (*types.Header, error)
	EcrecoverFromHeader(*types.Header) (types.Address, error)
	CreateCommittedSeal(*types.Header) ([]byte, error)
	WriteCommittedSeals(*types.Header, map[types.Address][]byte) (*types.Header, error)
	VerifyCommittedSeal(validators.ValidatorSet, *types.Header, validators.QuorumImplementation) error
	VerifyParentCommittedSeal(
		set validators.ValidatorSet,
		parent, header *types.Header,
		quorumFn validators.QuorumImplementation,
	) error
	SignIBFTMessage(*proto.MessageReq) error
	ValidateIBFTMessage(*proto.MessageReq) error
	CalculateHeaderHash(*types.Header) (types.Hash, error)
}

type SignerImpl struct {
	keyManager KeyManager
}

func NewSigner(keyManager KeyManager) Signer {
	return &SignerImpl{
		keyManager: keyManager,
	}
}

func (s *SignerImpl) Address() types.Address {
	return s.keyManager.Address()
}

func (s *SignerImpl) InitIBFTExtra(header, parent *types.Header, set validators.ValidatorSet) error {
	var parentCommittedSeal Sealer

	if header.Number > 1 {
		if parent == nil {
			return ErrNilParentHeader
		}

		parentExtra, err := s.GetIBFTExtra(parent)
		if err != nil {
			return err
		}

		parentCommittedSeal = parentExtra.CommittedSeal
	}

	putIbftExtra(header, &IstanbulExtra{
		Validators:          set,
		Seal:                nil,
		CommittedSeal:       s.keyManager.NewEmptyCommittedSeal(),
		ParentCommittedSeal: parentCommittedSeal,
	})

	return nil
}

func (s *SignerImpl) GetIBFTExtra(h *types.Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		return nil, fmt.Errorf(
			"wrong extra size, expected greater than or equal to %d but actual %d",
			IstanbulExtraVanity,
			len(h.ExtraData),
		)
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := s.keyManager.NewEmptyIstanbulExtra()

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

func (s *SignerImpl) WriteSeal(header *types.Header) (*types.Header, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	seal, err := s.keyManager.SignSeal(crypto.Keccak256(hash[:]))
	if err != nil {
		return nil, err
	}

	if err = s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.Seal = seal
	}); err != nil {
		return nil, err
	}

	return header, nil
}

func (s *SignerImpl) EcrecoverFromHeader(header *types.Header) (types.Address, error) {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return types.Address{}, err
	}

	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return types.Address{}, err
	}

	return s.keyManager.Ecrecover(extra.Seal, hash[:])
}

func (s *SignerImpl) CreateCommittedSeal(header *types.Header) ([]byte, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	msg := crypto.Keccak256(commitMsg(hash[:]))

	return s.keyManager.SignCommittedSeal(msg)
}

func (s *SignerImpl) WriteCommittedSeals(
	header *types.Header,
	sealMap map[types.Address][]byte,
) (*types.Header, error) {
	if len(sealMap) == 0 {
		return nil, ErrEmptyCommittedSeals
	}

	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return nil, err
	}

	committedSeal, err := s.keyManager.GenerateCommittedSeals(sealMap, extra)
	if err != nil {
		return nil, err
	}

	if err = s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.CommittedSeal = committedSeal
	}); err != nil {
		return nil, err
	}

	return header, nil
}

func (s *SignerImpl) VerifyCommittedSeal(
	validators validators.ValidatorSet,
	header *types.Header,
	quorumFn validators.QuorumImplementation,
) error {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash[:])

	numSeals, err := s.keyManager.VerifyCommittedSeal(extra.CommittedSeal, rawMsg, validators)
	if err != nil {
		return err
	}

	if numSeals < quorumFn(validators) {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SignerImpl) VerifyParentCommittedSeal(
	parentValidators validators.ValidatorSet,
	parent, header *types.Header,
	quorumFn validators.QuorumImplementation,
) error {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	hash, err := s.CalculateHeaderHash(parent)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash[:])

	numSeals, err := s.keyManager.VerifyCommittedSeal(extra.ParentCommittedSeal, rawMsg, parentValidators)
	if err != nil {
		return err
	}

	if numSeals < quorumFn(parentValidators) {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SignerImpl) SignIBFTMessage(msg *proto.MessageReq) error {
	return s.keyManager.SignIBFTMessage(msg)
}

func (s *SignerImpl) ValidateIBFTMessage(msg *proto.MessageReq) error {
	return s.keyManager.ValidateIBFTMessage(msg)
}

func (s *SignerImpl) initIbftExtra(
	header *types.Header,
	validators validators.ValidatorSet,
	parentCommittedSeal Sealer,
) {
	putIbftExtra(header, &IstanbulExtra{
		Validators:          validators,
		Seal:                nil,
		CommittedSeal:       s.keyManager.NewEmptyCommittedSeal(),
		ParentCommittedSeal: parentCommittedSeal,
	})
}

func (s *SignerImpl) packFieldIntoIbftExtra(h *types.Header, updateFn func(*IstanbulExtra)) error {
	extra, err := s.GetIBFTExtra(h)
	if err != nil {
		return err
	}

	updateFn(extra)

	putIbftExtra(h, extra)

	return nil
}

func (s *SignerImpl) CalculateHeaderHash(header *types.Header) (types.Hash, error) {
	filteredHeader, err := s.filterHeaderForHash(header)
	if err != nil {
		return types.ZeroHash, err
	}

	return calculateHeaderHash(filteredHeader), nil
}

func (s *SignerImpl) filterHeaderForHash(header *types.Header) (*types.Header, error) {
	clone := header.Copy()

	extra, err := s.GetIBFTExtra(clone)
	if err != nil {
		return nil, err
	}

	// This will effectively remove the Seal and Committed Seal fields,
	// while keeping proposer vanity and validator set
	// because extra.Validators, extra.ParentCommittedSeal is what we got from `h` in the first place.
	s.initIbftExtra(clone, extra.Validators, extra.ParentCommittedSeal)

	return clone, nil
}
