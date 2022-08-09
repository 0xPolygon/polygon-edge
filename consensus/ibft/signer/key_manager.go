package signer

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

// KeyManager is a delegated object to sign data
type KeyManager interface {
	Address() types.Address
	NewEmptyIstanbulExtra() *IstanbulExtra
	NewEmptyCommittedSeal() Sealer
	SignSeal([]byte) ([]byte, error)
	SignCommittedSeal([]byte) ([]byte, error)
	Ecrecover(sig []byte, digest []byte) (types.Address, error)
	GenerateCommittedSeals(map[types.Address][]byte, *IstanbulExtra) (Sealer, error)
	VerifyCommittedSeal(validators.ValidatorSet, types.Address, []byte, []byte) error
	VerifyCommittedSeals(Sealer, []byte, validators.ValidatorSet) (int, error)
	SignIBFTMessage(msg []byte) ([]byte, error)
}
