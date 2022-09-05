package signer

import (
	"errors"

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
	ErrValidatorNotFound          = errors.New("validator not found in validator set")
	ErrInvalidValidators          = errors.New("invalid validators type")
	ErrInvalidValidator           = errors.New("invalid validator type")
	ErrInvalidSignature           = errors.New("invalid signature")
)

// Signer is responsible for signing for blocks and messages in IBFT
type Signer interface {
	Type() validators.ValidatorType
	Address() types.Address

	// IBFT Extra
	InitIBFTExtra(*types.Header, validators.Validators, Seals)
	GetIBFTExtra(*types.Header) (*IstanbulExtra, error)
	GetValidators(*types.Header) (validators.Validators, error)

	// ProposerSeal
	WriteProposerSeal(*types.Header) (*types.Header, error)
	EcrecoverFromHeader(*types.Header) (types.Address, error)

	// CommittedSeal
	CreateCommittedSeal([]byte) ([]byte, error)
	VerifyCommittedSeal(validators.Validators, types.Address, []byte, []byte) error

	// CommittedSeals
	WriteCommittedSeals(*types.Header, map[types.Address][]byte) (*types.Header, error)
	VerifyCommittedSeals(
		header *types.Header,
		validators validators.Validators,
		quorumSize int,
	) error

	// ParentCommittedSeals
	VerifyParentCommittedSeals(
		parent, header *types.Header,
		parentValidators validators.Validators,
		quorum int,
		mustExist bool,
	) error

	// IBFTMessage
	SignIBFTMessage([]byte) ([]byte, error)
	EcrecoverFromIBFTMessage([]byte, []byte) (types.Address, error)

	// Hash of Header
	CalculateHeaderHash(*types.Header) (types.Hash, error)
}

// SignerImpl is an implementation that meets Signer
type SignerImpl struct {
	keyManager       KeyManager
	parentKeyManager KeyManager
}

// NewSigner is a constructor of SignerImpl
func NewSigner(
	keyManager KeyManager,
	parentKeyManager KeyManager,
) *SignerImpl {
	return &SignerImpl{
		keyManager:       keyManager,
		parentKeyManager: parentKeyManager,
	}
}

// Type returns that validator type the signer expects
func (s *SignerImpl) Type() validators.ValidatorType {
	return s.keyManager.Type()
}

// Address returns the signer's address
func (s *SignerImpl) Address() types.Address {
	return s.keyManager.Address()
}

// InitIBFTExtra initializes the extra field in the given header
// based on given validators and parent committed seals
func (s *SignerImpl) InitIBFTExtra(
	header *types.Header,
	validators validators.Validators,
	parentCommittedSeals Seals,
) {
	s.initIbftExtra(
		header,
		validators,
		parentCommittedSeals,
	)
}

// GetIBFTExtra extracts IBFT Extra from the given header
func (s *SignerImpl) GetIBFTExtra(header *types.Header) (*IstanbulExtra, error) {
	if err := verifyIBFTExtraSize(header); err != nil {
		return nil, err
	}

	data := header.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		Validators:     s.keyManager.NewEmptyValidators(),
		ProposerSeal:   []byte{},
		CommittedSeals: s.keyManager.NewEmptyCommittedSeals(),
	}

	if header.Number > 1 {
		extra.ParentCommittedSeals = s.parentKeyManager.NewEmptyCommittedSeals()
	}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

// WriteProposerSeal signs and set ProposerSeal into IBFT Extra of the header
func (s *SignerImpl) WriteProposerSeal(header *types.Header) (*types.Header, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	seal, err := s.keyManager.SignProposerSeal(
		crypto.Keccak256(hash.Bytes()),
	)
	if err != nil {
		return nil, err
	}

	header.ExtraData = packProposerSealIntoExtra(
		header.ExtraData,
		seal,
	)

	return header, nil
}

// EcrecoverFromIBFTMessage recovers signer address from given signature and header hash
func (s *SignerImpl) EcrecoverFromHeader(header *types.Header) (types.Address, error) {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return types.Address{}, err
	}

	return s.keyManager.Ecrecover(extra.ProposerSeal, crypto.Keccak256(header.Hash.Bytes()))
}

// CreateCommittedSeal returns CommittedSeal from given hash
func (s *SignerImpl) CreateCommittedSeal(hash []byte) ([]byte, error) {
	return s.keyManager.SignCommittedSeal(
		// Of course, this keccaking of an extended array is not according to the IBFT 2.0 spec,
		// but almost nothing in this legacy signing package is. This is kept
		// in order to preserve the running chains that used these
		// old (and very, very incorrect) signing schemes
		crypto.Keccak256(
			wrapCommitHash(hash[:]),
		),
	)
}

