package signer

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

// KeyManager is a delegated module that signs data
type KeyManager interface {
	// Type returns Validator type signer supports
	Type() validators.ValidatorType
	// Address returns an address of signer
	Address() types.Address
	// NewEmptyValidators creates empty validator collection the Signer expects
	NewEmptyValidators() validators.Validators
	// NewEmptyCommittedSeals creates empty committed seals the Signer expects
	NewEmptyCommittedSeals() Seals
	// SignProposerSeal creates a signature for ProposerSeal
	SignProposerSeal(hash []byte) ([]byte, error)
	// SignCommittedSeal creates a signature for committed seal
	SignCommittedSeal(hash []byte) ([]byte, error)
	// VerifyCommittedSeal verifies a committed seal
	VerifyCommittedSeal(vals validators.Validators, signer types.Address, sig, hash []byte) error
	// GenerateCommittedSeals creates CommittedSeals from committed seals
	GenerateCommittedSeals(sealsByValidator map[types.Address][]byte, vals validators.Validators) (Seals, error)
	// VerifyCommittedSeals verifies CommittedSeals
	VerifyCommittedSeals(seals Seals, hash []byte, vals validators.Validators) (int, error)
	// SignIBFTMessage signs for arbitrary bytes message
	SignIBFTMessage(msg []byte) ([]byte, error)
	// Ecrecover recovers address from signature and message
	Ecrecover(sig []byte, msg []byte) (types.Address, error)
}
