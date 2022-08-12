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
	ErrEmptyParentCommittedSeals  = errors.New("empty parent committed seals")
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
	Type() validators.ValidatorType
	Address() types.Address
	InitIBFTExtra(header *types.Header, parentCommittedSeal Sealer, set validators.Validators) error
	GetIBFTExtra(header *types.Header) (*IstanbulExtra, error)
	GetParentCommittedSeals(header *types.Header) (Sealer, error)
	WriteProposerSeal(*types.Header) (*types.Header, error)
	SignIBFTMessage([]byte) ([]byte, error)
	Ecrecover([]byte, []byte) (types.Address, error)
	EcrecoverFromHeader(*types.Header) (types.Address, error)
	CreateCommittedSeal([]byte) ([]byte, error)
	VerifyCommittedSeal(validators.Validators, types.Address, []byte, []byte) error
	WriteCommittedSeals(*types.Header, map[types.Address][]byte) (*types.Header, error)
	VerifyCommittedSeals(
		header *types.Header,
		validators validators.Validators,
		quorumSize int,
	) error
	VerifyParentCommittedSeals(
		parent, header *types.Header,
		parentValidators validators.Validators,
		quorum int,
		mustExist bool,
	) error
	CalculateHeaderHash(*types.Header) (types.Hash, error)
}

type SignerImpl struct {
	keyManager KeyManager
}

func NewSigner(
	keyManager KeyManager,
) Signer {
	return &SignerImpl{
		keyManager: keyManager,
	}
}

func (s *SignerImpl) Type() validators.ValidatorType {
	return s.keyManager.Type()
}

func (s *SignerImpl) Address() types.Address {
	return s.keyManager.Address()
}

func (s *SignerImpl) InitIBFTExtra(
	header *types.Header,
	parentCommittedSeal Sealer,
	validators validators.Validators,
) error {
	putIbftExtra(header, &IstanbulExtra{
		Validators:          validators,
		ProposerSeal:        nil,
		CommittedSeal:       s.keyManager.NewEmptyCommittedSeal(),
		ParentCommittedSeal: parentCommittedSeal,
	})

	return nil
}

func (s *SignerImpl) GetIBFTExtra(h *types.Header) (*IstanbulExtra, error) {
	if err := verifyIBFTExtraSize(h); err != nil {
		return nil, err
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		Validators:    s.keyManager.NewEmptyValidatorSet(),
		ProposerSeal:  nil,
		CommittedSeal: s.keyManager.NewEmptyCommittedSeal(),
	}

	if h.Number > 1 {
		extra.ParentCommittedSeal = s.keyManager.NewEmptyCommittedSeal()
	}

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

	seal, err := s.keyManager.SignSeal(
		crypto.Keccak256(hash[:]),
	)
	if err != nil {
		return nil, err
	}

	header.ExtraData = packSealIntoIExtra(
		header.ExtraData,
		seal,
	)

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
		crypto.Keccak256(wrapCommitHash(hash[:])),
	)
}

func (s *SignerImpl) VerifyCommittedSeal(
	validators validators.Validators,
	signer types.Address,
	signature, hash []byte,
) error {
	return s.keyManager.VerifyCommittedSeal(
		validators,
		signer,
		signature,
		wrapCommitHash(hash),
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

	header.ExtraData = packCommittedSealIntoExtra(
		header.ExtraData,
		committedSeal,
	)

	return header, nil
}

func (s *SignerImpl) VerifyCommittedSeals(
	header *types.Header,
	validators validators.Validators,
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

	rawMsg := wrapCommitHash(hash[:])

	numSeals, err := s.keyManager.VerifyCommittedSeals(
		extra.CommittedSeal,
		rawMsg,
		validators,
	)
	if err != nil {
		return err
	}

	if numSeals < quorumSize {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SignerImpl) VerifyParentCommittedSeals(
	parent, header *types.Header,
	parentValidators validators.Validators,
	quorum int,
	mustExist bool,
) error {
	parentCommittedSeals, err := s.GetParentCommittedSeals(header)
	if err != nil {
		return err
	}

	if parentCommittedSeals == nil {
		// Throw error for the proposed header
		if mustExist {
			return ErrEmptyParentCommittedSeals
		}

		// Don't throw if the flag is unset for backward compatibility
		// (for the past headers)
		return nil
	}

	rawMsg := wrapCommitHash(parent.Hash[:])

	numSeals, err := s.keyManager.VerifyCommittedSeals(
		parentCommittedSeals,
		rawMsg,
		parentValidators,
	)
	if err != nil {
		return err
	}

	if numSeals < quorum {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

func (s *SignerImpl) SignIBFTMessage(msg []byte) ([]byte, error) {
	return s.keyManager.SignIBFTMessage(msg)
}

func (s *SignerImpl) initIbftExtra(
	header *types.Header,
	validators validators.Validators,
	parentCommittedSeal Sealer,
) {
	putIbftExtra(header, &IstanbulExtra{
		Validators:          validators,
		ProposerSeal:        nil,
		CommittedSeal:       s.keyManager.NewEmptyCommittedSeal(),
		ParentCommittedSeal: parentCommittedSeal,
	})
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

// extractValidators extracts Validators from IBFT Extra in Header
func (s *SignerImpl) getValidators(header *types.Header) (validators.Validators, error) {
	data := header.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		Validators: s.keyManager.NewEmptyValidatorSet(),
	}

	if err := extra.unmarshalRLPForValidators(data); err != nil {
		return nil, err
	}

	return extra.Validators, nil
}

// extractParentCommittedSeals extracts Parent Committed Seals from IBFT Extra in Header
func (s *SignerImpl) GetParentCommittedSeals(header *types.Header) (Sealer, error) {
	data := header.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		ParentCommittedSeal: s.keyManager.NewEmptyCommittedSeal(),
	}

	if err := extra.unmarshalRLPForParentCS(data); err != nil {
		return nil, err
	}

	return extra.ParentCommittedSeal, nil
}

func verifyIBFTExtraSize(header *types.Header) error {
	if len(header.ExtraData) < IstanbulExtraVanity {
		return fmt.Errorf(
			"wrong extra size, expected greater than or equal to %d but actual %d",
			IstanbulExtraVanity,
			len(header.ExtraData),
		)
	}

	return nil
}