// CreateCommittedSeal verifies a CommittedSeal
func (s *SignerImpl) VerifyCommittedSeal(
	validators validators.Validators,
	signer types.Address,
	signature, hash []byte,
) error {
	return s.keyManager.VerifyCommittedSeal(
		validators,
		signer,
		signature,
		crypto.Keccak256(
			wrapCommitHash(hash[:]),
		),
	)
}

// WriteCommittedSeals builds and writes CommittedSeals into IBFT Extra of the header
func (s *SignerImpl) WriteCommittedSeals(
	header *types.Header,
	sealMap map[types.Address][]byte,
) (*types.Header, error) {
	if len(sealMap) == 0 {
		return nil, ErrEmptyCommittedSeals
	}

	validators, err := s.GetValidators(header)
	if err != nil {
		return nil, err
	}

	committedSeal, err := s.keyManager.GenerateCommittedSeals(sealMap, validators)
	if err != nil {
		return nil, err
	}

	header.ExtraData = packCommittedSealsIntoExtra(
		header.ExtraData,
		committedSeal,
	)

	return header, nil
}

// VerifyCommittedSeals verifies CommittedSeals in IBFT Extra of the header
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

	rawMsg := crypto.Keccak256(
		wrapCommitHash(hash[:]),
	)

	numSeals, err := s.keyManager.VerifyCommittedSeals(
		extra.CommittedSeals,
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

// VerifyParentCommittedSeals verifies ParentCommittedSeals in IBFT Extra of the header
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

	if parentCommittedSeals == nil || parentCommittedSeals.Num() == 0 {
		// Throw error for the proposed header
		if mustExist {
			return ErrEmptyParentCommittedSeals
		}

		// Don't throw if the flag is unset for backward compatibility
		// (for the past headers)
		return nil
	}

	rawMsg := crypto.Keccak256(
		wrapCommitHash(parent.Hash.Bytes()),
	)

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

// SignIBFTMessage signs arbitrary message
func (s *SignerImpl) SignIBFTMessage(msg []byte) ([]byte, error) {
	return s.keyManager.SignIBFTMessage(crypto.Keccak256(msg))
}

// EcrecoverFromIBFTMessage recovers signer address from given signature and digest
func (s *SignerImpl) EcrecoverFromIBFTMessage(signature, digest []byte) (types.Address, error) {
	return s.keyManager.Ecrecover(signature, crypto.Keccak256(digest))
}

// InitIBFTExtra initializes the extra field
func (s *SignerImpl) initIbftExtra(
	header *types.Header,
	validators validators.Validators,
	parentCommittedSeal Seals,
) {
	putIbftExtra(header, &IstanbulExtra{
		Validators:           validators,
		ProposerSeal:         []byte{},
		CommittedSeals:       s.keyManager.NewEmptyCommittedSeals(),
		ParentCommittedSeals: parentCommittedSeal,
	})
}

// CalculateHeaderHash calculates header hash for IBFT Extra
func (s *SignerImpl) CalculateHeaderHash(header *types.Header) (types.Hash, error) {
	filteredHeader, err := s.filterHeaderForHash(header)
	if err != nil {
		return types.ZeroHash, err
	}

	return calculateHeaderHash(filteredHeader), nil
}

func (s *SignerImpl) GetValidators(header *types.Header) (validators.Validators, error) {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return nil, err
	}

	return extra.Validators, nil
}

// GetParentCommittedSeals extracts Parent Committed Seals from IBFT Extra in Header
func (s *SignerImpl) GetParentCommittedSeals(header *types.Header) (Seals, error) {
	if err := verifyIBFTExtraSize(header); err != nil {
		return nil, err
	}

	data := header.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		ParentCommittedSeals: s.keyManager.NewEmptyCommittedSeals(),
	}

	if err := extra.unmarshalRLPForParentCS(data); err != nil {
		return nil, err
	}

	return extra.ParentCommittedSeals, nil
}

// filterHeaderForHash removes unnecessary fields from IBFT Extra of the header
// for hash calculation
func (s *SignerImpl) filterHeaderForHash(header *types.Header) (*types.Header, error) {
	clone := header.Copy()

	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return nil, err
	}

	parentCommittedSeals := extra.ParentCommittedSeals
	if parentCommittedSeals != nil && parentCommittedSeals.Num() == 0 {
		// avoid to set ParentCommittedSeals in extra for hash calculation
		// in case of empty ParentCommittedSeals for backward compatibility
		parentCommittedSeals = nil
	}

	// This will effectively remove the Seal and CommittedSeals from the IBFT Extra of header,
	// while keeping proposer vanity, validator set, and ParentCommittedSeals
	s.initIbftExtra(clone, extra.Validators, parentCommittedSeals)

	return clone, nil
}
