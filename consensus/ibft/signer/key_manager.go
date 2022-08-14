package signer

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

// KeyManager is a delegated object to sign data
type KeyManager interface {
	Type() validators.ValidatorType
	Address() types.Address
	NewEmptyValidatorSet() validators.Validators
	NewEmptyCommittedSeal() Sealer
	SignSeal([]byte) ([]byte, error)
	SignCommittedSeal([]byte) ([]byte, error)
	Ecrecover(sig []byte, digest []byte) (types.Address, error)
	GenerateCommittedSeals(map[types.Address][]byte, *IstanbulExtra) (Sealer, error)
	VerifyCommittedSeal(validators.Validators, types.Address, []byte, []byte) error
	VerifyCommittedSeals(Sealer, []byte, validators.Validators) (int, error)
	SignIBFTMessage(msg []byte) ([]byte, error)
}
