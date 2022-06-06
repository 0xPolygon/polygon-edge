package signer

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	ErrEmptyCommittedSeals        = errors.New("empty committed seals")
	ErrInvalidCommittedSealLength = errors.New("invalid committed seal length")
	ErrInvalidCommittedSealType   = errors.New("invalid committed seal type")
	ErrRepeatedCommittedSeal      = errors.New("repeated seal in committed seals")
	ErrNonValidatorCommittedSeal  = errors.New("found committed seal signed by non validator")
	ErrNotEnoughCommittedSeals    = errors.New("not enough seals to seal block")
	ErrSignerNotFound             = errors.New("not found signer in validator set")
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
	SignGossipMessage(*proto.MessageReq) error
	ValidateGossipMessage(*proto.MessageReq) error
	CalculateHeaderHash(*types.Header) (types.Hash, error)
}
