package signer

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

// KeyManager is a delegated module that signs data
type KeyManager interface {
	Type() validators.ValidatorType
	Address() types.Address

	// initializer of modules related to KeyManager
	NewEmptyValidators() validators.Validators
	NewEmptyCommittedSeals() Sealer

	// Seal
	SignSeal(hash []byte) ([]byte, error)

	// CommittedSeal
	SignCommittedSeal(hash []byte) ([]byte, error)
	VerifyCommittedSeal(vals validators.Validators, signer types.Address, sig, hash []byte) error

	// CommittedSeals
	GenerateCommittedSeals(sealsByValidator map[types.Address][]byte, extra *IstanbulExtra) (Sealer, error)
	VerifyCommittedSeals(seals Sealer, hash []byte, vals validators.Validators) (int, error)

	SignIBFTMessage(msg []byte) ([]byte, error)
	Ecrecover(sig []byte, msg []byte) (types.Address, error)
}
