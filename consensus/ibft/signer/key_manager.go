package signer

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

// KeyManager is a delegated object to sign data
type KeyManager interface {
	Type() validators.ValidatorType
	Address() types.Address

	// initializer of modules related to KeyManager
	NewEmptyValidators() validators.Validators
	NewEmptyCommittedSeals() Sealer

	// Seal
	SignSeal([]byte) ([]byte, error)

	// CommittedSeal
	SignCommittedSeal([]byte) ([]byte, error)
	VerifyCommittedSeal(validators.Validators, types.Address, []byte, []byte) error

	// CommittedSeals
	GenerateCommittedSeals(map[types.Address][]byte, *IstanbulExtra) (Sealer, error)
	VerifyCommittedSeals(Sealer, []byte, validators.Validators) (int, error)

	SignIBFTMessage(msg []byte) ([]byte, error)
	Ecrecover(sig []byte, digest []byte) (types.Address, error)
}
