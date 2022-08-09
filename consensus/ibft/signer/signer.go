package signer

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

var (
	ErrEmptyCommittedSeals        = errors.New("empty committed seals")
	ErrInvalidCommittedSealLength = errors.New("invalid committed seal length")
	ErrInvalidCommittedSealType   = errors.New("invalid committed seal type")
	ErrRepeatedCommittedSeal      = errors.New("repeated seal in committed seals")
	ErrNonValidatorCommittedSeal  = errors.New("found committed seal signed by non validator")
	ErrNotEnoughCommittedSeals    = errors.New("not enough seals to seal block")
	ErrSignerMismatch             = errors.New("mismatch address between signer and message sender")
	ErrValidatorNotFound          = errors.New("not found validator in validator set")
	ErrInvalidValidatorSet        = errors.New("invalid validator set type")
	ErrInvalidSignature           = errors.New("invalid signature")
	ErrNilParentHeader            = errors.New("parent header is nil")
)

// Signer is responsible for signing for blocks and messages in IBFT
type Signer interface {
	Address() types.Address
	InitIBFTExtra(header, parent *types.Header, set validators.ValidatorSet) error
	GetIBFTExtra(*types.Header) (*IstanbulExtra, error)
	WriteProposerSeal(*types.Header) (*types.Header, error)
	Ecrecover(sig, dig []byte) (types.Address, error)
	EcrecoverFromHeader(*types.Header) (types.Address, error)
	CreateCommittedSeal([]byte) ([]byte, error)
	VerifyCommittedSeal(validators.ValidatorSet, types.Address, []byte, []byte) error
	WriteCommittedSeals(*types.Header, map[types.Address][]byte) (*types.Header, error)
	VerifyCommittedSeals(validators.ValidatorSet, *types.Header, int) error
	VerifyParentCommittedSeals(
		set validators.ValidatorSet,
		parent, header *types.Header,
		quorumSize int,
	) error
	SignIBFTMessage([]byte) ([]byte, error)
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
		ProposerSeal:        nil,
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

func (s *SignerImpl) WriteProposerSeal(header *types.Header) (*types.Header, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	seal, err := s.keyManager.SignSeal(crypto.Keccak256(hash[:]))
	if err != nil {
		return nil, err
	}

	if err = s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.ProposerSeal = seal
	}); err != nil {
		return nil, err
	}

	return header, nil
}

func (s *SignerImpl) Ecrecover(signature, digest []byte) (types.Address, error) {
	return s.keyManager.Ecrecover(signature, digest)
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

	return s.keyManager.Ecrecover(extra.ProposerSeal, hash[:])
}

func (s *SignerImpl) CreateCommittedSeal(hash []byte) ([]byte, error) {
	return s.keyManager.SignCommittedSeal(
		crypto.Keccak256(commitMsg(hash[:])),
	)
}

func (s *SignerImpl) VerifyCommittedSeal(
	set validators.ValidatorSet,
	signer types.Address,
	signature, hash []byte,
) error {
	return s.keyManager.VerifyCommittedSeal(
		set,
		signer,
		signature,
		hash,
	)
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

func (s *SignerImpl) VerifyCommittedSeals(
	validators validators.ValidatorSet,
	header *types.Header,
	quorumSize int,
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

	numSeals, err := s.keyManager.VerifyCommittedSeals(extra.CommittedSeal, rawMsg, validators)
	if err != nil {
		return err
	}

	if numSeals < quorumSize {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SignerImpl) VerifyParentCommittedSeals(
	parentValidators validators.ValidatorSet,
	parent, header *types.Header,
	parentQuorumSize int,
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

	numSeals, err := s.keyManager.VerifyCommittedSeals(extra.ParentCommittedSeal, rawMsg, parentValidators)
	if err != nil {
		return err
	}

	if numSeals < parentQuorumSize {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SignerImpl) SignIBFTMessage(msg []byte) ([]byte, error) {
	return s.keyManager.SignIBFTMessage(msg)
}

func (s *SignerImpl) initIbftExtra(
	header *types.Header,
	validators validators.ValidatorSet,
	parentCommittedSeal Sealer,
) {
	putIbftExtra(header, &IstanbulExtra{
		Validators:          validators,
		ProposerSeal:        nil,
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
